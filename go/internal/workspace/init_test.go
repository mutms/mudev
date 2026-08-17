package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutms/mudev/go/internal/config"
)

// initPluginRemote builds a plugin remote whose version.php declares a
// frankenstyle component, the way `recipe init` expects to read one back.
func initPluginRemote(t *testing.T, dir string, branch string, component string) {
	t.Helper()

	initRepo(t, dir, "main")

	version := "<?php\n$plugin->version = 2026060550;\n$plugin->component = '" + component + "';\n"

	if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte(version), 0o644); err != nil {
		t.Fatal(err)
	}

	runGit(t, dir, "add", "--all")
	runGit(t, dir, "commit", "--quiet", "--message", "plugin")
	runGit(t, dir, "branch", branch)
}

// initFixture assembles a real workspace with `clone`, then strips the state a
// fresh checkout would not have — the live recipe and mudev's excludes — so what
// is left looks like a tree assembled by hand or by the old PHP tool, ready for
// `recipe init` to reconstruct.
//
// The plugin remote lives under an "acme" directory so its owner segment is
// deterministic, and it carries the component mod_thing, so the identifier
// reconstruction should land on acme/mod_thing.
func initFixture(t *testing.T) (root string) {
	t.Helper()

	base := t.TempDir()

	core := filepath.Join(base, "remote-core")
	pluginRemote := filepath.Join(base, "acme", "mod_thing")

	fakeCore(t, core, "502")
	initPluginRemote(t, pluginRemote, "MOODLE_500_STABLE", "mod_thing")

	recipePath := filepath.Join(base, "recipe.yaml")

	recipe := `name: seed
base:
  mdlbranch: "502"
  source:
    git:
      remotes:
        origin: ` + core + `
      ref: origin/patch/mutms/MOODLE_502_STABLE
  localbranch: MOODLE_502_STABLE
plugins:
  - name: acme/mod_thing
    relpath: public/mod/thing
    source:
      git:
        remotes:
          origin: ` + pluginRemote + `
        ref: origin/MOODLE_500_STABLE
`

	if err := os.WriteFile(recipePath, []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}

	root = filepath.Join(base, "workspace")

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := cloneInto(t, recipePath, root); err != nil {
		t.Fatalf("seed clone: %v", err)
	}

	// Make it look like a tree mudev never touched: no live recipe, and none of
	// the excludes clone leaves behind.
	if err := os.Remove(LivePath(root)); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(root, ".git", "info", "exclude")); err != nil {
		t.Fatal(err)
	}

	return root
}

func initInto(t *testing.T, root string) (string, error) {
	t.Helper()

	var out strings.Builder

	err := Init(context.Background(), InitOptions{
		Config: config.Defaults(),
		Root:   root,
		Out:    &out,
	})

	return out.String(), err
}

func TestInitReconstructsFromCheckouts(t *testing.T) {
	root := initFixture(t)

	if _, err := initInto(t, root); err != nil {
		t.Fatalf("Init: %v", err)
	}

	live, err := LoadLive(root)
	if err != nil {
		t.Fatalf("the reconstructed recipe must validate against the schema: %v", err)
	}

	// Base: read from the core checkout, including the localbranch override,
	// since the local branch differs from the ref's branch part.
	base := live.Base["source"].(map[string]any)["git"].(map[string]any)

	if base["ref"] != "origin/patch/mutms/MOODLE_502_STABLE" {
		t.Errorf("base ref = %v", base["ref"])
	}

	if live.Base["localbranch"] != "MOODLE_502_STABLE" {
		t.Errorf("base localbranch = %v", live.Base["localbranch"])
	}

	if live.Base["mdlbranch"] != "502" {
		t.Errorf("mdlbranch = %v", live.Base["mdlbranch"])
	}

	// The plugin is identified <owner>/<component> from its origin URL and its
	// own version.php, and recorded at the path it sits at.
	entry, ok := live.Plugin("acme/mod_thing")
	if !ok {
		t.Fatalf("plugin not identified as acme/mod_thing: %+v", live.Plugins)
	}

	if entry["relpath"] != "public/mod/thing" {
		t.Errorf("relpath = %v", entry["relpath"])
	}

	git := entry["source"].(map[string]any)["git"].(map[string]any)

	if git["ref"] != "origin/MOODLE_500_STABLE" {
		t.Errorf("plugin ref = %v", git["ref"])
	}

	// The local branch matches the ref's branch, so the default covers it and no
	// localbranch is written.
	if _, written := entry["localbranch"]; written {
		t.Errorf("localbranch recorded needlessly: %v", entry["localbranch"])
	}

	// The tree is now a managed workspace: its excludes are back.
	exclude, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf("read exclude: %v", err)
	}

	for _, want := range []string{"/" + config.LiveRecipeFile, "/public/mod/thing"} {
		if !strings.Contains(string(exclude), want) {
			t.Errorf("exclude missing %q:\n%s", want, exclude)
		}
	}
}

