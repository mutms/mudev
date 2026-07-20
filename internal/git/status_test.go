package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mutms/mudev/internal/exec"
)

// run executes a git command in dir, failing the test if it does not succeed.
// The tests drive the real git binary — that is the point, since Status exists
// to report what real git reports.
func run(t *testing.T, dir string, args ...string) {
	t.Helper()

	res, err := exec.Capture(context.Background(), exec.Cmd{Name: "git", Args: args, Dir: dir})
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}

	if res.Failed() {
		t.Fatalf("git %v: %s", args, res.Stderr)
	}
}

// initRepo creates a repository with committing configured locally, so the
// test never depends on the developer's global git config (or their signing
// key, which is not available to a test).
func initRepo(t *testing.T, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	run(t, dir, "init", "--quiet", "--initial-branch=MOODLE_502_STABLE")
	run(t, dir, "config", "user.email", "test@example.org")
	run(t, dir, "config", "user.name", "mudev test")
	run(t, dir, "config", "commit.gpgsign", "false")
}

func commit(t *testing.T, dir string, message string) {
	t.Helper()

	run(t, dir, "commit", "--quiet", "--allow-empty", "--message", message)
}

func TestStatusUnbornRepository(t *testing.T) {
	if !Available() {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()
	initRepo(t, dir)

	// A `git init` with no commit yet must not break a listing.
	s, err := (&Client{}).Status(context.Background(), dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if !s.Unborn {
		t.Errorf("expected an unborn repository: %+v", s)
	}

	if s.Branch != "MOODLE_502_STABLE" {
		t.Errorf("Branch = %q", s.Branch)
	}

	if s.Detached || s.Ahead != 0 || s.Behind != 0 {
		t.Errorf("unexpected state: %+v", s)
	}
}

func TestStatusBranchDirtyAndTags(t *testing.T) {
	if !Available() {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()
	initRepo(t, dir)
	commit(t, dir, "initial")
	run(t, dir, "tag", "v5.2.1.01")

	c := &Client{}
	ctx := context.Background()

	s, err := c.Status(ctx, dir)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if s.Branch != "MOODLE_502_STABLE" || s.Unborn || s.Detached {
		t.Errorf("unexpected state: %+v", s)
	}

	if s.Dirty {
		t.Error("a fresh checkout should be clean")
	}

	if len(s.Tags) != 1 || s.Tags[0] != "v5.2.1.01" {
		t.Errorf("Tags = %v", s.Tags)
	}

	if s.Head == "" {
		t.Error("Head should carry the abbreviated commit")
	}

	// An untracked file makes the tree dirty.
	if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if s, err = c.Status(ctx, dir); err != nil {
		t.Fatalf("Status: %v", err)
	}

	if !s.Dirty {
		t.Error("an untracked file should show as uncommitted work")
	}
}

func TestStatusAheadBehindAndDetached(t *testing.T) {
	if !Available() {
		t.Skip("git is not installed")
	}

	root := t.TempDir()

	// A local "remote" — no network needed, and file:// exercises the same
	// code path as any other git URL.
	origin := filepath.Join(root, "origin")
	initRepo(t, origin)
	commit(t, origin, "one")
	commit(t, origin, "two")

	clone := filepath.Join(root, "clone")

	c := &Client{}
	ctx := context.Background()

	if err := c.Clone(ctx, origin, clone); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	run(t, clone, "config", "user.email", "test@example.org")
	run(t, clone, "config", "user.name", "mudev test")
	run(t, clone, "config", "commit.gpgsign", "false")

	// One commit here that the remote does not have…
	commit(t, clone, "local work")

	// …and two on the remote that this checkout does not have yet.
	commit(t, origin, "three")
	commit(t, origin, "four")

	if err := c.Fetch(ctx, clone, "origin"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	s, err := c.Status(ctx, clone)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if s.Ahead != 1 || s.Behind != 2 {
		t.Errorf("Ahead/Behind = %d/%d, want 1/2", s.Ahead, s.Behind)
	}

	if s.Tracking != "origin/MOODLE_502_STABLE" {
		t.Errorf("Tracking = %q", s.Tracking)
	}

	// A pinned edition is checked out detached, which reports no divergence.
	if err := c.CheckoutDetached(ctx, clone, "HEAD~1"); err != nil {
		t.Fatalf("CheckoutDetached: %v", err)
	}

	if s, err = c.Status(ctx, clone); err != nil {
		t.Fatalf("Status: %v", err)
	}

	if !s.Detached || s.Branch != "" || s.Ahead != 0 || s.Behind != 0 {
		t.Errorf("unexpected detached state: %+v", s)
	}
}

func TestPullFastForwardsAndRefusesDivergence(t *testing.T) {
	if !Available() {
		t.Skip("git is not installed")
	}

	root := t.TempDir()

	origin := filepath.Join(root, "origin")
	initRepo(t, origin)
	commit(t, origin, "one")

	clone := filepath.Join(root, "clone")

	c := &Client{}
	ctx := context.Background()

	if err := c.Clone(ctx, origin, clone); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	run(t, clone, "config", "user.email", "test@example.org")
	run(t, clone, "config", "user.name", "mudev test")
	run(t, clone, "config", "commit.gpgsign", "false")

	// A checkout that is merely behind fast-forwards.
	commit(t, origin, "two")

	if err := c.Pull(ctx, clone); err != nil {
		t.Fatalf("Pull: %v", err)
	}

	s, err := c.Status(ctx, clone)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	if s.Behind != 0 || s.Ahead != 0 {
		t.Errorf("after a pull the checkout should match its upstream: %+v", s)
	}

	// A checkout that has diverged must refuse rather than merge behind the
	// developer's back — the whole reason pull is --ff-only.
	commit(t, clone, "local work")
	commit(t, origin, "three")

	if err := c.Pull(ctx, clone); err == nil {
		t.Fatal("expected a diverged checkout to refuse to pull")
	}

	if s, err = c.Status(ctx, clone); err != nil {
		t.Fatalf("Status: %v", err)
	}

	// The refusal must leave the checkout exactly as it was.
	if s.Ahead != 1 || s.Behind != 1 {
		t.Errorf("a refused pull should change nothing: %+v", s)
	}
}

func TestOnBranchAndHasUpstream(t *testing.T) {
	if !Available() {
		t.Skip("git is not installed")
	}

	root := t.TempDir()

	origin := filepath.Join(root, "origin")
	initRepo(t, origin)
	commit(t, origin, "one")

	c := &Client{}
	ctx := context.Background()

	// A repository with no commits has a branch name but nothing to pull into.
	unborn := filepath.Join(root, "unborn")
	initRepo(t, unborn)

	if _, ok := c.OnBranch(ctx, unborn); ok {
		t.Error("an unborn repository is not on a usable branch")
	}

	clone := filepath.Join(root, "clone")

	if err := c.Clone(ctx, origin, clone); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	branch, ok := c.OnBranch(ctx, clone)
	if !ok || branch != "MOODLE_502_STABLE" {
		t.Errorf("OnBranch = %q, %v", branch, ok)
	}

	if !c.HasUpstream(ctx, clone) {
		t.Error("a cloned branch tracks its remote")
	}

	// A pinned edition is checked out detached: nothing to pull into.
	if err := c.CheckoutDetached(ctx, clone, "HEAD"); err != nil {
		t.Fatalf("CheckoutDetached: %v", err)
	}

	if _, ok := c.OnBranch(ctx, clone); ok {
		t.Error("a detached checkout is not on a branch")
	}

	// A local branch with no upstream has nothing to pull from.
	run(t, clone, "switch", "--quiet", "--create", "local-only")

	if c.HasUpstream(ctx, clone) {
		t.Error("a fresh local branch has no upstream")
	}
}
