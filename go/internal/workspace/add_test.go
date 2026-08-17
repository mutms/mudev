package workspace

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutms/mudev/go/internal/config"
)

// bareRecipe writes a recipe with no plugins at all — the plain Moodle base a
// developer then adds to by hand.
func bareRecipe(t *testing.T, path string, core string, mdlbranch string) {
	t.Helper()

	recipe := `name: plain moodle
base:
  mdlbranch: "` + mdlbranch + `"
  source:
    git:
      remotes:
        origin: ` + core + `
      ref: origin/patch/mutms/MOODLE_502_STABLE
  localbranch: MOODLE_502_STABLE
plugins: []
`

	if err := os.WriteFile(path, []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}
}

// catalogueEntryFile writes one plugin catalogue file, optionally declaring a
// dependency on another plugin.
func catalogueEntryFile(t *testing.T, dir string, pkg string, remote string, requires string) string {
	t.Helper()

	vendor := filepath.Join(dir, "mutms")

	if err := os.MkdirAll(vendor, 0o755); err != nil {
		t.Fatal(err)
	}

	entry := `name: mutms/` + pkg + `
title: ` + pkg + `
relpath: public/admin/tool/` + strings.TrimPrefix(pkg, "tool_") + `
source:
  git:
    remotes:
      origin: ` + remote + `
requirements:
  MOODLE_500_STABLE:
    mdlbranches: ["502"]
`

	if requires != "" {
		entry += "    plugins: [" + requires + "]\n"
	}

	path := filepath.Join(vendor, pkg+".yaml")

	if err := os.WriteFile(path, []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

// addFixture assembles a plain Moodle workspace and a catalogue holding two
// plugins, the second of which needs the first.
func addFixture(t *testing.T) (root string, pluginsDir string) {
	t.Helper()

	base := t.TempDir()

	core := filepath.Join(base, "remote-core")
	mulib := filepath.Join(base, "remote-mulib")
	muprog := filepath.Join(base, "remote-muprog")

	fakeCore(t, core, "502")
	fakePlugin(t, mulib, "MOODLE_500_STABLE")
	fakePlugin(t, muprog, "MOODLE_500_STABLE")

	recipePath := filepath.Join(base, "recipe.yaml")
	bareRecipe(t, recipePath, core, "502")

	root = filepath.Join(base, "workspace")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := cloneInto(t, recipePath, root); err != nil {
		t.Fatalf("assembling the plain workspace: %v", err)
	}

	pluginsDir = filepath.Join(base, "catalogue")

	catalogueEntryFile(t, pluginsDir, "tool_mulib", mulib, "")
	catalogueEntryFile(t, pluginsDir, "tool_muprog", muprog, "mutms/tool_mulib")

	return root, pluginsDir
}

// addInto runs the add exactly as the command does.
func addInto(t *testing.T, root string, pluginsDir string, name string, ref string) error {
	t.Helper()

	return addWithOutput(t, root, pluginsDir, name, ref, io.Discard)
}

// addWithOutput is addInto for the tests that read what was reported.
func addWithOutput(t *testing.T, root string, pluginsDir string, name string, ref string, out io.Writer) error {
	t.Helper()

	cfg := config.Defaults()
	cfg.PluginsDir = pluginsDir

	return Add(context.Background(), AddOptions{
		Config: cfg,
		Plugin: name,
		Ref:    ref,
		Root:   root,
		Out:    out,
	})
}

// liveOf reads back the workspace's record of itself.
func liveOf(t *testing.T, root string) *Live {
	t.Helper()

	live, err := LoadLive(root)
	if err != nil {
		t.Fatal(err)
	}

	if live == nil {
		t.Fatalf("no live recipe in %s", root)
	}

	return live
}

func TestAddChecksOutAndRecordsAPlugin(t *testing.T) {
	root, pluginsDir := addFixture(t)

	if err := addInto(t, root, pluginsDir, "mutms/tool_mulib", ""); err != nil {
		t.Fatalf("add: %v", err)
	}

	dir := filepath.Join(root, "public", "admin", "tool", "mulib")

	if branch := strings.TrimSpace(gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD")); branch != "MOODLE_500_STABLE" {
		t.Errorf("checked out branch %q, want MOODLE_500_STABLE", branch)
	}

	live := liveOf(t, root)

	if _, ok := live.Plugin("mutms/tool_mulib"); !ok {
		t.Errorf("the plugin was checked out but not recorded in %s", config.LiveRecipeFile)
	}

	// The nested checkout must not show up as untracked in core.
	excludes, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(excludes), "/public/admin/tool/mulib") {
		t.Errorf("the checkout was not excluded in core:\n%s", excludes)
	}
}

// TestAddDoesNotInstallDependencies pins the rule that a dev site is composed
// deliberately: a declared requirement is information, and mudev adds exactly
// what it was asked for. Moodle checks dependencies at install time, which is
// where that check belongs.
func TestAddDoesNotInstallDependencies(t *testing.T) {
	root, pluginsDir := addFixture(t)

	var out strings.Builder

	if err := addWithOutput(t, root, pluginsDir, "mutms/tool_muprog", "", &out); err != nil {
		t.Fatalf("add: %v", err)
	}

	live := liveOf(t, root)

	if _, ok := live.Plugin("mutms/tool_muprog"); !ok {
		t.Fatalf("the requested plugin is missing from %s", config.LiveRecipeFile)
	}

	if _, ok := live.Plugin("mutms/tool_mulib"); ok {
		t.Errorf("a declared dependency was installed without being asked for")
	}

	if _, err := os.Stat(filepath.Join(root, "public", "admin", "tool", "mulib")); !os.IsNotExist(err) {
		t.Errorf("a declared dependency was checked out without being asked for")
	}

	// Silence would be worse than acting: the requirement is written down, and
	// a site that will not install is a slow way to discover it.
	if !strings.Contains(out.String(), "mutms/tool_mulib") {
		t.Errorf("the missing requirement was not reported:\n%s", out.String())
	}
}

// TestAddDoesNotReportSatisfiedRequirements is the other half: a requirement
// the workspace already has is not worth a line.
func TestAddDoesNotReportSatisfiedRequirements(t *testing.T) {
	root, pluginsDir := addFixture(t)

	if err := addInto(t, root, pluginsDir, "mutms/tool_mulib", ""); err != nil {
		t.Fatalf("adding the dependency: %v", err)
	}

	var out strings.Builder

	if err := addWithOutput(t, root, pluginsDir, "mutms/tool_muprog", "", &out); err != nil {
		t.Fatalf("add: %v", err)
	}

	if strings.Contains(out.String(), "does not have") {
		t.Errorf("a satisfied requirement was reported as missing:\n%s", out.String())
	}
}

func TestAddIsIdempotent(t *testing.T) {
	root, pluginsDir := addFixture(t)

	if err := addInto(t, root, pluginsDir, "mutms/tool_muprog", ""); err != nil {
		t.Fatalf("first add: %v", err)
	}

	before := gitOut(t, filepath.Join(root, "public", "admin", "tool", "muprog"), "rev-parse", "HEAD")

	if err := addInto(t, root, pluginsDir, "mutms/tool_muprog", ""); err != nil {
		t.Fatalf("second add: %v", err)
	}

	if got := len(liveOf(t, root).Plugins); got != 1 {
		t.Errorf("recorded %d plugin(s) after adding the same one twice, want 1", got)
	}

	after := gitOut(t, filepath.Join(root, "public", "admin", "tool", "muprog"), "rev-parse", "HEAD")

	if before != after {
		t.Errorf("repeating the add moved a checkout: %s → %s", before, after)
	}
}

// TestAddAcceptsAPluginFile covers the plugin that is not in the catalogue yet
// — the usual case for something being written right now.
func TestAddAcceptsAPluginFile(t *testing.T) {
	root, pluginsDir := addFixture(t)

	// Somewhere else entirely, so only the path can find it.
	elsewhere := filepath.Join(t.TempDir(), "loose")

	remote := liveRemoteOf(t, pluginsDir, "tool_mulib")
	path := catalogueEntryFile(t, elsewhere, "tool_scratch", remote, "")

	if err := addInto(t, root, pluginsDir, path, ""); err != nil {
		t.Fatalf("add: %v", err)
	}

	if _, ok := liveOf(t, root).Plugin("mutms/tool_scratch"); !ok {
		t.Errorf("a plugin given as a file was not recorded")
	}
}

// TestAddPinsARefWhenAsked checks --ref: a workspace of pinned editions wants a
// tag, not the tip of a branch.
func TestAddPinsARefWhenAsked(t *testing.T) {
	root, pluginsDir := addFixture(t)

	if err := addInto(t, root, pluginsDir, "mutms/tool_mulib", "v5.2.1.01"); err != nil {
		t.Fatalf("add: %v", err)
	}

	dir := filepath.Join(root, "public", "admin", "tool", "mulib")

	if head := strings.TrimSpace(gitOut(t, dir, "rev-parse", "--abbrev-ref", "HEAD")); head != "HEAD" {
		t.Errorf("a pinned tag left the checkout on branch %q, want a detached HEAD", head)
	}

	if tag := strings.TrimSpace(gitOut(t, dir, "describe", "--tags")); tag != "v5.2.1.01" {
		t.Errorf("checked out %q, want v5.2.1.01", tag)
	}
}

func TestAddRefusesADirectoryThatIsNotAWorkspace(t *testing.T) {
	root := t.TempDir()

	err := addInto(t, root, t.TempDir(), "mutms/tool_mulib", "")
	if err == nil {
		t.Fatal("adding to a directory with no live recipe succeeded")
	}

	if !strings.Contains(err.Error(), config.LiveRecipeFile) {
		t.Errorf("the error does not say what is missing: %v", err)
	}
}

// TestAddStopsBeforeTouchingTheTree checks that a plugin which serves no branch
// of this workspace's Moodle is refused with nothing checked out — resolution
// happens first precisely so a failure leaves no debris.
func TestAddStopsBeforeTouchingTheTree(t *testing.T) {
	root, pluginsDir := addFixture(t)

	// This workspace is Moodle 502; the entry serves only 405.
	entry := `name: mutms/tool_elsewhere
title: serves another Moodle
relpath: public/admin/tool/elsewhere
source:
  git:
    remotes:
      origin: ` + liveRemoteOf(t, pluginsDir, "tool_mulib") + `
requirements:
  MOODLE_405_STABLE:
    mdlbranches: ["405"]
`

	path := filepath.Join(pluginsDir, "mutms", "tool_elsewhere.yaml")

	if err := os.WriteFile(path, []byte(entry), 0o644); err != nil {
		t.Fatal(err)
	}

	err := addInto(t, root, pluginsDir, "mutms/tool_elsewhere", "")
	if err == nil {
		t.Fatal("a plugin with no branch for this Moodle was accepted")
	}

	if !strings.Contains(err.Error(), "502") {
		t.Errorf("the error does not say which Moodle branch went unserved: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "public", "admin", "tool", "elsewhere")); !os.IsNotExist(err) {
		t.Errorf("the plugin was checked out even though it could not be resolved")
	}

	if got := len(liveOf(t, root).Plugins); got != 0 {
		t.Errorf("recorded %d plugin(s) after a failed add, want 0", got)
	}
}

// liveRemoteOf digs the origin URL back out of a catalogue file, so a test can
// point a second entry at the same repository.
func liveRemoteOf(t *testing.T, pluginsDir string, pkg string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(pluginsDir, "mutms", pkg+".yaml"))
	if err != nil {
		t.Fatal(err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "origin:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "origin:"))
		}
	}

	t.Fatalf("no origin remote in the catalogue entry for %s", pkg)

	return ""
}
