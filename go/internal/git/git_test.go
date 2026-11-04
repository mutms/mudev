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
		"git@10.1.10.100:mutms/patches.git",
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

// TestUnshallowLegacyShallowCheckout covers checkouts assembled by older mudev
// versions with --shallow: they report themselves shallow, and unshallowing
// fills the history back in, which `mudev fetch` does before anything else.
func TestUnshallowLegacyShallowCheckout(t *testing.T) {
	ctx := context.Background()
	c := &Client{}

	// An origin with several commits and a tag on the last one, so a
	// depth-1 fetch of that tag is provably fewer commits than the whole.
	origin := t.TempDir()
	mustGit(t, origin, "init", "--quiet", "-b", "main")
	mustGit(t, origin, "config", "user.email", "t@example.org")
	mustGit(t, origin, "config", "user.name", "t")
	// The developer's own git config may sign commits; a fixture must not
	// depend on an agent being reachable.
	mustGit(t, origin, "config", "commit.gpgsign", "false")

	for i := range 3 {
		name := filepath.Join(origin, fmt.Sprintf("f%d", i))
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mustGit(t, origin, "add", ".")
		mustGit(t, origin, "commit", "--quiet", "-m", fmt.Sprintf("c%d", i))
	}
	mustGit(t, origin, "tag", "v1.0")

	dir := t.TempDir()
	if err := c.Init(ctx, dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	// file:// so git honours --depth over the local transport.
	if err := c.SetRemote(ctx, dir, "origin", "file://"+origin); err != nil {
		t.Fatalf("set remote: %v", err)
	}
	mustGit(t, dir, "fetch", "--quiet", "--depth", "1", "origin", "tag", "v1.0")

	if !c.IsShallow(ctx, dir) {
		t.Fatal("repository does not report itself shallow after --depth 1")
	}
	if got := commitCount(t, dir, "v1.0"); got != 1 {
		t.Errorf("shallow fetch brought %d commits, want 1", got)
	}

	if err := c.Unshallow(ctx, dir, "origin"); err != nil {
		t.Fatalf("unshallow: %v", err)
	}
	if c.IsShallow(ctx, dir) {
		t.Error("still reports itself shallow after --unshallow")
	}
	if got := commitCount(t, dir, "v1.0"); got != 3 {
		t.Errorf("after unshallow %d commits, want 3", got)
	}
}

// TestIsShallowOnOrdinaryRepo guards the negative: a normal checkout must not
// be mistaken for a shallow one, or every fetch would pay for an unshallow
// that has nothing to do.
func TestIsShallowOnOrdinaryRepo(t *testing.T) {
	ctx := context.Background()
	c := &Client{}

	dir := t.TempDir()
	if err := c.Init(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if c.IsShallow(ctx, dir) {
		t.Error("a freshly initialised repository reports itself shallow")
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := &Client{}
	if err := c.run(t.Context(), dir, args...); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}

// commitCount counts reachable commits from ref. Counted from the fetched
// ref rather than HEAD: mudev fetches before it checks anything out, which is
// exactly the state under test.
func commitCount(t *testing.T, dir string, ref string) int {
	t.Helper()
	c := &Client{}
	out, err := c.capture(t.Context(), dir, "rev-list", "--count", ref)
	if err != nil {
		t.Fatalf("rev-list: %v", err)
	}
	n := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d", &n); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	return n
}
