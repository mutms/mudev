package workspace

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/mutms/mudev/go/internal/git"
)

// pushRemote is where push sends work. It is always origin: the one remote
// every recipe entry must have, and the one a developer's own commits belong
// on. Mirrors and fork upstreams are for reading.
const pushRemote = "origin"

// Push sends every plugin's unpushed commits to origin.
//
// Moodle core is left alone. Its checkout is a patched base branch that
// changes by deliberate, reviewed merges rather than by day-to-day work, so a
// push there is a decision to make by hand, not a side effect of pushing the
// plugins.
//
// A plugin is pushed only while it is on the branch the live recipe recorded,
// and only to that branch on origin. A checkout on any other branch — a
// feature branch left behind, a manual switch, a detached HEAD — is reported
// and passed over: which branch should receive that work is not something mudev
// can know. So is a repository the recipe does not record at all.
//
// Plugins with nothing to push print nothing and are counted at the end. The
// first failed push (the remote has moved on, say) stops the run there, so it
// cannot scroll past unnoticed.
func Push(ctx context.Context, opts FanOptions) error {
	ws, err := Enumerate(opts.Root)
	if err != nil {
		return err
	}

	client := git.New(opts.Config)

	out := newOutput(opts.Out)

	var visited, quiet int

	for _, repo := range ws.Repos {
		if repo.Core || repo.Missing {
			continue
		}

		visited++

		dir := filepath.Join(ws.Root, repo.Path)

		shown, err := pushPlugin(ctx, client, out, dir, repo)
		if err != nil {
			return fmt.Errorf("%s: %w", repo.Path, err)
		}

		if !shown {
			quiet++
		}
	}

	switch {
	case visited == 0:
		out.printf("no plugin checkouts found in %s", ws.Root)

	case quiet > 0:
		out.printf("%d plugin(s) with nothing to push", quiet)
	}

	return nil
}

// pushPlugin pushes one plugin checkout, or explains why it did not. The
// checkout's header is printed only when there is something to show under it;
// shown reports whether it was.
func pushPlugin(ctx context.Context, client *git.Client, out output, dir string, repo Repo) (shown bool, err error) {
	notice := func(format string, args ...any) (bool, error) {
		out.printf("%s", header(repo.Path))
		out.stepf(format+" — not pushed", args...)

		return true, nil
	}

	if !repo.Managed {
		return notice("not recorded in the live recipe, so there is no branch to push to")
	}

	status, err := client.Status(ctx, dir)
	if err != nil {
		return false, err
	}

	// No commit yet: nothing exists that could be pushed.
	if status.Unborn {
		return false, nil
	}

	// The recipe pins a tag or commit. Detached there is the expected state
	// and there is nothing of the developer's to send; on a branch it is a
	// checkout that has wandered off the recipe.
	if repo.RecordedBranch == "" {
		if status.Detached {
			return false, nil
		}

		return notice("on branch %s, but the recipe pins %s rather than a branch", status.Branch, repo.RecordedRef)
	}

	if status.Detached {
		return notice("HEAD is detached, the recipe expects branch %s", repo.RecordedBranch)
	}

	if status.Branch != repo.RecordedBranch {
		return notice("on branch %s, the recipe expects %s", status.Branch, repo.RecordedBranch)
	}

	unpushed, known, err := client.Unpushed(ctx, dir, repo.RecordedBranch, pushRemote, repo.RecordedRemoteBranch)
	if err != nil {
		return false, err
	}

	if known && unpushed == 0 {
		return false, nil
	}

	out.printf("%s", header(repo.Path))

	return true, client.Push(ctx, dir, pushRemote, repo.RecordedBranch, repo.RecordedRemoteBranch)
}
