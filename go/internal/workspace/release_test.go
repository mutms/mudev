package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// releasePlugin builds a plugin repository with the given version.php on
// branch, and leaves its default branch elsewhere.
func releasePlugin(t *testing.T, dir string, branch string, versionPHP string) {
	t.Helper()

	initRepo(t, dir, "main")

	if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte(versionPHP), 0o644); err != nil {
		t.Fatal(err)
	}

	runGit(t, dir, "add", "--all")
	runGit(t, dir, "commit", "--quiet", "--message", "plugin")
	runGit(t, dir, "branch", branch)
}

const (
	mulibPHP = `<?php
$plugin->component = 'tool_mulib';
$plugin->version   = 2026103053;
$plugin->requires  = 2026100600;
`

	muprogPHP = `<?php
$plugin->component = 'tool_muprog';
$plugin->version   = 2026103053;
$plugin->dependencies = [
    'tool_mulib'   => 2026103053,
    'tool_mutrain' => 2026050150,
];
`

	mutrainPHP = `<?php
$plugin->component = 'tool_mutrain';
$plugin->version   = 2026050150;
`

	certificatePHP = `<?php
$plugin->component = 'tool_certificate';
$plugin->version   = 2026010100;
`
)

// releaseFixture clones a MuTMS-style workspace: two release-managed plugins
// on the mdlbranch's development branch, one release-managed plugin left on an
// older shared branch, and one plugin that is not release-managed at all.
func releaseFixture(t *testing.T, mdlbranch string, flavour string) string {
	t.Helper()

	base := t.TempDir()

	core := filepath.Join(base, "remote-core")
	fakeCore(t, core, mdlbranch)

	devBranch := "MOODLE_" + mdlbranch + "_DEV"

	plugins := []struct {
		component string
		branch    string
		php       string
		release   bool
	}{
		{"tool_mulib", devBranch, mulibPHP, true},
		{"tool_muprog", devBranch, muprogPHP, true},
		{"tool_mutrain", "MOODLE_500_STABLE", mutrainPHP, true},
		{"tool_certificate", devBranch, certificatePHP, false},
	}

	extra := ""
	if flavour != "" {
		extra = "extra:\n  mudev: {release: " + flavour + "}\n"
	}

	recipe := "name: release test\n" + extra + `base:
  mdlbranch: "` + mdlbranch + `"
  source:
    git:
      remotes:
        origin: ` + core + `
      ref: origin/patch/mutms/MOODLE_502_STABLE
plugins:
`

	for _, p := range plugins {
		remote := filepath.Join(base, "remote-"+p.component)
		releasePlugin(t, remote, p.branch, p.php)

		recipe += "  - name: mutms/" + p.component + `
    relpath: public/admin/tool/` + strings.TrimPrefix(p.component, "tool_") + `
    source:
      git:
        remotes:
          origin: ` + remote + `
        ref: origin/` + p.branch + "\n"

		if p.release {
			recipe += "    extra: {mudev: {release: mutms}}\n"
		}
	}

	recipePath := filepath.Join(base, "recipe.yaml")

	if err := os.WriteFile(recipePath, []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(base, "workspace")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := cloneInto(t, recipePath, root); err != nil {
		t.Fatalf("clone: %v", err)
	}

	return root
}

func setVersionIn(t *testing.T, root string, value string) (string, error) {
	t.Helper()

	var out strings.Builder

	err := SetVersion(context.Background(), SetVersionOptions{
		Config: testConfig(),
		Root:   root,
		Value:  value,
		Now: func() time.Time {
			return time.Date(2026, time.November, 5, 10, 0, 0, 0, time.Local)
		},
		Out: &out,
	})

	return out.String(), err
}

func versionPHPOf(t *testing.T, root string, plugin string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, "public", "admin", "tool", plugin, "version.php"))
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}

