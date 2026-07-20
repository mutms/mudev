package workspace

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/mutms/mudev/internal/config"
	"github.com/mutms/mudev/internal/git"
	"github.com/mutms/mudev/internal/moodle"
	"github.com/mutms/mudev/internal/recipe"
)

// CoreDir is how the Moodle core checkout — the workspace root itself — is
// identified in a listing, where every other row is a path relative to it.
const CoreDir = "."

// skipDirs are never descended into when looking for checkouts: they hold
// third-party code that is not part of the workspace composition.
var skipDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
}

// Repo is one checkout in a workspace, as a listing sees it: what the live
// recipe says it should be, and what git says it actually is.
type Repo struct {
	// Path identifies the row: the checkout's path relative to the workspace
	// root, exactly as it appears in the tree — including the public/ prefix
	// on Moodle 5.1 and later. The core checkout is CoreDir.
	Path string

	// Name is the plugin identifier from the live recipe; empty for the core
	// checkout and for anything not recorded there.
	Name string

	// Core marks the Moodle core checkout at the workspace root.
	Core bool

	// Managed reports that the live recipe records this checkout. An
	// unmanaged one is a repository someone cloned by hand — it is listed
	// precisely so it does not go unnoticed.
	Managed bool

	// Missing reports a checkout the live recipe records but which is not on
	// disk (an interrupted clone, or a directory someone removed).
	Missing bool

	// RecordedRef is the ref the live recipe says this checkout is on;
	// RecordedBranch is its branch part, when the ref names a branch.
	RecordedRef    string
	RecordedBranch string

	// Status is the live git state; zero for a missing checkout.
	Status git.Status

	// Version is what the checkout's version.php declares, if it has one.
	Version moodle.Version
}

// Strayed reports a checkout sitting on a different branch from the one the
// live recipe recorded — a feature branch left behind, or a manual switch.
func (r Repo) Strayed() bool {
	if r.Missing || r.RecordedBranch == "" || r.Status.Detached {
		return false
	}

	return r.Status.Branch != r.RecordedBranch
}

// ListOptions configure a listing.
type ListOptions struct {
	// Config is the resolved configuration.
	Config config.Config

	// Root is the workspace directory to inspect.
	Root string
}

// List reports every checkout in a workspace: the ones the live recipe
// records, plus any other git repository found in the tree.
//
// Both halves matter. The recipe's own plugins are what the workspace is
// supposed to contain — including any that are not there — while a repository
// nobody recorded is either a plugin waiting to be adopted into the recipe or
// something that should not be in the tree at all.
func List(ctx context.Context, opts ListOptions) ([]Repo, error) {
	ws, err := Enumerate(opts.Root)
	if err != nil {
		return nil, err
	}

	client := git.New(opts.Config)

	for i := range ws.Repos {
		if ws.Repos[i].Missing {
			continue
		}

		dir := filepath.Join(ws.Root, ws.Repos[i].Path)

		if ws.Repos[i].Status, err = client.Status(ctx, dir); err != nil {
			return nil, fmt.Errorf("%s: %w", ws.Repos[i].Path, err)
		}

		if ws.Repos[i].Version, err = moodle.ReadVersion(dir); err != nil {
			return nil, fmt.Errorf("%s: %w", ws.Repos[i].Path, err)
		}
	}

	return ws.Repos, nil
}

// Workspace is an assembled tree as mudev finds it on disk.
type Workspace struct {
	// Root is the absolute path of the workspace directory.
	Root string

	// Repos are its checkouts, core first and then by path.
	Repos []Repo

	// FetchOrder is the live recipe's extra.mudev.fetch_order — which remotes
	// to contact first, so a near mirror can spare the slow one the work.
	FetchOrder []string
}

