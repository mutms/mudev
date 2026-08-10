// Package git is mudev's domain wrapper around the real git binary.
//
// mudev shells out to git rather than using a Go implementation so that the
// SSH agent, submodules, subtrees and ordinary git semantics all behave
// exactly as they do on the command line — and so that no hosting platform is
// assumed: bare SSH (user@host:path), ssh://, file:// and https URLs are all
// passed through untouched. Every process here goes through internal/exec,
// mudev's single process gateway.
package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mutms/mudev/internal/config"
	"github.com/mutms/mudev/internal/exec"
)

// Client runs git commands for mudev.
//
// It holds no authentication policy of its own: a URL from a recipe goes to git
// exactly as written. Whether that URL is reachable, and with which key or
// credential, is git's business and the developer's — an SSH agent, a
// credential helper, or a url.<base>.insteadOf rule in ~/.gitconfig, all of
// which work for every git tool on the machine rather than only for this one.
type Client struct{}

// New builds a client from the resolved configuration.
//
// The configuration is not consulted yet — the parameter is kept because every
// caller already holds a Config, and the day mudev needs a git-affecting
// setting it should arrive here rather than through a new plumbing path.
func New(cfg config.Config) *Client {
	return &Client{}
}

// Available reports whether git can be found on PATH.
func Available() bool {
	return exec.Available("git")
}

// run executes a git command in dir, streaming its output to the terminal —
// git's own progress reporting is what a developer expects to see.
func (c *Client) run(ctx context.Context, dir string, args ...string) error {
	code, err := exec.Run(ctx, exec.Cmd{Name: "git", Args: args, Dir: dir})
	if err != nil {
		return err
	}

	if code != 0 {
		return fmt.Errorf("git %s: exit status %d", args[0], code)
	}

	return nil
}

// capture executes a git command and returns its trimmed stdout.
func (c *Client) capture(ctx context.Context, dir string, args ...string) (string, error) {
	res, err := exec.Capture(ctx, exec.Cmd{Name: "git", Args: args, Dir: dir})
	if err != nil {
		return "", err
	}

	if err := res.Err(); err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	return res.Stdout, nil
}

// IsRepo reports whether dir is the root of a git working tree.
func IsRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))

	return err == nil && (info.IsDir() || info.Mode().IsRegular())
}

// Init creates an empty repository in dir (which must exist).
func (c *Client) Init(ctx context.Context, dir string) error {
	return c.run(ctx, dir, "init", "--quiet")
}

// Clone clones raw into dir, creating parent directories as needed.
//
// mudev's own assembly never uses this — it builds every checkout with
// init + fetch + checkout so that exactly the recipe's ref lands and no stray
// default branch is left behind (see workspace.acquire). It is kept because
// tests need an ordinary clone to build fixtures from.
func (c *Client) Clone(ctx context.Context, raw string, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}

	return c.run(ctx, "", "clone", raw, dir)
}

// Passthrough runs an arbitrary git subcommand in dir, streaming its output.
//
// It is what the top-level verbs that mirror git are built on: mudev decides
// which checkouts to visit, git decides what the command means, so anything a
// developer already knows about `git status` or `git log` still applies.
func (c *Client) Passthrough(ctx context.Context, dir string, args ...string) error {
	return c.run(ctx, dir, args...)
}

// HasHead reports whether the repository has a commit at HEAD. A repository
// that does not is one where `git init` ran but the fetch never finished — an
// interrupted acquisition, not a usable checkout.
func (c *Client) HasHead(ctx context.Context, dir string) bool {
	_, err := c.optional(ctx, dir, "rev-parse", "--verify", "-q", "HEAD")

	return err == nil
}

// Remotes lists the repository's remotes as name → URL.
func (c *Client) Remotes(ctx context.Context, dir string) (map[string]string, error) {
	out, err := c.capture(ctx, dir, "remote", "-v")
	if err != nil {
		return nil, err
	}

	remotes := map[string]string{}

	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		remotes[fields[0]] = fields[1]
	}

	return remotes, nil
}

