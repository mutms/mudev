package workspace

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutms/mudev/go/internal/config"
	"github.com/mutms/mudev/go/internal/exec"
)

// runGit runs a git command in dir, failing the test if it does not succeed.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	res, err := exec.Capture(context.Background(), exec.Cmd{Name: "git", Args: args, Dir: dir})
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}

	if res.Failed() {
		t.Fatalf("git %v: %s", args, res.Stderr)
	}
}

// gitOut runs a git command and returns its output.
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()

	res, err := exec.Capture(context.Background(), exec.Cmd{Name: "git", Args: args, Dir: dir})
	if err != nil || res.Failed() {
		t.Fatalf("git %v: %v %s", args, err, res.Stderr)
	}

	return res.Stdout
}

// initRepo creates a repository that can commit without the developer's global
// git config (or their signing key, which a test does not have).
func initRepo(t *testing.T, dir string, defaultBranch string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	runGit(t, dir, "init", "--quiet", "--initial-branch="+defaultBranch)
	runGit(t, dir, "config", "user.email", "test@example.org")
	runGit(t, dir, "config", "user.name", "mudev test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
}

// fakeCore builds a repository that looks enough like a Moodle code tree to
// assemble against: a public/ code root with a version.php declaring $branch.
//
// Its default branch is deliberately NOT the branch the recipe wants, so any
// checkout that goes via the remote's default would leave that branch behind
// for the test to catch.
func fakeCore(t *testing.T, dir string, mdlbranch string) {
	t.Helper()

	initRepo(t, dir, "main")

	public := filepath.Join(dir, "public")

	if err := os.MkdirAll(filepath.Join(public, "admin", "tool"), 0o755); err != nil {
		t.Fatal(err)
	}

	version := "<?php\n$version = 2026042001.00;\n$release = '5.2.1';\n$branch = '" + mdlbranch + "';\n"

	if err := os.WriteFile(filepath.Join(public, "version.php"), []byte(version), 0o644); err != nil {
		t.Fatal(err)
	}

	runGit(t, dir, "add", "--all")
	runGit(t, dir, "commit", "--quiet", "--message", "moodle core")
	runGit(t, dir, "switch", "--quiet", "--create", "patch/mutms/MOODLE_502_STABLE")
	runGit(t, dir, "commit", "--quiet", "--allow-empty", "--message", "mutms patches")
	// Leave HEAD on the default branch, the way a real remote presents itself.
	runGit(t, dir, "switch", "--quiet", "main")
}

// fakePlugin builds a plugin repository whose default branch is, again, not
// the one a recipe asks for.
func fakePlugin(t *testing.T, dir string, branch string) {
	t.Helper()

	initRepo(t, dir, "main")

	if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte("<?php\n$plugin->version = 2026060550;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runGit(t, dir, "add", "--all")
	runGit(t, dir, "commit", "--quiet", "--message", "plugin")
	runGit(t, dir, "branch", branch)
	runGit(t, dir, "tag", "v5.2.1.01")
}

// writeRecipe writes a recipe with the plugin defined inline, so the test needs
// no catalogue on disk.
func writeRecipe(t *testing.T, path string, core string, plugin string, mdlbranch string) {
	t.Helper()

	recipe := `name: test recipe
base:
  mdlbranch: "` + mdlbranch + `"
  source:
    git:
      remotes:
        origin: ` + core + `
      ref: origin/patch/mutms/MOODLE_502_STABLE
  localbranch: MOODLE_502_STABLE
plugins:
  - name: mutms/tool_mulib
    title: MuTMS shared library
    relpath: public/admin/tool/mulib
    requirements:
      MOODLE_500_STABLE:
        mdlbranches: ["502"]
    source:
      git:
        remotes:
          origin: ` + plugin + `
`

	if err := os.WriteFile(path, []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}
}

// workspaceFixture assembles core + plugin repositories and a recipe, and
// returns the recipe path and an empty workspace root to clone into.
func workspaceFixture(t *testing.T, mdlbranch string) (recipePath string, root string) {
	t.Helper()

	base := t.TempDir()

	core := filepath.Join(base, "remote-core")
	plugin := filepath.Join(base, "remote-plugin")

	fakeCore(t, core, "502")
	fakePlugin(t, plugin, "MOODLE_500_STABLE")

	recipePath = filepath.Join(base, "recipe.yaml")
	writeRecipe(t, recipePath, core, plugin, mdlbranch)

	root = filepath.Join(base, "workspace")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	return recipePath, root
}

func cloneInto(t *testing.T, recipePath string, root string) error {
	t.Helper()

	return Clone(context.Background(), Options{
		Config: config.Defaults(),
		Recipe: recipePath,
		Root:   root,
		Out:    io.Discard,
	})
}

func TestCloneChecksOutOnlyTheExpectedBranch(t *testing.T) {
	recipePath, root := workspaceFixture(t, "502")

	if err := cloneInto(t, recipePath, root); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Cloning would check out the remote's default branch first and leave it
	// behind. Each checkout must hold exactly the branch the recipe named.
	cases := map[string]string{
		".":                       "MOODLE_502_STABLE",
		"public/admin/tool/mulib": "MOODLE_500_STABLE",
	}

	for path, want := range cases {
		branches := gitOut(t, filepath.Join(root, path), "branch", "--format=%(refname:short)")

		if branches != want {
			t.Errorf("%s has branches %q, want only %q", path, branches, want)
		}
	}

	// The live recipe records the assembled tree.
	live, err := LoadLive(root)
	if err != nil {
		t.Fatalf("LoadLive: %v", err)
	}

	if _, ok := live.Plugin("mutms/tool_mulib"); !ok {
		t.Errorf("plugin not recorded: %+v", live.Plugins)
	}
}

func TestCloneCompletesAnInterruptedCore(t *testing.T) {
	recipePath, root := workspaceFixture(t, "502")

	if err := cloneInto(t, recipePath, root); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Reproduce the state an interrupted first run leaves: core has a .git
	// directory but no commit, while the plugins are already in the tree. The
	// tell-tale is that a re-run must NOT accept this as "already checked out".
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(root, "public", "version.php")); err != nil {
		t.Fatal(err)
	}

	initRepo(t, root, "main")

	if err := cloneInto(t, recipePath, root); err != nil {
		t.Fatalf("Clone should have completed the core checkout: %v", err)
	}

	// Core is real again — checked out into a tree that already held plugins.
	if _, err := os.Stat(filepath.Join(root, "public", "version.php")); err != nil {
		t.Errorf("core was not checked out: %v", err)
	}

	branches := gitOut(t, root, "branch", "--format=%(refname:short)")

	if branches != "MOODLE_502_STABLE" {
		t.Errorf("core branches = %q", branches)
	}
}

func TestCloneRefusesAMoodleThatIsNotTheRecipesMoodle(t *testing.T) {
	// The recipe claims 405 while the core tree says 502. Every plugin path and
	// branch is derived from that claim, so it has to be fatal.
	recipePath, root := workspaceFixture(t, "405")

	err := cloneInto(t, recipePath, root)
	if err == nil {
		t.Fatal("expected a mismatched Moodle branch to stop the run")
	}

	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "405") {
		t.Errorf("the error should name both branches: %v", err)
	}

	// Nothing was assembled on top of the wrong core.
	if _, statErr := os.Stat(filepath.Join(root, "public", "admin", "tool", "mulib")); statErr == nil {
		t.Error("plugins must not be installed onto an unverified core")
	}
}

