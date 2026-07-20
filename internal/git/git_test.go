package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRemoteURLsAreStoredUntouched pins mudev's URL policy, which is to have
// none: a recipe names the URL it means, and git gets exactly that. No hosting
// platform is assumed, and nothing is rewritten on the way through — so bare
// SSH, ssh://, file:// and https all reach a repository's config verbatim.
func TestRemoteURLsAreStoredUntouched(t *testing.T) {
	ctx := context.Background()
	c := &Client{}

	urls := []string{
		"git@github.com:mutms/moodle-tool_mulib.git",
		"https://github.com/mutms/moodle-tool_mulib.git",
		"git@192.168.1.100:mutms/patches.git",
		"ssh://git@example.org/plugin.git",
		"file:///srv/git/plugin.git",
	}

	dir := t.TempDir()

	if err := c.Init(ctx, dir); err != nil {
		t.Fatalf("init: %v", err)
	}

	for i, want := range urls {
		name := fmt.Sprintf("remote%d", i)

		if err := c.SetRemote(ctx, dir, name, want); err != nil {
			t.Fatalf("set remote %s: %v", name, err)
		}
	}

	remotes, err := c.Remotes(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	for i, want := range urls {
		if got := remotes[fmt.Sprintf("remote%d", i)]; got != want {
			t.Errorf("stored %q, want %q", got, want)
		}
	}
}

func TestSplitBranchRef(t *testing.T) {
	remotes := map[string]string{
		"origin":   "https://example.org/a.git",
		"upstream": "https://example.org/b.git",
	}

	// A branch name may itself contain slashes — only the remote list tells a
	// branch ref and a tag apart.
	remote, branch, ok := SplitBranchRef("origin/patch/mutms/MOODLE_502_STABLE", remotes)
	if !ok || remote != "origin" || branch != "patch/mutms/MOODLE_502_STABLE" {
		t.Errorf("SplitBranchRef = %q, %q, %v", remote, branch, ok)
	}

	if _, _, ok := SplitBranchRef("upstream/MOODLE_405_STABLE", remotes); !ok {
		t.Error("a non-origin remote should be recognised")
	}

	for _, ref := range []string{"v5.2.1.01", "bc04c7bf8256eb43e846dcd8519e5b8aa62adc62", "notaremote/x"} {
		if _, _, ok := SplitBranchRef(ref, remotes); ok {
			t.Errorf("SplitBranchRef(%q) should be a tag or commit", ref)
		}
	}
}

func TestAddExclude(t *testing.T) {
	repo := t.TempDir()

	if err := AddExclude(repo, "/public/admin/tool/mulib"); err != nil {
		t.Fatalf("AddExclude: %v", err)
	}

	// Repeating it must not duplicate the entry — clone is run again and again.
	if err := AddExclude(repo, "/public/admin/tool/mulib"); err != nil {
		t.Fatalf("AddExclude: %v", err)
	}

	if err := AddExclude(repo, "/.mudev.json"); err != nil {
		t.Fatalf("AddExclude: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.Count(string(content), "/public/admin/tool/mulib"); got != 1 {
		t.Errorf("pattern written %d times:\n%s", got, content)
	}

	if !strings.Contains(string(content), "/.mudev.json") {
		t.Errorf("second pattern missing:\n%s", content)
	}
}
