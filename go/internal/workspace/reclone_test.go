package workspace

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutms/mudev/go/internal/config"
)

// recloneInto runs the reclone exactly as the command does.
func recloneInto(t *testing.T, root string, pluginsDir string, out io.Writer) error {
	t.Helper()

	cfg := testConfig()
	cfg.PluginsDir = pluginsDir

	return Reclone(context.Background(), RecloneOptions{
		Config: cfg,
		Root:   root,
		Out:    out,
	})
}

// recloneFixture is a workspace holding core and one plugin, tool_mulib.
func recloneFixture(t *testing.T) (root string, pluginsDir string, mulib string) {
	t.Helper()

	root, pluginsDir = addFixture(t)

	if err := addInto(t, root, pluginsDir, "mutms/tool_mulib", ""); err != nil {
		t.Fatalf("add: %v", err)
	}

	return root, pluginsDir, filepath.Join(root, "public", "admin", "tool", "mulib")
}

// editLive is the developer opening .mudev.json in an editor: it hands the
// decoded file to the test and writes back whatever the test made of it.
func editLive(t *testing.T, root string, edit func(live *Live)) {
	t.Helper()

	live := liveOf(t, root)

	edit(live)

	if err := live.Save(root); err != nil {
		t.Fatal(err)
	}
}

// sourceGitOf digs the source.git block out of a base or plugin definition.
func sourceGitOf(t *testing.T, definition map[string]any) map[string]any {
	t.Helper()

	source, _ := definition["source"].(map[string]any)
	block, _ := source["git"].(map[string]any)

	if block == nil {
		t.Fatalf("no source.git in %v", definition)
	}

	return block
}

// remotesOf digs the remotes out of a base or plugin definition.
func remotesOf(t *testing.T, definition map[string]any) map[string]any {
	t.Helper()

	remotes, _ := sourceGitOf(t, definition)["remotes"].(map[string]any)
	if remotes == nil {
		t.Fatalf("no source.git.remotes in %v", definition)
	}

	return remotes
}

// pluginOf returns the recorded entry of a plugin that must be there.
func pluginOf(t *testing.T, live *Live, name string) map[string]any {
	t.Helper()

	entry, ok := live.Plugin(name)
	if !ok {
		t.Fatalf("%s is not recorded in %s", name, config.LiveRecipeFile)
	}

	return entry
}

// remoteURL asks git where a remote points.
func remoteURL(t *testing.T, dir string, name string) string {
	t.Helper()

	return strings.TrimSpace(gitOut(t, dir, "remote", "get-url", name))
}

// headOf is the commit a checkout is sitting on.
func headOf(t *testing.T, dir string) string {
	t.Helper()

	return strings.TrimSpace(gitOut(t, dir, "rev-parse", "HEAD"))
}

// TestRecloneRepointsARemoteWithoutFetching is the command's reason to exist:
// swap a URL in the live recipe and have the checkout follow. The new URL
// leads nowhere, so the run succeeding also proves that an existing checkout
// is not fetched.
func TestRecloneRepointsARemoteWithoutFetching(t *testing.T) {
	root, pluginsDir, mulib := recloneFixture(t)

	const moved = "git@nowhere.invalid:mutms/moodle-tool_mulib.git"

	head := headOf(t, mulib)

	editLive(t, root, func(live *Live) {
		remotesOf(t, pluginOf(t, live, "mutms/tool_mulib"))["origin"] = moved
	})

	var out bytes.Buffer

	if err := recloneInto(t, root, pluginsDir, &out); err != nil {
		t.Fatalf("reclone: %v\n%s", err, out.String())
	}

	if got := remoteURL(t, mulib, "origin"); got != moved {
		t.Errorf("origin is %q, want %q", got, moved)
	}

	if got := headOf(t, mulib); got != head {
		t.Errorf("the checkout moved from %s to %s", head, got)
	}

	if !strings.Contains(out.String(), "→ "+moved) {
		t.Errorf("the changed remote was not reported:\n%s", out.String())
	}

	if !strings.Contains(out.String(), "mudev fetch") {
		t.Errorf("no hint that the new remote still needs a fetch:\n%s", out.String())
	}

	// The record keeps what the developer wrote.
	if got := remotesOf(t, pluginOf(t, liveOf(t, root), "mutms/tool_mulib"))["origin"]; got != moved {
		t.Errorf("the live recipe records origin %q, want %q", got, moved)
	}
}

// TestRecloneRepointsCore checks that the base block is applied too.
func TestRecloneRepointsCore(t *testing.T) {
	root, pluginsDir, _ := recloneFixture(t)

	const moved = "git@nowhere.invalid:mutms/moodle.git"

	editLive(t, root, func(live *Live) {
		remotesOf(t, live.Base)["origin"] = moved
	})

	if err := recloneInto(t, root, pluginsDir, io.Discard); err != nil {
		t.Fatalf("reclone: %v", err)
	}

	if got := remoteURL(t, root, "origin"); got != moved {
		t.Errorf("core's origin is %q, want %q", got, moved)
	}
}