func TestInitRefusesToOverwriteALiveRecipe(t *testing.T) {
	root := initFixture(t)

	if _, err := initInto(t, root); err != nil {
		t.Fatalf("first Init: %v", err)
	}

	_, err := initInto(t, root)
	if err == nil {
		t.Fatal("a second Init must refuse rather than clobber the record")
	}

	if !strings.Contains(err.Error(), config.LiveRecipeFile) {
		t.Errorf("the error should name the file it refuses to overwrite: %v", err)
	}
}

func TestInitRefusesATreeThatIsNotMoodle(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "notmoodle")

	initRepo(t, root, "main")

	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	runGit(t, root, "add", "--all")
	runGit(t, root, "commit", "--quiet", "--message", "not moodle")

	_, err := initInto(t, root)
	if err == nil {
		t.Fatal("a tree with no Moodle version.php must be refused")
	}

	if !strings.Contains(err.Error(), "not a Moodle core tree") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestInitSkipsACheckoutWithoutOrigin checks the honesty rule: a checkout mudev
// cannot name (no origin to derive an owner from) is reported and left out,
// while the rest of the tree is still reconstructed.
func TestInitSkipsACheckoutWithoutOrigin(t *testing.T) {
	root := initFixture(t)

	// Turn the plugin's origin into a differently-named remote: now it has
	// remotes, but none called origin.
	plugin := filepath.Join(root, "public", "mod", "thing")
	runGit(t, plugin, "remote", "rename", "origin", "somewhere")

	out, err := initInto(t, root)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	if !strings.Contains(out, "no origin remote") {
		t.Errorf("the skipped checkout was not reported:\n%s", out)
	}

	live, err := LoadLive(root)
	if err != nil {
		t.Fatalf("LoadLive: %v", err)
	}

	if len(live.Plugins) != 0 {
		t.Errorf("a checkout with no origin should not be recorded: %+v", live.Plugins)
	}
}

func TestRemoteOwnerAndRepo(t *testing.T) {
	cases := []struct {
		url   string
		owner string
		repo  string
	}{
		{"git@github.com:mutms/moodle-tool_mulib.git", "mutms", "tool_mulib"},
		{"git@forge.example.test:acme/mod_thing.git", "acme", "mod_thing"},
		{"https://github.com/moodlehq/moodle-mod_customcert.git", "moodlehq", "mod_customcert"},
		{"ssh://git@host:22/group/sub/repo.git", "sub", "repo"},
		{"file:///srv/repos/acme/tool_x.git", "acme", "tool_x"},
		{"/srv/repos/acme/tool_x", "acme", "tool_x"},
	}

	for _, tc := range cases {
		if got := remoteOwner(tc.url); got != tc.owner {
			t.Errorf("remoteOwner(%q) = %q, want %q", tc.url, got, tc.owner)
		}

		if got := remoteRepo(tc.url); got != tc.repo {
			t.Errorf("remoteRepo(%q) = %q, want %q", tc.url, got, tc.repo)
		}
	}
}

func TestReconstructName(t *testing.T) {
	// Component present: owner + component.
	name, err := reconstructName("mutms", "tool_mulib", "git@github.com:mutms/moodle-tool_mulib.git")
	if err != nil || name != "mutms/tool_mulib" {
		t.Errorf("reconstructName with component = %q, %v", name, err)
	}

	// No component: fall back to the repository name (minus the moodle- prefix).
	name, err = reconstructName("moodlehq", "", "https://github.com/moodlehq/moodle-mod_customcert.git")
	if err != nil || name != "moodlehq/mod_customcert" {
		t.Errorf("reconstructName fallback = %q, %v", name, err)
	}

	// No owner: a neutral vendor rather than a guess.
	name, err = reconstructName("", "local_thing", "thing")
	if err != nil || name != "local/local_thing" {
		t.Errorf("reconstructName without owner = %q, %v", name, err)
	}
}

func TestBranchOf(t *testing.T) {
	remotes := map[string]string{"origin": "x", "upstream": "y"}

	if got := branchOf("origin/MOODLE_502_STABLE", remotes); got != "MOODLE_502_STABLE" {
		t.Errorf("branchOf branch = %q", got)
	}

	if got := branchOf("upstream/patch/mutms/MOODLE_502_STABLE", remotes); got != "patch/mutms/MOODLE_502_STABLE" {
		t.Errorf("branchOf slashed branch = %q", got)
	}

	// A tag or commit names no branch.
	if got := branchOf("v5.2.1.01", remotes); got != "" {
		t.Errorf("branchOf tag = %q, want empty", got)
	}
}
