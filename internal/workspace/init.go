package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/mutms/mudev/internal/config"
	"github.com/mutms/mudev/internal/git"
)

// InitOptions configure reconstructing a live recipe from an existing tree.
type InitOptions struct {
	// Config is the resolved configuration.
	Config config.Config

	// Root is the workspace directory to inspect. It must already be a Moodle
	// core checkout with plugin checkouts placed inside it.
	Root string

	// Out receives mudev's own progress lines.
	Out io.Writer
}

// Init writes a .mudev.json for a tree that already holds the checkouts, so a
// workspace assembled by hand (or by the old PHP tool) becomes one mudev can
// manage: list, status, fetch, pull, export.
//
// It is the reverse of clone, and it touches no working tree. Where clone reads
// a recipe and produces checkouts, init reads the checkouts and produces the
// recipe — asking git what each checkout is (its remotes, its branch and
// tracking ref) and reading each plugin's version.php for the frankenstyle
// component that names it. The result is a fully flattened, self-contained live
// recipe, exactly the shape clone would have written, so `recipe export` and
// every fan-out command work against it from then on.
//
// A plugin's identifier is reconstructed as <remote-owner>/<component>: the
// owner segment of its origin URL, and its own frankenstyle component. For a
// checkout cloned from git@github.com:mutms/moodle-tool_mulib.git that is
// mutms/tool_mulib — the catalogue identifier — and for a private forge
// (git@forge.example:acme/mod_thing.git) it is acme/mod_thing, which is the
// honest name for a tree whose code comes from that forge. It is mudev's own
// state key, editable afterwards with `recipe set`, not a claim about any
// catalogue.
//
// Once a tree is initialised, a single new or moved checkout is folded in one
// at a time with `recipe update <relpath>` rather than by re-running init.
func Init(ctx context.Context, opts InitOptions) error {
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

	// Refuse to overwrite an existing record: it may hold state (release flags,
	// fetch order, a hand-set name) that reconstruction cannot recover. Removing
	// it first is a deliberate act.
	if _, err := os.Stat(LivePath(root)); err == nil {
		return fmt.Errorf(
			"%s already has a %s — remove it first to reconstruct it, or use `mudev recipe update <relpath>` for one checkout",
			root, config.LiveRecipeFile,
		)
	} else if !os.IsNotExist(err) {
		return err
	}

	if !git.IsRepo(root) {
		return fmt.Errorf("%s is not a git checkout — a workspace root is Moodle core", root)
	}

	s := &scanner{
		client: git.New(opts.Config),
		root:   root,
		out:    newOutput(opts.Out),
	}

	return runInit(ctx, s)
}

// runInit scans the whole tree and writes the reconstructed live recipe.
func runInit(ctx context.Context, s *scanner) error {
	mdlbranch, err := s.detectLayout()
	if err != nil {
		return err
	}

	base, err := s.baseBlock(ctx, mdlbranch)
	if err != nil {
		return err
	}

	s.out.printf("reconstructing %s from the checkouts in %s", config.LiveRecipeFile, s.root)
	s.out.printf("base:    branch %s%s", mdlbranch, s.layoutNote())

	live := &Live{
		Name:    filepath.Base(s.root),
		Base:    base,
		Plugins: []map[string]any{},
	}

	// mudev's own state file is not part of Moodle, so keep it out of core's
	// git status — the same exclude clone writes.
	if err := git.AddExclude(s.root, "/"+config.LiveRecipeFile); err != nil {
		return err
	}

	paths, err := pluginPaths(s.root)
	if err != nil {
		return err
	}

	for _, path := range paths {
		entry, err := s.pluginEntry(ctx, path)
		if err != nil {
			// A checkout mudev cannot name or record is reported and left out,
			// rather than sinking the reconstruction of the rest. A real git
			// failure, though, is fatal.
			if errors.Is(err, errNoOrigin) || errors.Is(err, errUnborn) || errors.Is(err, errUnnameable) {
				s.out.warnf("%s %v — skipped", path, err)

				continue
			}

			return fmt.Errorf("%s: %w", path, err)
		}

		live.SetPlugin(entry)

		repo, relative := containingRepo(s.root, path)

		if err := git.AddExclude(repo, "/"+relative); err != nil {
			return err
		}

		s.out.printf("%s  ->  %s", path, entry["name"])
	}

	if err := live.Save(s.root); err != nil {
		return err
	}

	s.out.printf("wrote %s: %d plugin(s)", config.LiveRecipeFile, len(live.Plugins))

	return nil
}

// pluginPaths lists the plugin checkouts under the root — every git checkout in
// the tree except core itself — ancestors first, so a subplugin is recorded
// after the plugin that contains it.
func pluginPaths(root string) ([]string, error) {
	found, err := discover(root)
	if err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(found))

	for _, path := range found {
		if path != CoreDir {
			paths = append(paths, path)
		}
	}

	sort.Strings(paths)

	return paths, nil
}
