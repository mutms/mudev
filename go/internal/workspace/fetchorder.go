package workspace

import (
	"context"
	"sort"

	"github.com/mutms/mudev/go/internal/git"
)

// OriginRemote is the remote a checkout is acquired from, and the one whose
// failure is always fatal.
const OriginRemote = "origin"

// orderRemotes decides what order a checkout's remotes are fetched in: origin
// first, then the rest alphabetically, so a run is reproducible.
func orderRemotes(remotes map[string]string) []string {
	rest := make([]string, 0, len(remotes))

	for name := range remotes {
		rest = append(rest, name)
	}

	sort.Strings(rest)

	// origin leads the unlisted remainder: it is the remote everything else is
	// measured against.
	for i, name := range rest {
		if name == OriginRemote {
			copy(rest[1:i+1], rest[:i])
			rest[0] = OriginRemote

			break
		}
	}

	return rest
}

// fetchRemotes fetches every remote of a checkout, origin first.
//
// Fetching them all is deliberate: a development recipe carries mirrors and
// fork upstreams that a developer expects to be up to date, and a public
// recipe has nothing but origin, so the rule costs it nothing.
//
// Only origin is load-bearing. A mirror that cannot be reached — a laptop off
// the office network, a backup host down for maintenance — is reported and
// stepped over, because failing the whole assembly over a copy of data that
// origin also has would make the mirror a single point of failure.
func fetchRemotes(ctx context.Context, client *git.Client, dir string, out output) error {
	remotes, err := client.Remotes(ctx, dir)
	if err != nil {
		return err
	}

	ordered := orderRemotes(remotes)

	// A shallow checkout — a pinned edition assembled with --depth 1 — is
	// deepened before anything else happens. Asking for a fetch is asking
	// for history, and mudev has no business making the caller discover
	// git's "unshallow first" errors on their own. One-way on purpose:
	// nothing re-shallows a checkout that has been filled in.
	if client.IsShallow(ctx, dir) {
		out.stepf("unshallow %s (was assembled shallow)", OriginRemote)

		if err := client.Unshallow(ctx, dir, OriginRemote); err != nil {
			return err
		}
	}

	for i, name := range ordered {
		// Say which remote the transfer that follows is coming from: git's own
		// progress does not, and "1.2M objects" from a LAN mirror and from
		// github look exactly alike until the download finishes.
		out.stepf("fetch %s (%d/%d) %s", name, i+1, len(ordered), remotes[name])

		if err := client.Fetch(ctx, dir, name); err != nil {
			if name == OriginRemote {
				return err
			}

			out.warnf("could not fetch remote %s (%s): %v", name, remotes[name], err)
		}
	}

	return nil
}
