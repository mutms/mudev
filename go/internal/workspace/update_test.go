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

// updatedWorkspace assembles a tree and reconstructs its live recipe with Init,
// leaving a managed workspace for update tests to poke at. The plugin lands at
// public/mod/thing, recorded as acme/mod_thing.
func updatedWorkspace(t *testing.T) string {
	t.Helper()

	root := initFixture(t)

	if err := Init(context.Background(), InitOptions{Config: config.Defaults(), Root: root, Out: io.Discard}); err != nil {
		t.Fatalf("seed init: %v", err)
	}

	return root
}

func updateInto(t *testing.T, root string, relpath string) (string, error) {
	t.Helper()

	var out strings.Builder

	err := Update(context.Background(), UpdateOptions{
		Config:  config.Defaults(),
		Root:    root,
		Relpath: relpath,
		Out:     &out,
	})

	return out.String(), err
}

func TestUpdateRefreshKeepsNameAndMetadata(t *testing.T) {
	root := updatedWorkspace(t)

	// Simulate a workspace whose entry was edited after init: the name was
	// changed and a field was added by hand. update must preserve both.
	live, err := LoadLive(root)
	if err != nil {
		t.Fatal(err)
	}

	entry, _ := live.Plugin("acme/mod_thing")
	entry["name"] = "acme/renamed"
	entry["title"] = "Hand written"

	if err := live.Save(root); err != nil {
		t.Fatal(err)
	}

	// The checkout gains a second remote — a git-identity change update records.
	plugin := filepath.Join(root, "public", "mod", "thing")
	runGit(t, plugin, "remote", "add", "backup", "git@mirror.example:acme/mod_thing.git")

	out, err := updateInto(t, root, "public/mod/thing")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !strings.Contains(out, "acme/renamed") {
		t.Errorf("update should report the recorded (renamed) entry:\n%s", out)
	}

	reloaded, err := LoadLive(root)
	if err != nil {
		t.Fatal(err)
	}

	refreshed, ok := reloaded.Plugin("acme/renamed")
	if !ok {
		t.Fatalf("the hand-set name was lost: %+v", reloaded.Plugins)
	}

	if refreshed["title"] != "Hand written" {
		t.Errorf("hand-set metadata lost: %v", refreshed["title"])
	}

	remotes := refreshed["source"].(map[string]any)["git"].(map[string]any)["remotes"].(map[string]any)

	if remotes["backup"] != "git@mirror.example:acme/mod_thing.git" {
		t.Errorf("the new remote was not recorded: %v", remotes)
	}
}

func TestUpdateRecordsAMovedBranch(t *testing.T) {
	root := updatedWorkspace(t)

	// Move the checkout onto a feature branch — the `≠` a listing would show.
	plugin := filepath.Join(root, "public", "mod", "thing")
	runGit(t, plugin, "switch", "--quiet", "--create", "MDL-1-fix")

	if _, err := updateInto(t, root, "public/mod/thing"); err != nil {
		t.Fatalf("Update: %v", err)
	}

	live, err := LoadLive(root)
	if err != nil {
		t.Fatal(err)
	}

	entry, _ := live.Plugin("acme/mod_thing")
	ref := entry["source"].(map[string]any)["git"].(map[string]any)["ref"].(string)

	// A local branch with no upstream cannot be spelled against a remote, so its
	// commit is what gets recorded.
	head := strings.TrimSpace(gitOut(t, plugin, "rev-parse", "HEAD"))

	if ref != head {
		t.Errorf("moved branch recorded as %q, want the commit %q", ref, head)
	}
}

