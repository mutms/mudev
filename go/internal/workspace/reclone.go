package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mutms/mudev/go/internal/config"
	"github.com/mutms/mudev/go/internal/git"
)

// RecloneOptions configure applying a hand-edited live recipe to its workspace.
type RecloneOptions struct {
	// Config is the resolved configuration.
	Config config.Config

	// Root is the workspace whose live recipe is applied.
	Root string

	// Out receives mudev's own progress lines.
	Out io.Writer
}

// Reclone assembles the workspace again, from its own live recipe.
//
// The live recipe is a complete recipe document, so a developer can edit it —
// swap the https remotes a public recipe names for ssh ones, add a mirror, add
// a whole plugin entry — and this is what makes the tree follow. It is `clone`
// with .mudev.json as the recipe, and it inherits clone's idempotence, which is
// exactly the behaviour wanted here:
//
//   - a checkout that already exists gets its remotes added or re-pointed and
//     is otherwise left alone — not fetched, not pulled, not switched, even
//     when the entry's ref was edited, because the developer may be on a
//     feature branch;
//   - an entry with no checkout yet is acquired like any new plugin.
//
// It only ever adds. A remote taken out of an entry stays in the checkout, as
// it may equally be one the developer added by hand, and a plugin taken out of
// the file keeps its directory — mudev never deletes plugin code.
//
// Every entry is resolved before git is asked to change anything, so an edit
// that does not resolve stops the command with the tree as it was.
func Reclone(ctx context.Context, opts RecloneOptions) error {
	if !git.Available() {
		return fmt.Errorf("git was not found on PATH")
	}

	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}

	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return fmt.Errorf("workspace directory %s does not exist", root)
	}

	c := &cloner{
		cfg:  opts.Config,
		root: root,
		out:  newOutput(opts.Out),

		// Applying a workspace's own record is by definition a continuation.
		resuming: true,
	}

	// Core is not verified here: assembleBase does it, after completing a core
	// that an interrupted clone left unfinished.
	if err := c.loadWorkspace(); err != nil {
		return err
	}

	if err := refusePatches(c.recipe); err != nil {
		return err
	}

	c.printf("recipe:      %s", LivePath(c.root))
	c.printf("base:        branch %s%s", c.recipe.Base.Mdlbranch, c.layoutNote())
	c.printf("plugins:     %d", len(c.recipe.Plugins))

	plugins, err := c.resolvePlugins()
	if err != nil {
		return err
	}

	if err := c.assembleBase(ctx); err != nil {
		return fmt.Errorf("base: %w", err)
	}

	for _, p := range plugins {
		if err := c.assemblePlugin(ctx, p); err != nil {
			return fmt.Errorf("plugin %s: %w", p.Name, err)
		}
	}

	c.printf("workspace matches %s: %d new checkout(s), %d remote(s) changed",
		config.LiveRecipeFile, c.created, c.remoteChanges)

	if c.remoteChanges > 0 {
		// A changed URL has not been contacted, and a new remote has no
		// tracking refs yet — both are one fetch away.
		c.printf("existing checkouts were not fetched — run `mudev fetch` to use the new remotes")
	}

	return nil
}