func TestSetVersionBumpsReleasePlugins(t *testing.T) {
	root := releaseFixture(t, "503", "mutms")

	out, err := setVersionIn(t, root, "2026110553")
	if err != nil {
		t.Fatalf("SetVersion: %v\n%s", err, out)
	}

	if got := versionPHPOf(t, root, "mulib"); got != strings.Replace(mulibPHP, "2026103053", "2026110553", 1) {
		t.Errorf("tool_mulib version.php:\n%s", got)
	}

	// The dependency on a plugin released alongside follows; the one on a
	// plugin left on an older branch does not.
	wantProg := `<?php
$plugin->component = 'tool_muprog';
$plugin->version   = 2026110553;
$plugin->dependencies = [
    'tool_mulib'   => 2026110553,
    'tool_mutrain' => 2026050150,
];
`
	if got := versionPHPOf(t, root, "muprog"); got != wantProg {
		t.Errorf("tool_muprog version.php:\n%s", got)
	}

	if got := versionPHPOf(t, root, "mutrain"); got != mutrainPHP {
		t.Errorf("a plugin on an older branch must not change:\n%s", got)
	}

	if got := versionPHPOf(t, root, "certificate"); got != certificatePHP {
		t.Errorf("a plugin that is not release-managed must not change:\n%s", got)
	}

	for _, want := range []string{
		"version 2026103053 → 2026110553",
		"dependency tool_mulib 2026103053 → 2026110553",
		"skipped: branch MOODLE_500_STABLE is not a 503 branch",
		"version 2026110553 set in 2 plugin(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output does not report %q:\n%s", want, out)
		}
	}

	if strings.Contains(out, "certificate") {
		t.Errorf("a plugin that is not release-managed should not be mentioned:\n%s", out)
	}

	// A second run finds nothing to do.
	out, err = setVersionIn(t, root, "2026110553")
	if err != nil {
		t.Fatalf("second SetVersion: %v", err)
	}

	if !strings.Contains(out, "2 plugin(s) already at 2026110553") {
		t.Errorf("a second run should only count:\n%s", out)
	}
}

func TestSetVersionToday(t *testing.T) {
	root := releaseFixture(t, "503", "mutms")

	if out, err := setVersionIn(t, root, "today"); err != nil {
		t.Fatalf("SetVersion: %v\n%s", err, out)
	}

	if got := versionPHPOf(t, root, "mulib"); !strings.Contains(got, "$plugin->version   = 2026110553;") {
		t.Errorf("today on 503 should be 2026110553:\n%s", got)
	}
}

func TestSetVersionRejectsAnotherBranchSuffix(t *testing.T) {
	root := releaseFixture(t, "503", "mutms")

	_, err := setVersionIn(t, root, "2026110552")
	if err == nil || !strings.Contains(err.Error(), "YYYYMMDD53") {
		t.Fatalf("a 502 stamp must be rejected on 503: %v", err)
	}

	if got := versionPHPOf(t, root, "mulib"); got != mulibPHP {
		t.Errorf("nothing may change:\n%s", got)
	}
}

func TestSetVersionRefusesDowngrades(t *testing.T) {
	root := releaseFixture(t, "503", "mutms")

	_, err := setVersionIn(t, root, "2026102953")
	if err == nil || !strings.Contains(err.Error(), "public/admin/tool/mulib is at 2026103053") {
		t.Fatalf("a downgrade must be refused, naming the plugin: %v", err)
	}

	if versionPHPOf(t, root, "mulib") != mulibPHP || versionPHPOf(t, root, "muprog") != muprogPHP {
		t.Error("a refused run must not write any file")
	}
}

func TestSetVersionSkipsAStrayedCheckout(t *testing.T) {
	root := releaseFixture(t, "503", "mutms")

	runGit(t, filepath.Join(root, "public", "admin", "tool", "mulib"), "switch", "--quiet", "--create", "MDL-1234-fix")

	out, err := setVersionIn(t, root, "2026110553")
	if err != nil {
		t.Fatalf("SetVersion: %v\n%s", err, out)
	}

	if !strings.Contains(out, "skipped: on branch MDL-1234-fix, the recipe expects MOODLE_503_DEV") {
		t.Errorf("the strayed checkout should be reported:\n%s", out)
	}

	if got := versionPHPOf(t, root, "mulib"); got != mulibPHP {
		t.Errorf("a strayed checkout must not change:\n%s", got)
	}

	// mulib is not part of this release, so the dependency on it stays.
	if got := versionPHPOf(t, root, "muprog"); !strings.Contains(got, "'tool_mulib'   => 2026103053") {
		t.Errorf("a dependency on a skipped plugin must not change:\n%s", got)
	}
}

func TestSetVersionNeedsAReleaseWorkspace(t *testing.T) {
	root := releaseFixture(t, "503", "")

	_, err := setVersionIn(t, root, "2026110553")
	if err == nil || !strings.Contains(err.Error(), "extra.mudev.release") {
		t.Fatalf("a workspace without a release flavour must be refused: %v", err)
	}
}

func TestSetVersionRefusesOlderMoodle(t *testing.T) {
	root := releaseFixture(t, "502", "mutms")

	_, err := setVersionIn(t, root, "2026110552")
	if err == nil || !strings.Contains(err.Error(), "Moodle 5.3 or later") {
		t.Fatalf("a 502 workspace must be refused: %v", err)
	}

	if got := versionPHPOf(t, root, "mulib"); got != mulibPHP {
		t.Errorf("nothing may change:\n%s", got)
	}
}
