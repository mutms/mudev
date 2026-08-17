package workspace

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/mutms/mudev/go/internal/config"
	"github.com/mutms/mudev/go/internal/git"
)

// headerWidth is how wide the per-checkout separator is drawn, carried over
// from the old tool: wide enough to stand out above git's own output.
const headerWidth = 75

// FanOptions configure a command that runs git in every checkout.
type FanOptions struct {
	// Config is the resolved configuration.
	Config config.Config

	// Root is the workspace directory to work in.
	Root string

	// Out receives mudev's own lines. git writes to the terminal itself, so
	// its output appears under each header as it happens.
	Out io.Writer
}

// Fetch updates every checkout in the workspace, from every remote it has.
//
// All remotes, always: a development recipe carries mirrors and fork upstreams
// that a developer expects to be current, and a public recipe has nothing but
// origin, so the rule costs it nothing. The recipe's extra.mudev.fetch_order
// decides who is asked first — a mirror on the local network leaves the slow
// remote with only the difference to send.
//
// Fetching changes nothing in a working tree, so it runs everywhere: recorded
// plugins, Moodle core, and repositories nobody recorded.
func Fetch(ctx context.Context, opts FanOptions) error {
	client := git.New(opts.Config)

	out := newOutput(opts.Out)

	return fanOutWith(ctx, opts, func(ws *Workspace, dir string, repo Repo) (string, error) {
		return "", fetchRemotes(ctx, client, dir, ws.FetchOrder, out)
	})
}

// Pull fast-forwards every checkout in the workspace.
//
// A checkout that cannot simply move forward — diverged from its upstream, so
// a rebase or merge is needed — stops the whole run at that checkout, with
// nothing after it touched. That is deliberate: resolving a divergence is a
// decision for a human to make in one repository, not something to paper over
// twenty times in a row.
//
// Checkouts with nothing to pull into are skipped rather than failed: a pinned
// edition is entirely detached, and a fresh branch may have no upstream yet.
func Pull(ctx context.Context, opts FanOptions) error {
	client := git.New(opts.Config)

	return fanOut(ctx, opts, func(dir string, repo Repo) (string, error) {
		if _, ok := client.OnBranch(ctx, dir); !ok {
			return "not on a branch — skipped", nil
		}

		if !client.HasUpstream(ctx, dir) {
			return "no upstream branch — skipped", nil
		}

		return "", client.Pull(ctx, dir)
	})
}

// fanOut runs an action in every checkout, announcing each one first so that
// git's output underneath is attributable.
//
// The first failure stops the run: with twenty checkouts scrolling past, an
// error that merely got reported at the end is an error that gets missed.
func fanOut(ctx context.Context, opts FanOptions, action func(dir string, repo Repo) (string, error)) error {
	return fanOutWith(ctx, opts, func(ws *Workspace, dir string, repo Repo) (string, error) {
		return action(dir, repo)
	})
}

// fanOutWith is fanOut for actions that need the workspace itself — the fetch
// order, say, which is a property of the recipe rather than of one checkout.
func fanOutWith(ctx context.Context, opts FanOptions, action fanAction) error {
	return fanOutFiltered(ctx, opts, nil, action)
}

// fanAction is the work done in one checkout. Its string result is a short
// note printed underneath the header ("skipped", and the like).
type fanAction func(ws *Workspace, dir string, repo Repo) (string, error)

// fanFilter decides whether a checkout is worth showing at all. Returning
// false skips it silently, header included — which is the difference between
// a status report and twenty repetitions of "nothing to commit".
type fanFilter func(ws *Workspace, dir string, repo Repo) (bool, error)

// fanOutFiltered runs an action in every checkout the filter admits, counting
// the ones it passed over so the run can say so at the end. A skipped checkout
// must still be accounted for: silence about a repository is not the same as
// there being nothing to say about it.
func fanOutFiltered(ctx context.Context, opts FanOptions, filter fanFilter, action fanAction) error {
	ws, err := Enumerate(opts.Root)
	if err != nil {
		return err
	}

	out := newOutput(opts.Out)

	var worked, skipped int

	for _, repo := range ws.Repos {
		if repo.Missing {
			continue
		}

		dir := filepath.Join(ws.Root, repo.Path)

		if filter != nil {
			show, err := filter(ws, dir, repo)
			if err != nil {
				return fmt.Errorf("%s: %w", repo.Path, err)
			}

			if !show {
				skipped++

				continue
			}
		}

		out.printf("%s", header(repo.Path))

		note, err := action(ws, dir, repo)
		if err != nil {
			return fmt.Errorf("%s: %w", repo.Path, err)
		}

		if note != "" {
			out.stepf("%s", note)
		}

		worked++
	}

	switch {
	case worked == 0 && skipped == 0:
		out.printf("no git checkouts found in %s", ws.Root)

	case skipped > 0:
		out.printf("%d checkout(s) with nothing to report (--all shows them)", skipped)
	}

	return nil
}

// header draws the separator that introduces one checkout's git output.
func header(path string) string {
	line := "---- " + path + " "

	if len(line) >= headerWidth {
		return line
	}

	return line + strings.Repeat("-", headerWidth-len(line))
}

// Status runs git status in every checkout that has something to report.
//
// A workspace holds twenty-odd repositories, almost all of them untouched on
// any given day, so reporting them all would bury the one or two that matter.
// "Something to report" means work that is yours and not yet safe: uncommitted
// changes (tracked or untracked) and commits that have not been pushed. Being
// *behind* the upstream is deliberately not included — that is incoming work,
// not a pending decision, and mudev list shows it as N↓.
//
// Any extra arguments go straight to git, so `mudev status -s` or
// `mudev status -uall` mean exactly what they mean in git.
func Status(ctx context.Context, opts FanOptions, all bool, args []string) error {
	client := git.New(opts.Config)

	command := append([]string{"status"}, args...)

	// The filter already asks git for each checkout's state; remember it so the
	// action can explain a header that git's own output leaves bare.
	seen := map[string]git.Status{}

	return fanOutFiltered(ctx, opts,
		func(ws *Workspace, dir string, repo Repo) (bool, error) {
			status, err := client.Status(ctx, dir)
			if err != nil {
				return false, err
			}

			seen[dir] = status

			return all || status.Dirty || status.Ahead > 0, nil
		},
		func(ws *Workspace, dir string, repo Repo) (string, error) {
			if err := client.Passthrough(ctx, dir, command...); err != nil {
				return "", err
			}

			// `git status -s` prints nothing at all for a clean tree, so a
			// checkout listed purely because it has unpushed commits would show
			// an unexplained header. Say why it is here.
			if status := seen[dir]; status.Ahead > 0 && !status.Dirty {
				return fmt.Sprintf("%d commit(s) not pushed to %s", status.Ahead, status.Tracking), nil
			}

			return "", nil
		},
	)
}
