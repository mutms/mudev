package workspace

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutms/mudev/internal/config"
	"github.com/mutms/mudev/internal/exec"
	"github.com/mutms/mudev/internal/git"
)

// fakePHPAndComposer puts stand-in `php` and `composer` binaries on PATH for
// the duration of the test.
//
// The fake `php` prints a real-looking `php -v` banner for the given version,
// standing in for mpd's dispatcher, so detectPHP has a version to read. The
// fake `composer` records the PHP version it was forced to use — from
// MPD_PHP_FORCE_VERSION — into vendor/marker.txt in its working directory (the
// export root), so a test can assert composer was invoked, that its output
// lands in the artifact, and that the detected version reached it.
func fakePHPAndComposer(t *testing.T, version string) {
	t.Helper()

	bin := t.TempDir()

	php := "#!/bin/sh\necho \"PHP " + version + ".9 (cli) (built: today)\"\n"

	if err := os.WriteFile(filepath.Join(bin, "php"), []byte(php), 0o755); err != nil {
		t.Fatal(err)
	}

	composer := "#!/bin/sh\nmkdir -p vendor\necho \"php=${MPD_PHP_FORCE_VERSION}\" > vendor/marker.txt\n"

	if err := os.WriteFile(filepath.Join(bin, "composer"), []byte(composer), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// tarList returns the paths inside a gzipped tar, so a test can assert what did
// and did not make it into the artifact.
func tarList(t *testing.T, tgz string) string {
	t.Helper()

	res, err := exec.Capture(context.Background(), exec.Cmd{Name: "tar", Args: []string{"-tzf", tgz}})
	if err != nil || res.Failed() {
		t.Fatalf("tar -tzf %s: %v %s", tgz, err, res.Stderr)
	}

	return res.Stdout
}

// extractFile unpacks a gzipped tar into a temp dir and returns one file's
// contents, so a test can assert on what an entry actually holds rather than
// just that its name is present.
func extractFile(t *testing.T, tgz string, name string) string {
	t.Helper()

	dir := t.TempDir()

	res, err := exec.Capture(context.Background(), exec.Cmd{Name: "tar", Args: []string{"-xzf", tgz, "-C", dir}})
	if err != nil || res.Failed() {
		t.Fatalf("tar -xzf %s: %v %s", tgz, err, res.Stderr)
	}

	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("reading %s from artifact: %v", name, err)
	}

	return string(data)
}

// assembledFixture clones the standard core + plugin fixture into a workspace
// and returns its root and the target path for the exported tarball.
func assembledFixture(t *testing.T) (root string, target string) {
	t.Helper()

	recipePath, root := workspaceFixture(t, "502")

	if err := cloneInto(t, recipePath, root); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	return root, filepath.Join(filepath.Dir(root), "out.tgz")
}

func exportInto(root string, target string) error {
	return ProductionExport(context.Background(), ProductionExportOptions{
		Config: config.Defaults(),
		Root:   root,
		Target: target,
		Out:    io.Discard,
	})
}

func TestProductionExportPacksManagedCheckouts(t *testing.T) {
	fakePHPAndComposer(t, "8.4")

	root, target := assembledFixture(t)

	if err := exportInto(root, target); err != nil {
		t.Fatalf("ProductionExport: %v", err)
	}

	list := tarList(t, target)

	// Core at the root and the plugin at its real path both ship.
	want := []string{
		"./public/version.php",
		"./public/admin/tool/mulib/version.php",
		// public/ layout triggered composer, and its vendor/ output is packed.
		"./vendor/marker.txt",
	}

	for _, path := range want {
		if !strings.Contains(list, path) {
			t.Errorf("artifact is missing %s:\n%s", path, list)
		}
	}

	// The plugin's own .git must not travel with it.
	if strings.Contains(list, "/.git/") {
		t.Errorf("a checkout's .git leaked into the artifact:\n%s", list)
	}

	// The PHP version detected from the workspace root was forced onto composer:
	// the marker records what MPD_PHP_FORCE_VERSION it saw.
	marker := extractFile(t, target, "vendor/marker.txt")

	if strings.TrimSpace(marker) != "php=8.4" {
		t.Errorf("composer did not receive the detected PHP version: %q", marker)
	}
}

func TestDetectPHPRequiresPHP(t *testing.T) {
	// composer for a public/ layout tree needs php; a missing php has to be a
	// hard error, not a silent fall-through to a default interpreter.
	t.Setenv("PATH", t.TempDir())

	_, err := detectPHP(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected a missing php to be an error")
	}

	if !strings.Contains(err.Error(), "php") {
		t.Errorf("the error should name php: %v", err)
	}
}

func TestProductionExportRejectsDirty(t *testing.T) {
	fakePHPAndComposer(t, "8.4")

	root, target := assembledFixture(t)

	// A stray untracked file in a managed checkout must stop the export.
	plugin := filepath.Join(root, "public", "admin", "tool", "mulib")

	if err := os.WriteFile(filepath.Join(plugin, "scratch.txt"), []byte("wip"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := exportInto(root, target)
	if err == nil {
		t.Fatal("expected uncommitted changes to stop the export")
	}

	if !strings.Contains(err.Error(), "public/admin/tool/mulib") || !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("the error should name the dirty checkout: %v", err)
	}

	// Nothing was written when the pre-flight refused the run.
	if _, statErr := os.Stat(target); statErr == nil {
		t.Error("a dirty tree must not produce an artifact")
	}
}

func TestProductionExportIgnoresUnmanaged(t *testing.T) {
	fakePHPAndComposer(t, "8.4")

	root, target := assembledFixture(t)

	// A checkout nobody recorded, hidden from core the way a developer's own
	// scratch clone would be, so it does not itself make core dirty.
	extra := filepath.Join(root, "public", "local", "extra")

	initRepo(t, extra, "main")

	if err := os.WriteFile(filepath.Join(extra, "readme.txt"), []byte("scratch"), 0o644); err != nil {
		t.Fatal(err)
	}

	runGit(t, extra, "add", "--all")
	runGit(t, extra, "commit", "--quiet", "--message", "scratch")

	if err := git.AddExclude(root, "public/local"); err != nil {
		t.Fatal(err)
	}

	if err := exportInto(root, target); err != nil {
		t.Fatalf("ProductionExport: %v", err)
	}

	if list := tarList(t, target); strings.Contains(list, "public/local/extra") {
		t.Errorf("an unmanaged checkout was exported:\n%s", list)
	}
}

func TestNeedsComposer(t *testing.T) {
	root := t.TempDir()

	if needsComposer(root) {
		t.Error("a tree without public/version.php should not need composer")
	}

	if err := os.MkdirAll(filepath.Join(root, "public"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "public", "version.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !needsComposer(root) {
		t.Error("a public/ layout tree should need composer")
	}
}