func TestUpdateAdoptsANewCheckout(t *testing.T) {
	root := updatedWorkspace(t)

	// A plugin cloned into the tree after init: update should adopt it.
	dir := filepath.Join(root, "public", "local", "extra")
	initRepo(t, dir, "main")

	version := "<?php\n$plugin->version = 2026010100;\n$plugin->component = 'local_extra';\n"
	if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte(version), 0o644); err != nil {
		t.Fatal(err)
	}

	runGit(t, dir, "remote", "add", "origin", "git@forge.example:acme/local_extra.git")
	runGit(t, dir, "add", "--all")
	runGit(t, dir, "commit", "--quiet", "--message", "extra")

	out, err := updateInto(t, root, "public/local/extra")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !strings.Contains(out, "adopted acme/local_extra") {
		t.Errorf("adoption not reported:\n%s", out)
	}

	live, err := LoadLive(root)
	if err != nil {
		t.Fatal(err)
	}

	entry, ok := live.Plugin("acme/local_extra")
	if !ok {
		t.Fatalf("new checkout not adopted: %+v", live.Plugins)
	}

	if entry["relpath"] != "public/local/extra" {
		t.Errorf("relpath = %v", entry["relpath"])
	}

	// Adoption hides it from the containing repository, like clone and init.
	exclude, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(exclude), "/public/local/extra") {
		t.Errorf("adopted checkout not excluded:\n%s", exclude)
	}
}

func TestUpdateRefreshesCore(t *testing.T) {
	root := updatedWorkspace(t)

	runGit(t, root, "remote", "add", "backup", "git@mirror.example:msk/moodle.git")

	if _, err := updateInto(t, root, "."); err != nil {
		t.Fatalf("Update core: %v", err)
	}

	live, err := LoadLive(root)
	if err != nil {
		t.Fatal(err)
	}

	remotes := live.Base["source"].(map[string]any)["git"].(map[string]any)["remotes"].(map[string]any)

	if remotes["backup"] != "git@mirror.example:msk/moodle.git" {
		t.Errorf("core remote not recorded: %v", remotes)
	}
}

func TestUpdateIsANoOpWhenNothingChanged(t *testing.T) {
	root := updatedWorkspace(t)

	before, err := os.ReadFile(LivePath(root))
	if err != nil {
		t.Fatal(err)
	}

	out, err := updateInto(t, root, "public/mod/thing")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if !strings.Contains(out, "already matches") {
		t.Errorf("an unchanged checkout should say so:\n%s", out)
	}

	after, err := os.ReadFile(LivePath(root))
	if err != nil {
		t.Fatal(err)
	}

	// Nothing changed, so the file is not rewritten.
	if string(before) != string(after) {
		t.Error("the live recipe was rewritten for a no-op update")
	}
}

func TestUpdateReportsARemovedPluginAsPrune(t *testing.T) {
	root := updatedWorkspace(t)

	if err := os.RemoveAll(filepath.Join(root, "public", "mod", "thing")); err != nil {
		t.Fatal(err)
	}

	_, err := updateInto(t, root, "public/mod/thing")
	if err == nil {
		t.Fatal("a recorded-but-gone checkout should be refused")
	}

	if !strings.Contains(err.Error(), "prune") {
		t.Errorf("the error should point at prune: %v", err)
	}
}

func TestUpdateRefusesAnUninitialisedWorkspace(t *testing.T) {
	_, err := updateInto(t, t.TempDir(), "public/mod/thing")
	if err == nil {
		t.Fatal("update needs a live recipe to update")
	}

	if !strings.Contains(err.Error(), "recipe init") {
		t.Errorf("the error should point at init: %v", err)
	}
}

func TestCleanRelpath(t *testing.T) {
	ok := map[string]string{
		".":              ".",
		"./public/mod/x": "public/mod/x",
		"public/mod/x/":  "public/mod/x",
		"public/mod/x":   "public/mod/x",
	}

	for in, want := range ok {
		got, err := cleanRelpath(in)
		if err != nil || got != want {
			t.Errorf("cleanRelpath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	for _, bad := range []string{"", "  ", "../etc", "../../x"} {
		if _, err := cleanRelpath(bad); err == nil {
			t.Errorf("cleanRelpath(%q) should have failed", bad)
		}
	}
}