func TestCloneRefusesATreeThatIsNotMoodle(t *testing.T) {
	recipePath, root := workspaceFixture(t, "502")

	// A core repository with no version.php at all: not a Moodle code tree.
	base := filepath.Dir(root)
	core := filepath.Join(base, "remote-core")

	runGit(t, core, "switch", "--quiet", "patch/mutms/MOODLE_502_STABLE")
	runGit(t, core, "rm", "--quiet", "public/version.php")
	runGit(t, core, "commit", "--quiet", "--message", "not moodle any more")
	runGit(t, core, "switch", "--quiet", "main")

	err := cloneInto(t, recipePath, root)
	if err == nil {
		t.Fatal("expected a tree without version.php to stop the run")
	}

	if !strings.Contains(err.Error(), "public/version.php") {
		t.Errorf("the error should name the file it looked for: %v", err)
	}
}

func TestCloneIsIdempotent(t *testing.T) {
	recipePath, root := workspaceFixture(t, "502")

	if err := cloneInto(t, recipePath, root); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// A developer's own branch must survive a re-run untouched.
	plugin := filepath.Join(root, "public", "admin", "tool", "mulib")
	runGit(t, plugin, "switch", "--quiet", "--create", "MDL-1234-fix")

	if err := cloneInto(t, recipePath, root); err != nil {
		t.Fatalf("second Clone: %v", err)
	}

	if head := gitOut(t, plugin, "branch", "--show-current"); head != "MDL-1234-fix" {
		t.Errorf("a re-run moved the working tree to %q", head)
	}
}

