package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/mutms/mudev/go/internal/config"
	"github.com/mutms/mudev/go/internal/git"
	"github.com/mutms/mudev/go/internal/moodle"
)

// UpdateOptions configure folding one checkout's current state into the record.
type UpdateOptions struct {
	// Config is the resolved configuration.
	Config config.Config

	// Root is the workspace to update.
	Root string

	// Relpath names the checkout, as `mudev list` prints it: a path relative to
	// the workspace root, or "." for Moodle core.
	Relpath string

	// Out receives mudev's own progress lines.
	Out io.Writer
}

// Update folds the current state of one checkout into the live recipe: it
// adopts a checkout the record does not know yet, or refreshes one it does.
//
// It is the per-checkout companion to `recipe init`. Where init reconstructs a
// whole tree at once, update takes a single path — the one you just cloned into
// the tree, or the one whose branch or remotes you changed — and brings its
// entry in .mudev.json up to date. It reads the same on-disk facts init does
// (remotes, branch, tracking ref, the plugin's own component) and touches no
// working tree.
//
// A refresh keeps the recorded name and everything else about the entry; only
// the git identity — remotes, ref, localbranch — is rewritten to match the
// checkout. So a name you fixed by hand after init survives, while a checkout
// you moved onto a new branch is recorded where it now is (which is how a
// strayed `≠` row in `mudev list` becomes the baseline again). Removing a plugin
// stays `recipe prune`'s job — update never drops an entry.
func Update(ctx context.Context, opts UpdateOptions) error {
	if !git.Available() {
		return fmt.Errorf("git was not found on PATH")
	}

	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}

	relpath, err := cleanRelpath(opts.Relpath)
	if err != nil {
		return err
	}

	live, err := LoadLive(root)
	if err != nil {
		return err
	}

	if live == nil {
		return fmt.Errorf(
			"%s has no %s — reconstruct it with `mudev recipe init` first",
			root, config.LiveRecipeFile,
		)
	}

	s := &scanner{
		client: git.New(opts.Config),
		root:   root,
		out:    newOutput(opts.Out),
	}

	// detectLayout also confirms the root really is Moodle core, and sets the
	// public/ layout that decides how a plugin's relpath is spelled.
	mdlbranch, err := s.detectLayout()
	if err != nil {
		return err
	}

	if relpath == CoreDir {
		return s.updateBase(ctx, root, live, mdlbranch)
	}

	return s.updatePlugin(ctx, root, live, relpath)
}

// updateBase refreshes the base block from the core checkout.
func (s *scanner) updateBase(ctx context.Context, root string, live *Live, mdlbranch string) error {
	base, err := s.baseBlock(ctx, mdlbranch)
	if err != nil {
		return err
	}

	if reflect.DeepEqual(base, live.Base) {
		s.out.printf("core already matches the tree — nothing to record")

		return nil
	}

	old := gitRefOf(live.Base)
	live.Base = base

	if err := live.Save(root); err != nil {
		return err
	}

	s.out.printf("updated core: %s -> %s", old, gitRefOf(base))

	return nil
}

// updatePlugin adopts or refreshes the plugin checkout at relpath.
func (s *scanner) updatePlugin(ctx context.Context, root string, live *Live, relpath string) error {
	dir := filepath.Join(root, relpath)
	existing, recorded := recordedAtPath(live, relpath, s.strippublic)

	if !git.IsRepo(dir) {
		switch {
		case recorded:
			// The record has it but the tree does not: that is a removed plugin,
			// which `recipe prune` reconciles — update does not drop entries.
			return fmt.Errorf(
				"%s is recorded but not checked out — `mudev recipe prune` drops a removed plugin",
				relpath,
			)

		case dirExists(dir):
			return fmt.Errorf("%s exists but is not a git checkout", relpath)

		default:
			return fmt.Errorf("no checkout at %s", relpath)
		}
	}

	if recorded {
		return s.refreshPlugin(ctx, root, live, relpath, existing)
	}

	return s.adoptPlugin(ctx, root, live, relpath)
}

// refreshPlugin rewrites a recorded entry's git identity to match its checkout,
// leaving its name and every other field intact.
func (s *scanner) refreshPlugin(ctx context.Context, root string, live *Live, relpath string, existing map[string]any) error {
	remotes, ref, localbranch, err := s.identity(ctx, dirOf(root, relpath))
	if err != nil {
		return fmt.Errorf("%s %w", relpath, err)
	}

	// Start from the recorded entry so name, requirements, extra and any
	// catalogue fields survive; overwrite only the git kind and localbranch.
	candidate := deepCopy(existing)
	narrowSourceToGit(candidate, remotes, ref)
	applyLocalbranch(candidate, localbranch, ref, remotes)

	if reflect.DeepEqual(candidate, existing) {
		s.out.printf("%s already matches the tree — nothing to record", relpath)

		return nil
	}

	live.SetPlugin(candidate)

	if err := live.Save(root); err != nil {
		return err
	}

	s.out.printf("updated %s: %s -> %s", candidate["name"], gitRefOf(existing), ref)

	return nil
}

// adoptPlugin records a checkout the live recipe did not know about, deriving
// its identity the way init does and hiding it from its containing repository.
func (s *scanner) adoptPlugin(ctx context.Context, root string, live *Live, relpath string) error {
	entry, err := s.pluginEntry(ctx, relpath)
	if err != nil {
		return fmt.Errorf("%s %w", relpath, err)
	}

	live.SetPlugin(entry)

	repo, relative := containingRepo(root, relpath)

	if err := git.AddExclude(repo, "/"+relative); err != nil {
		return err
	}

	if err := live.Save(root); err != nil {
		return err
	}

	s.out.printf("adopted %s at %s (%s)", entry["name"], relpath, gitRefOf(entry))

	return nil
}

// recordedAtPath finds the recorded entry that occupies a tree path, resolving
// each entry's stored (public/) relpath through the tree's layout — the same
// mapping `mudev list` uses to print the path column, so the argument matches
// what the user reads there.
func recordedAtPath(live *Live, treePath string, strippublic bool) (map[string]any, bool) {
	for _, entry := range live.Plugins {
		relpath, _ := entry["relpath"].(string)
		if relpath == "" {
			continue
		}

		path, err := moodle.PluginPath(relpath, strippublic)
		if err != nil {
			continue
		}

		if path == treePath {
			return entry, true
		}
	}

	return nil, false
}

// gitRefOf reads source.git.ref from a base or plugin definition, for a report
// line. An entry with no git ref (there should be none in a live recipe) yields
// an empty string rather than a panic.
func gitRefOf(definition map[string]any) string {
	source, ok := definition["source"].(map[string]any)
	if !ok {
		return ""
	}

	kind, ok := source["git"].(map[string]any)
	if !ok {
		return ""
	}

	ref, _ := kind["ref"].(string)

	return ref
}

// cleanRelpath normalises the path argument and refuses one that escapes the
// workspace root.
func cleanRelpath(arg string) (string, error) {
	trimmed := strings.TrimSpace(arg)
	if trimmed == "" {
		return "", fmt.Errorf("a relpath is required (\".\" for Moodle core)")
	}

	clean := filepath.ToSlash(filepath.Clean(trimmed))

	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%s escapes the workspace root", arg)
	}

	return clean, nil
}

// dirOf joins a tree-relative path onto the root.
func dirOf(root string, relpath string) string {
	return filepath.Join(root, relpath)
}

// dirExists reports whether something is present at dir (of any kind).
func dirExists(dir string) bool {
	_, err := os.Stat(dir)

	return err == nil
}