// Enumerate finds the workspace's checkouts without asking git anything about
// them: what the live recipe records, plus whatever else is in the tree.
//
// It is the shared front half of every command that fans out over a workspace
// — list fills in each checkout's git state, while fetch and pull just need to
// know where the repositories are and in what order to contact their remotes.
func Enumerate(dir string) (*Workspace, error) {
	if !git.Available() {
		return nil, fmt.Errorf("git was not found on PATH")
	}

	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	recorded, order, err := recordedRepos(root)
	if err != nil {
		return nil, err
	}

	found, err := discover(root)
	if err != nil {
		return nil, err
	}

	// Anything on disk that the recipe does not know about is unmanaged.
	for _, path := range found {
		if _, ok := recorded[path]; !ok {
			recorded[path] = &Repo{Path: path, Core: path == CoreDir}
		}
	}

	repos := make([]Repo, 0, len(recorded))

	for path, repo := range recorded {
		// An unmanaged path can only come from the walk, so a recorded entry
		// that is not on disk is the only way to be missing.
		repo.Missing = repo.Managed && !contains(found, path)

		repos = append(repos, *repo)
	}

	sortRepos(repos)

	return &Workspace{Root: root, Repos: repos, FetchOrder: order}, nil
}

// recordedRepos reads the live recipe and maps each entry onto the path it
// occupies in the tree.
//
// A workspace with no live recipe is not an error: the listing still reports
// whatever checkouts are there, all of them unmanaged.
func recordedRepos(root string) (map[string]*Repo, []string, error) {
	repos := map[string]*Repo{}

	path := LivePath(root)

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return repos, nil, nil
		}

		return nil, nil, err
	}

	live, err := recipe.Load(path)
	if err != nil {
		return nil, nil, err
	}

	core := &Repo{Path: CoreDir, Core: true, Managed: true}

	if gs, err := live.Base.GitSource(); err == nil {
		core.RecordedRef = gs.Ref
		core.RecordedBranch = recordedBranch(gs.Ref, gs.Remotes, live.Base.Localbranch)
	}

	repos[CoreDir] = core

	for i := range live.Plugins {
		entry := live.Plugins[i]

		relpath, err := moodle.PluginPath(entry.Relpath, live.Base.Strippublic)
		if err != nil {
			return nil, nil, fmt.Errorf("plugin %s: %w", entry.Name, err)
		}

		repo := &Repo{
			Path:        relpath,
			Name:        entry.Name,
			Managed:     true,
			RecordedRef: entry.Ref(),
		}

		if entry.Source != nil && entry.Source.Git != nil {
			repo.RecordedBranch = recordedBranch(entry.Ref(), entry.Source.Git.Remotes, entry.Localbranch)
		}

		repos[relpath] = repo
	}

	return repos, live.FetchOrder(), nil
}

// recordedBranch works out which local branch a recorded ref implies, so a
// listing can tell a checkout that strayed onto another branch from one that
// is where the recipe put it. A tag or commit implies no branch.
func recordedBranch(ref string, remotes map[string]string, localbranch string) string {
	_, branch, isBranch := git.SplitBranchRef(ref, remotes)
	if !isBranch {
		return ""
	}

	if localbranch != "" {
		return localbranch
	}

	return branch
}

// discover walks the tree for git checkouts, including the root itself.
func discover(root string) ([]string, error) {
	var found []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory is not worth failing the whole listing.
			if path != root {
				return fs.SkipDir
			}

			return err
		}

		name := entry.Name()

		// .git may be a directory or, for a submodule or worktree, a file.
		if name == ".git" {
			repo, relErr := filepath.Rel(root, filepath.Dir(path))
			if relErr != nil {
				return relErr
			}

			found = append(found, filepath.ToSlash(repo))

			if entry.IsDir() {
				return fs.SkipDir
			}

			return nil
		}

		if entry.IsDir() && path != root && skipDirs[name] {
			return fs.SkipDir
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return found, nil
}

// sortRepos orders a listing: core first, then by path — which puts a
// subplugin directly under the plugin that contains it.
func sortRepos(repos []Repo) {
	sort.Slice(repos, func(i, j int) bool {
		if repos[i].Path == CoreDir || repos[j].Path == CoreDir {
			return repos[i].Path == CoreDir && repos[j].Path != CoreDir
		}

		return repos[i].Path < repos[j].Path
	})
}

// contains reports whether a path was found on disk.
func contains(paths []string, path string) bool {
	for _, candidate := range paths {
		if candidate == path {
			return true
		}
	}

	return false
}