// TestCloneFetchesEveryRemoteInOrder checks the mirror-priming arrangement: a
// recipe naming a fast local mirror before origin must fetch both, mirror
// first, so that origin is left with only the difference to send.
func TestCloneFetchesEveryRemoteInOrder(t *testing.T) {
	base := t.TempDir()

	core := filepath.Join(base, "remote-core")
	mirror := filepath.Join(base, "mirror-core")

	fakeCore(t, core, "502")

	// The mirror is a copy of core, the way a local forgejo instance mirrors
	// what lives on github.
	runGit(t, base, "clone", "--quiet", "--mirror", core, mirror)

	recipePath := filepath.Join(base, "recipe.yaml")

	recipe := `name: mirrored
extra:
  mudev: {fetch_order: [backup, origin]}
base:
  mdlbranch: "502"
  source:
    git:
      remotes:
        origin: ` + core + `
        backup: ` + mirror + `
      ref: origin/patch/mutms/MOODLE_502_STABLE
  localbranch: MOODLE_502_STABLE
plugins: []
`

	if err := os.WriteFile(recipePath, []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(base, "workspace")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := cloneInto(t, recipePath, root); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Both remotes were fetched, not just origin: the mirror's refs are here.
	refs := gitOut(t, root, "for-each-ref", "--format=%(refname)", "refs/remotes")

	for _, want := range []string{"refs/remotes/backup/", "refs/remotes/origin/"} {
		if !strings.Contains(refs, want) {
			t.Errorf("no %s refs — that remote was never fetched:\n%s", want, refs)
		}
	}
}

// TestCloneSurvivesAnUnreachableMirror checks the failure policy: a mirror that
// is down must not stop an assembly that origin can serve perfectly well.
func TestCloneSurvivesAnUnreachableMirror(t *testing.T) {
	base := t.TempDir()

	core := filepath.Join(base, "remote-core")
	fakeCore(t, core, "502")

	recipePath := filepath.Join(base, "recipe.yaml")

	recipe := `name: mirror down
extra:
  mudev: {fetch_order: [backup, origin]}
base:
  mdlbranch: "502"
  source:
    git:
      remotes:
        origin: ` + core + `
        backup: ` + filepath.Join(base, "nowhere") + `
      ref: origin/patch/mutms/MOODLE_502_STABLE
  localbranch: MOODLE_502_STABLE
plugins: []
`

	if err := os.WriteFile(recipePath, []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(base, "workspace")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder

	err := Clone(context.Background(), Options{
		Config: config.Defaults(),
		Recipe: recipePath,
		Root:   root,
		Out:    &out,
	})
	if err != nil {
		t.Fatalf("an unreachable mirror must not fail the assembly: %v", err)
	}

	// It must be reported, though — a mirror silently not being fetched is how
	// a backup quietly stops being one.
	if !strings.Contains(out.String(), "could not fetch remote backup") {
		t.Errorf("the unreachable mirror was not reported:\n%s", out.String())
	}

	if _, err := os.Stat(filepath.Join(root, "public", "version.php")); err != nil {
		t.Errorf("core was not checked out from origin: %v", err)
	}
}

// TestCloneStopsWhenOriginIsUnreachable is the other half of that policy:
// origin is load-bearing, so its failure is fatal.
func TestCloneStopsWhenOriginIsUnreachable(t *testing.T) {
	base := t.TempDir()

	recipePath := filepath.Join(base, "recipe.yaml")

	recipe := `name: origin down
base:
  mdlbranch: "502"
  source:
    git:
      remotes:
        origin: ` + filepath.Join(base, "nowhere") + `
      ref: origin/patch/mutms/MOODLE_502_STABLE
plugins: []
`

	if err := os.WriteFile(recipePath, []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(base, "workspace")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := cloneInto(t, recipePath, root); err == nil {
		t.Fatal("an unreachable origin must stop the run")
	}
}

// TestCloneReportsWhatItIsDoing checks the progress output itself. git's
// transfer progress says nothing about which recipe, which Moodle or which
// remote a developer is waiting for, and a fetch of a million objects from a
// LAN mirror looks exactly like one from the other side of the internet.
func TestCloneReportsWhatItIsDoing(t *testing.T) {
	base := t.TempDir()

	core := filepath.Join(base, "remote-core")
	mirror := filepath.Join(base, "mirror-core")

	fakeCore(t, core, "502")
	runGit(t, base, "clone", "--quiet", "--mirror", core, mirror)

	recipePath := filepath.Join(base, "recipe.yaml")

	recipe := `name: Narrated recipe
extra:
  mudev: {release: mutms, fetch_order: [backup, origin]}
base:
  mdlbranch: "502"
  source:
    git:
      remotes:
        origin: ` + core + `
        backup: ` + mirror + `
      ref: origin/patch/mutms/MOODLE_502_STABLE
  localbranch: MOODLE_502_STABLE
plugins: []
`

	if err := os.WriteFile(recipePath, []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(base, "workspace")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder

	err := Clone(context.Background(), Options{
		Config: config.Defaults(),
		Recipe: recipePath,
		Root:   root,
		Out:    &out,
	})
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	got := out.String()

	want := []string{
		"Narrated recipe",              // which recipe
		"branch 502",                   // which Moodle
		"release:     mutms",           // whether this tree can be tagged
		"fetch order: backup, origin",  // who gets asked first
		"fetch backup (1/2) " + mirror, // …and that it really was asked first
		"fetch origin (2/2) " + core,
		"checkout MOODLE_502_STABLE tracking origin/patch/mutms/MOODLE_502_STABLE",
	}

	for _, line := range want {
		if !strings.Contains(got, line) {
			t.Errorf("output does not report %q:\n%s", line, got)
		}
	}

	// The mirror must be named before origin in the output, not just present.
	if strings.Index(got, "fetch backup") > strings.Index(got, "fetch origin") {
		t.Errorf("the mirror was not fetched first:\n%s", got)
	}

	// A re-run says it is continuing rather than starting.
	out.Reset()

	if err := Clone(context.Background(), Options{
		Config: config.Defaults(),
		Recipe: recipePath,
		Root:   root,
		Out:    &out,
	}); err != nil {
		t.Fatalf("second Clone: %v", err)
	}

	if !strings.Contains(out.String(), "continuing the workspace") {
		t.Errorf("a re-run should say so:\n%s", out.String())
	}
}