// TestRecloneAddsARemoteAndKeepsOthers pins that reclone only ever adds: a
// remote written into the entry appears, and one the developer added with git
// — which no entry names — is not taken away.
func TestRecloneAddsARemoteAndKeepsOthers(t *testing.T) {
	root, pluginsDir, mulib := recloneFixture(t)

	runGit(t, mulib, "remote", "add", "personal", "git@nowhere.invalid:me/fork.git")

	editLive(t, root, func(live *Live) {
		remotesOf(t, pluginOf(t, live, "mutms/tool_mulib"))["upstream"] = "git@nowhere.invalid:up/stream.git"
	})

	if err := recloneInto(t, root, pluginsDir, io.Discard); err != nil {
		t.Fatalf("reclone: %v", err)
	}

	if got := remoteURL(t, mulib, "upstream"); got != "git@nowhere.invalid:up/stream.git" {
		t.Errorf("upstream is %q", got)
	}

	if got := remoteURL(t, mulib, "personal"); got != "git@nowhere.invalid:me/fork.git" {
		t.Errorf("the hand-added remote is %q — it should have been left alone", got)
	}
}

// TestRecloneChecksOutAnEntryAddedByHand covers the other half: an entry with
// no checkout is acquired, and a bare one is flattened into the record.
func TestRecloneChecksOutAnEntryAddedByHand(t *testing.T) {
	root, pluginsDir, mulib := recloneFixture(t)

	head := headOf(t, mulib)

	editLive(t, root, func(live *Live) {
		live.Plugins = append(live.Plugins, map[string]any{"name": "mutms/tool_muprog"})
	})

	var out bytes.Buffer

	if err := recloneInto(t, root, pluginsDir, &out); err != nil {
		t.Fatalf("reclone: %v\n%s", err, out.String())
	}

	muprog := filepath.Join(root, "public", "admin", "tool", "muprog")

	if branch := strings.TrimSpace(gitOut(t, muprog, "rev-parse", "--abbrev-ref", "HEAD")); branch != "MOODLE_500_STABLE" {
		t.Errorf("checked out branch %q, want MOODLE_500_STABLE", branch)
	}

	entry := pluginOf(t, liveOf(t, root), "mutms/tool_muprog")

	if entry["relpath"] != "public/admin/tool/muprog" {
		t.Errorf("the bare entry was not flattened: %v", entry)
	}

	if got := headOf(t, mulib); got != head {
		t.Errorf("the existing checkout moved from %s to %s", head, got)
	}

	if !strings.Contains(out.String(), "1 new checkout(s), 0 remote(s) changed") {
		t.Errorf("unexpected summary:\n%s", out.String())
	}
}

// TestRecloneLeavesAnExistingWorkingTreeAlone pins that an edited ref does not
// move a checkout that is already there — the developer may be on a feature
// branch, and switching is theirs to do.
func TestRecloneLeavesAnExistingWorkingTreeAlone(t *testing.T) {
	root, pluginsDir, mulib := recloneFixture(t)

	runGit(t, mulib, "switch", "--quiet", "-c", "feature")

	editLive(t, root, func(live *Live) {
		sourceGitOf(t, pluginOf(t, live, "mutms/tool_mulib"))["ref"] = "origin/SOMETHING_ELSE"
	})

	if err := recloneInto(t, root, pluginsDir, io.Discard); err != nil {
		t.Fatalf("reclone: %v", err)
	}

	if branch := strings.TrimSpace(gitOut(t, mulib, "rev-parse", "--abbrev-ref", "HEAD")); branch != "feature" {
		t.Errorf("the checkout was switched to %q", branch)
	}
}

// TestRecloneStopsBeforeTouchingTheTree checks that a file with one good edit
// and one that cannot resolve applies neither.
func TestRecloneStopsBeforeTouchingTheTree(t *testing.T) {
	root, pluginsDir, mulib := recloneFixture(t)

	original := remoteURL(t, mulib, "origin")

	editLive(t, root, func(live *Live) {
		remotesOf(t, pluginOf(t, live, "mutms/tool_mulib"))["origin"] = "git@nowhere.invalid:moved.git"

		live.Plugins = append(live.Plugins, map[string]any{"name": "mutms/tool_unheardof"})
	})

	err := recloneInto(t, root, pluginsDir, io.Discard)
	if err == nil {
		t.Fatal("an entry that is in no catalogue was accepted")
	}

	if !strings.Contains(err.Error(), "mutms/tool_unheardof") {
		t.Errorf("the error does not name the entry: %v", err)
	}

	if got := remoteURL(t, mulib, "origin"); got != original {
		t.Errorf("origin was changed to %q although the run failed", got)
	}
}

// TestRecloneRefusesADirectoryThatIsNotAWorkspace checks the missing-record case.
func TestRecloneRefusesADirectoryThatIsNotAWorkspace(t *testing.T) {
	err := recloneInto(t, t.TempDir(), t.TempDir(), io.Discard)
	if err == nil {
		t.Fatal("a directory with no live recipe was accepted")
	}

	if !strings.Contains(err.Error(), config.LiveRecipeFile) {
		t.Errorf("the error does not mention %s: %v", config.LiveRecipeFile, err)
	}
}

// TestRecloneIsANoOpOnAnUntouchedWorkspace checks that applying an unedited
// record changes neither the tree nor the file.
func TestRecloneIsANoOpOnAnUntouchedWorkspace(t *testing.T) {
	root, pluginsDir, _ := recloneFixture(t)

	before, err := os.ReadFile(LivePath(root))
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer

	if err := recloneInto(t, root, pluginsDir, &out); err != nil {
		t.Fatalf("reclone: %v", err)
	}

	after, err := os.ReadFile(LivePath(root))
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(before, after) {
		t.Errorf("an unedited live recipe was rewritten:\n%s\n---\n%s", before, after)
	}

	if !strings.Contains(out.String(), "0 new checkout(s), 0 remote(s) changed") {
		t.Errorf("unexpected summary:\n%s", out.String())
	}
}