// SetRemote adds the named remote, or corrects its URL when it points
// somewhere else (a recipe or catalogue may have moved a repository).
func (c *Client) SetRemote(ctx context.Context, dir string, name string, want string) error {
	remotes, err := c.Remotes(ctx, dir)
	if err != nil {
		return err
	}

	switch have, exists := remotes[name]; {
	case !exists:
		return c.run(ctx, dir, "remote", "add", name, want)
	case have != want:
		return c.run(ctx, dir, "remote", "set-url", name, want)
	}

	return nil
}

// Fetch updates one remote, including its tags.
func (c *Client) Fetch(ctx context.Context, dir string, remote string) error {
	return c.run(ctx, dir, "fetch", "--tags", remote)
}

// FetchShallowTag fetches one tag and its tip commit, and nothing else.
//
// `tag <name>`, emphatically not `--tags`: the latter brings EVERY tag, and
// at depth 1 each one drags its own tree along. Measured against Moodle:
// --tags is 447 MB and 575 tags, `tag v4.5.12` is 77 MB and one — the
// difference between shallow being worth doing and not.
//
// Fails when ref is not a tag, which is how the caller learns to fall back to
// a full fetch (a commit pin cannot be fetched shallowly from every server).
func (c *Client) FetchShallowTag(ctx context.Context, dir string, remote string, ref string) error {
	return c.run(ctx, dir, "fetch", "--depth", "1", remote, "tag", ref)
}

