package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mutms/mudev/internal/config"
	"github.com/mutms/mudev/internal/git"
	"github.com/mutms/mudev/internal/plugin"
	"github.com/mutms/mudev/internal/recipe"
)

// AddOptions configure adding a plugin to a workspace that already exists.
type AddOptions struct {
	// Config is the resolved configuration.
	Config config.Config

	// Plugin is what the user asked for: a vendor/package catalogue identifier
	// or a path to a plugin YAML file.
	Plugin string

	// Ref pins what to check out, overriding branch resolution. Empty means the
	// branch that serves this workspace's Moodle.
	Ref string

	// Root is the workspace to add to.
	Root string

	// Out receives mudev's own progress lines.
	Out io.Writer
}

// Add checks a plugin out into an existing workspace and records it in the live
// recipe.
//
// The live recipe is itself a recipe document, so this drives exactly the same
// machinery `clone` does — same path resolution, same branch resolution against
// the workspace's Moodle, same incremental record. The difference is only where
// the recipe came from: the workspace, rather than the catalogue.
//
// Exactly the plugin asked for is added, and nothing else. A branch declares
// what it depends on, but composing a site is a decision, not a consequence:
// mudev reports the requirements it can see and leaves acting on them to the
// developer, who may well already have the dependency elsewhere, want a
// different version of it, or be adding it next anyway. Moodle itself checks
// dependencies at install time, which is where that check belongs.
//
// The workspace is left diverged from the recipe it was assembled from — that
// is the point of the command, and the closing line says so.
func Add(ctx context.Context, opts AddOptions) error {
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

		// Adding to a workspace is by definition a continuation of one.
		resuming: true,
	}

	if err := c.openWorkspace(); err != nil {
		return err
	}

	requested, err := plugin.Open(c.cfg.PluginsDir, opts.Plugin)
	if err != nil {
		return err
	}

	c.printf("moodle:  branch %s%s", c.recipe.Moodle.Mdlbranch, c.layoutNote())
	c.printf("adding:  %s%s", requested.Name, from(requested.File))

	if _, recorded := c.live.Plugin(requested.Name); recorded {
		c.printf("%s is already in %s — nothing to do", requested.Name, config.LiveRecipeFile)

		return nil
	}

	// Resolution happens before anything is checked out, so a recipe or
	// catalogue problem stops the command with the tree still untouched.
	entry := recipe.Entry{Name: requested.Name}

	if opts.Ref != "" {
		entry.Raw = pinnedRef(opts.Ref)
	}

	resolved, err := c.resolveEntry(entry, requested.Raw)
	if err != nil {
		return fmt.Errorf("plugin %s: %w", requested.Name, err)
	}

	if err := c.assemblePlugin(ctx, resolved); err != nil {
		return fmt.Errorf("plugin %s: %w", requested.Name, err)
	}

	c.reportRequirements(resolved)

	c.printf("added %s; %s now differs from %s",
		resolved.Name, config.LiveRecipeFile, basedOn(c.live))

	return nil
}

// reportRequirements names what the checked-out branch says it depends on and
// the workspace does not have.
//
// It is a hint, not an action: mudev assembles what it was asked for, and the
// developer decides what a dev site is composed of. Saying nothing at all would
// be worse, though — the requirement is written down in the catalogue, and a
// site that will not install is a slow way to find that out.
func (c *cloner) reportRequirements(p resolvedPlugin) {
	var missing []string

	for _, name := range p.Requires {
		if _, recorded := c.live.Plugin(name); !recorded {
			missing = append(missing, name)
		}
	}

	if len(missing) == 0 {
		return
	}

	c.stepf("%s declares it needs %s, which this workspace does not have",
		p.Name, strings.Join(missing, ", "))
}

// openWorkspace loads the live recipe as the recipe driving this run, and
// makes sure the tree underneath it is still the Moodle it claims to be.
func (c *cloner) openWorkspace() error {
	live, err := LoadLive(c.root)
	if err != nil {
		return err
	}

	if live == nil {
		return fmt.Errorf(
			"%s has no %s — assemble a workspace there with `mudev clone <recipe>` first",
			c.root, config.LiveRecipeFile,
		)
	}

	c.live = live

	r, err := recipe.Load(LivePath(c.root))
	if err != nil {
		return err
	}

	// A live recipe records no catalog of its own: every entry in it is already
	// flattened, so the only thing still needing a catalogue is the plugin
	// being added now, which resolves in the configured one.
	c.use(r, c.cfg.PluginsDir)

	// Same rule as clone: a plugin installed into a tree that is not the Moodle
	// it claims to be is just a directory.
	if err := c.verifyCore(); err != nil {
		return fmt.Errorf("moodle core: %w", err)
	}

	return nil
}

// pinnedRef is the entry overlay that pins a ref, in recipe spelling.
func pinnedRef(ref string) map[string]any {
	return map[string]any{
		"source": map[string]any{
			"git": map[string]any{
				"ref": ref,
			},
		},
	}
}

// basedOn names the recipe the workspace was assembled from, for the closing
// line. It is what `mudev recipe diff` will later compare against.
func basedOn(live *Live) string {
	if source, ok := live.BasedOnRecipe.(string); ok && source != "" {
		return source
	}

	return "its recipe"
}

// from names the file a definition was read from, for the progress line.
func from(file string) string {
	if file == "" {
		return ""
	}

	return " (" + file + ")"
}
