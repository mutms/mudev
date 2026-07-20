package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mutms/mudev/internal/config"
	"github.com/mutms/mudev/internal/git"
)

// PruneOptions configure reconciling a live recipe with the tree.
type PruneOptions struct {
	// Root is the workspace to reconcile.
	Root string

	// Out receives mudev's own lines.
	Out io.Writer
}

// Prune drops the plugins the live recipe records but the tree no longer has.
//
// This is how a plugin leaves a workspace: the developer deletes the directory,
// and mudev catches the record up. Deliberately in that order — a checkout can
// hold uncommitted changes, unpushed commits or a stash, none of which the
// recipe knows about and none of which mudev could give back, so removing code
// is the developer's decision to make and to carry out. Reconciling the record
// with what is actually on disk is the part mudev can do safely.
//
// It touches no git repository and no working tree: entries leave .mudev.json,
// and the exclude that hid each one from its containing repository is cleaned
// up. `mudev list` marks exactly what this would remove with ✗, so it doubles
// as the preview.
func Prune(opts PruneOptions) error {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}

	out := newOutput(opts.Out)

	live, err := LoadLive(root)
	if err != nil {
		return err
	}

	if live == nil {
		return fmt.Errorf(
			"%s has no %s — there is no record to reconcile", root, config.LiveRecipeFile,
		)
	}

	ws, err := Enumerate(root)
	if err != nil {
		return err
	}

	pruned := 0

	for _, repo := range ws.Repos {
		if !repo.Missing || repo.Core {
			continue
		}

		// A directory that is still there but is not a checkout is not a
		// deleted plugin — it is a plugin whose .git went missing, or one
		// unpacked from a zip. Either way, dropping the record would strand
		// files nobody is tracking any more.
		if _, err := os.Stat(filepath.Join(root, repo.Path)); err == nil {
			out.warnf("%s is still in the tree but is not a git checkout — left recorded", repo.Path)

			continue
		}

		if !live.RemovePlugin(repo.Name) {
			continue
		}

		// The exclude outlives the checkout otherwise, and would go on hiding
		// that path from the containing repository.
		container, relative := containingRepo(root, repo.Path)

		if _, err := git.RemoveExclude(container, "/"+relative); err != nil {
			return err
		}

		out.printf("%s (%s): gone from the tree — removed from %s",
			repo.Name, repo.Path, config.LiveRecipeFile)

		pruned++
	}

	if pruned == 0 {
		out.printf("%s already matches the tree — nothing to prune", config.LiveRecipeFile)

		return nil
	}

	if err := live.Save(root); err != nil {
		return err
	}

	out.printf("pruned %d plugin(s); %s now differs from %s",
		pruned, config.LiveRecipeFile, basedOn(live))

	return nil
}
