package workspace

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pruneInto runs the prune exactly as the command does, returning what it said.
func pruneInto(t *testing.T, root string) (string, error) {
	t.Helper()

	var out strings.Builder

	err := Prune(PruneOptions{Root: root, Out: &out})

	return out.String(), err
}

// addedFixture is a workspace with one plugin added by hand — the state a
// developer prunes from.
func addedFixture(t *testing.T) (root string, pluginsDir string, dir string) {
	t.Helper()

	root, pluginsDir = addFixture(t)

	if err := addInto(t, root, pluginsDir, "mutms/tool_mulib", ""); err != nil {
		t.Fatalf("add: %v", err)
	}

	return root, pluginsDir, filepath.Join(root, "public", "admin", "tool", "mulib")
}

func TestPruneForgetsADeletedPlugin(t *testing.T) {
	root, _, dir := addedFixture(t)

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	out, err := pruneInto(t, root)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}

	if _, ok := liveOf(t, root).Plugin("mutms/tool_mulib"); ok {
		t.Errorf("a deleted plugin is still recorded:\n%s", out)
	}

	// The exclude has to go too, or the containing repository keeps hiding a
	// path that anything might legitimately put files in later.
	excludes, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(excludes), "/public/admin/tool/mulib") {
		t.Errorf("the exclude outlived the checkout:\n%s", excludes)
	}

	// mudev's own state file must keep its exclude.
	if !strings.Contains(string(excludes), "/.mudev.json") {
		t.Errorf("prune removed an exclude that was not its own:\n%s", excludes)
	}
}

func TestPruneLeavesAPresentPluginAlone(t *testing.T) {
	root, _, _ := addedFixture(t)

	out, err := pruneInto(t, root)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}

	if _, ok := liveOf(t, root).Plugin("mutms/tool_mulib"); !ok {
		t.Errorf("a plugin that is still checked out was pruned:\n%s", out)
	}

	if !strings.Contains(out, "nothing to prune") {
		t.Errorf("prune did not report that there was nothing to do:\n%s", out)
	}
}

// TestPruneKeepsADirectoryThatIsNoLongerACheckout guards the ambiguous case: a
// path with files but no .git is not a deleted plugin, and forgetting it would
// strand code nobody tracks.
func TestPruneKeepsADirectoryThatIsNoLongerACheckout(t *testing.T) {
	root, _, dir := addedFixture(t)

	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}

	out, err := pruneInto(t, root)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}

	if _, ok := liveOf(t, root).Plugin("mutms/tool_mulib"); !ok {
		t.Errorf("a directory that still holds files was pruned from the recipe:\n%s", out)
	}

	if !strings.Contains(out, "not a git checkout") {
		t.Errorf("prune did not report why it left the entry alone:\n%s", out)
	}
}

// TestPruneAndAddRoundTrip is the loop a developer actually runs: drop a
// plugin, then put it back.
func TestPruneAndAddRoundTrip(t *testing.T) {
	root, pluginsDir, dir := addedFixture(t)

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := pruneInto(t, root); err != nil {
		t.Fatalf("prune: %v", err)
	}

	if err := addInto(t, root, pluginsDir, "mutms/tool_mulib", ""); err != nil {
		t.Fatalf("re-add: %v", err)
	}

	if _, ok := liveOf(t, root).Plugin("mutms/tool_mulib"); !ok {
		t.Errorf("the plugin was not recorded again after being pruned")
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Errorf("the plugin was not checked out again: %v", err)
	}

	excludes, err := os.ReadFile(filepath.Join(root, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(excludes), "/public/admin/tool/mulib") {
		t.Errorf("the exclude was not restored:\n%s", excludes)
	}
}

func TestPruneRefusesADirectoryThatIsNotAWorkspace(t *testing.T) {
	if err := Prune(PruneOptions{Root: t.TempDir(), Out: io.Discard}); err == nil {
		t.Fatal("pruning a directory with no live recipe succeeded")
	}
}