// IsShallow reports whether dir has a truncated history.
//
// Asked of git rather than recorded by mudev: git owns this fact (.git/shallow),
// and a copy in the live recipe would start lying the moment someone runs
// `git fetch --unshallow` by hand.
func (c *Client) IsShallow(ctx context.Context, dir string) bool {
	out, err := c.capture(ctx, dir, "rev-parse", "--is-shallow-repository")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Unshallow restores the full history of a shallow checkout.
//
// Run before any fetch or pull: a shallow checkout is an assembly-time
// optimisation, and the moment someone asks for history they should simply
// have it rather than meet git's "unshallow first" errors.
func (c *Client) Unshallow(ctx context.Context, dir string, remote string) error {
	return c.run(ctx, dir, "fetch", "--unshallow", "--tags", remote)
}

// Pull updates the current branch from its upstream, fast-forward only.
//
// Refusing anything but a fast-forward is the point: a divergence needs a
// human decision (rebase or merge, and in which order), and a fan-out across
// twenty checkouts is the worst possible place to make that decision silently.
func (c *Client) Pull(ctx context.Context, dir string) error {
	return c.run(ctx, dir, "pull", "--ff-only")
}

// OnBranch reports the branch HEAD is on. A detached HEAD — how a pinned
// edition is checked out — has none, and neither does a repository without
// commits.
func (c *Client) OnBranch(ctx context.Context, dir string) (string, bool) {
	branch, err := c.optional(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil || branch == "" {
		return "", false
	}

	// A branch with no commit yet cannot be pulled into.
	if _, err := c.optional(ctx, dir, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		return branch, false
	}

	return branch, true
}

// HasUpstream reports whether the current branch tracks a remote branch.
func (c *Client) HasUpstream(ctx context.Context, dir string) bool {
	tracking, err := c.optional(ctx, dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")

	return err == nil && tracking != ""
}

// HasLocalBranch reports whether a local branch of that name exists.
func (c *Client) HasLocalBranch(ctx context.Context, dir string, name string) bool {
	res, err := exec.Capture(ctx, exec.Cmd{
		Name: "git",
		Args: []string{"show-ref", "--verify", "--quiet", "refs/heads/" + name},
		Dir:  dir,
	})

	return err == nil && !res.Failed()
}

// SwitchBranch puts the working tree on a local branch, creating it from start
// (a remote-tracking ref) when it does not exist yet. Creating it this way
// also sets the upstream, so pull/push and ahead-behind reporting just work.
func (c *Client) SwitchBranch(ctx context.Context, dir string, local string, start string) error {
	if c.HasLocalBranch(ctx, dir, local) {
		return c.run(ctx, dir, "switch", local)
	}

	return c.run(ctx, dir, "switch", "--create", local, "--track", start)
}

// CheckoutDetached checks out a tag or commit, leaving HEAD detached — the
// right state for a pinned edition, where there is no branch to follow.
func (c *Client) CheckoutDetached(ctx context.Context, dir string, ref string) error {
	return c.run(ctx, dir, "checkout", "--detach", ref)
}

// Head returns the checked-out branch name, or an empty string when HEAD is
// detached.
func (c *Client) Head(ctx context.Context, dir string) (string, error) {
	out, err := c.capture(ctx, dir, "branch", "--show-current")
	if err != nil {
		return "", err
	}

	return out, nil
}

// CommitSHA returns the full commit hash HEAD points at.
//
// `mudev recipe init` records it as the ref of a detached checkout that sits on
// no tag — a commit pin is the only honest thing to write down for a tree that
// is not on a branch and carries no tag to name it by.
func (c *Client) CommitSHA(ctx context.Context, dir string) (string, error) {
	out, err := c.capture(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

// SubmoduleUpdate initialises and updates any submodules a plugin carries.
// Site-level composition is mudev's job, but submodules *inside* a plugin are
// that plugin's own business and must still be materialised.
func (c *Client) SubmoduleUpdate(ctx context.Context, dir string) error {
	if _, err := os.Stat(filepath.Join(dir, ".gitmodules")); err != nil {
		return nil
	}

	return c.run(ctx, dir, "submodule", "update", "--init", "--recursive")
}

// SplitBranchRef decides whether a ref names a branch or a tag/commit.
//
// A branch is written the way git itself takes a start point —
// "<remote>/<branch>", where <remote> is one of that entry's own remotes. That
// is why the remote list is needed: branch names contain slashes too
// (origin/patch/mutms/MOODLE_502_STABLE), and only the remote list tells the
// two apart. Anything else is a tag or commit, checked out detached.
func SplitBranchRef(ref string, remotes map[string]string) (remote string, branch string, ok bool) {
	name, rest, found := strings.Cut(ref, "/")
	if !found || rest == "" {
		return "", "", false
	}

	if _, known := remotes[name]; !known {
		return "", "", false
	}

	return name, rest, true
}

// AddExclude adds a pattern to a repository's .git/info/exclude, unless it is
// already listed.
//
// This is how a plugin checked out inside the core working tree stays invisible
// to the surrounding repository: the containing repo excludes the plugin's
// path, so its `git status` stays clean instead of reporting the whole plugin
// as untracked.
func AddExclude(repo string, pattern string) error {
	path := filepath.Join(repo, ".git", "info", "exclude")

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == pattern {
			return nil
		}
	}

	content := string(existing)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	content += pattern + "\n"

	return os.WriteFile(path, []byte(content), 0o644)
}

// RemoveExclude drops a pattern from a repository's .git/info/exclude, and
// reports whether it was there.
//
// It is AddExclude's inverse, for when a nested checkout is gone: an exclude
// left behind would go on hiding that path from the surrounding repository, so
// files put there later — by a core upgrade, or by the developer — would be
// invisible to git status. Only mudev's own exact pattern is touched; anything
// else in the file is somebody's deliberate local exclude.
func RemoveExclude(repo string, pattern string) (bool, error) {
	path := filepath.Join(repo, ".git", "info", "exclude")

	existing, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, err
	}

	lines := strings.Split(string(existing), "\n")
	kept := make([]string, 0, len(lines))

	removed := false

	for _, line := range lines {
		if strings.TrimSpace(line) == pattern {
			removed = true

			continue
		}

		kept = append(kept, line)
	}

	if !removed {
		return false, nil
	}

	return true, os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o644)
}
