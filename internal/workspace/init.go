package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mutms/mudev/internal/config"
	"github.com/mutms/mudev/internal/git"
	"github.com/mutms/mudev/internal/moodle"
)

// identifierPattern is the vendor/package spelling the recipe schema accepts
// for a plugin name. `recipe init` builds each name by hand from on-disk facts,
// so it must produce something the schema will later validate on load.
var identifierPattern = regexp.MustCompile(`^[a-z0-9]([_.-]?[a-z0-9]+)*/[a-z0-9](([_.]?|-{0,2})[a-z0-9]+)*$`)

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
			"%s already has a %s — remove it first to reconstruct it from the checkouts",
			root, config.LiveRecipeFile,
		)
	} else if !os.IsNotExist(err) {
		return err
	}

	if !git.IsRepo(root) {
		return fmt.Errorf("%s is not a git checkout — a workspace root is Moodle core", root)
	}

	i := &initialiser{
		client: git.New(opts.Config),
		root:   root,
		out:    newOutput(opts.Out),
	}

	return i.run(ctx)
}

// initialiser carries the state of one init run.
type initialiser struct {
	client *git.Client
	root   string
	out    output

	// strippublic is read off the tree: core with a public/ code root is 5.1+,
	// without it an older branch. Every plugin relpath is stored in the newest
	// (public/) layout, so a stripped tree's paths get the prefix put back.
	strippublic bool
}

// run reconstructs and writes the live recipe.
func (i *initialiser) run(ctx context.Context) error {
	base, mdlbranch, err := i.base(ctx)
	if err != nil {
		return err
	}

	i.out.printf("reconstructing %s from the checkouts in %s", config.LiveRecipeFile, i.root)
	i.out.printf("base:    branch %s%s", mdlbranch, i.layoutNote())

	live := &Live{
		Name:    filepath.Base(i.root),
		Base:    base,
		Plugins: []map[string]any{},
	}

	// mudev's own state file is not part of Moodle, so keep it out of core's
	// git status — the same exclude clone writes.
	if err := git.AddExclude(i.root, "/"+config.LiveRecipeFile); err != nil {
		return err
	}

	paths, err := i.pluginPaths()
	if err != nil {
		return err
	}

	for _, path := range paths {
		entry, err := i.plugin(ctx, path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}

		if entry == nil {
			// Reported already; a checkout mudev cannot name is left out of the
			// record rather than recorded under a guessed identity.
			continue
		}

		live.SetPlugin(entry)

		repo, relative := containingRepo(i.root, path)

		if err := git.AddExclude(repo, "/"+relative); err != nil {
			return err
		}

		i.out.printf("%s  ->  %s", path, entry["name"])
	}

	if err := live.Save(i.root); err != nil {
		return err
	}

	i.out.printf("wrote %s: %d plugin(s)", config.LiveRecipeFile, len(live.Plugins))

	return nil
}

// base builds the recipe's base block from the core checkout at the root, and
// reports the Moodle branch it declares.
//
// The layout is read, not assumed: a public/ code root is Moodle 5.1+, its
// absence an older branch that wants strippublic. version.php at whichever of
// the two the tree actually has yields the $branch code.
func (i *initialiser) base(ctx context.Context) (map[string]any, string, error) {
	mdlbranch, err := moodle.Branch(i.root, false)
	if err != nil {
		if mdlbranch, err = moodle.Branch(i.root, true); err != nil {
			return nil, "", fmt.Errorf(
				"%s is not a Moodle core tree (no readable version.php with a $branch): %w",
				i.root, err,
			)
		}

		i.strippublic = true
	}

	remotes, err := i.client.Remotes(ctx, i.root)
	if err != nil {
		return nil, "", err
	}

	if remotes["origin"] == "" {
		return nil, "", fmt.Errorf("core checkout has no origin remote to record")
	}

	ref, localbranch, unborn, err := i.recordedRef(ctx, i.root, remotes)
	if err != nil {
		return nil, "", err
	}

	if unborn {
		return nil, "", fmt.Errorf("core checkout has no commit yet")
	}

	base := map[string]any{
		"mdlbranch": mdlbranch,
		"source":    map[string]any{"git": gitBlock(remotes, ref)},
	}

	if i.strippublic {
		base["strippublic"] = true
	}

	if localbranch != "" && localbranch != branchOf(ref, remotes) {
		base["localbranch"] = localbranch
	}

	return base, mdlbranch, nil
}

// plugin builds one flattened plugin entry from the checkout at path, or nil
// when the checkout cannot be identified or recorded (reported, not fatal — one
// odd checkout should not sink the reconstruction of the rest).
func (i *initialiser) plugin(ctx context.Context, path string) (map[string]any, error) {
	dir := filepath.Join(i.root, path)

	remotes, err := i.client.Remotes(ctx, dir)
	if err != nil {
		return nil, err
	}

	origin := remotes["origin"]
	if origin == "" {
		i.out.warnf("%s has no origin remote — skipped (add one, or `recipe add` it later)", path)

		return nil, nil
	}

	component, err := moodle.Component(dir)
	if err != nil {
		return nil, err
	}

	name, err := reconstructName(remoteOwner(origin), component, origin)
	if err != nil {
		i.out.warnf("%s — skipped: %v", path, err)

		return nil, nil
	}

	ref, localbranch, unborn, err := i.recordedRef(ctx, dir, remotes)
	if err != nil {
		return nil, err
	}

	if unborn {
		i.out.warnf("%s has no commit yet — skipped", path)

		return nil, nil
	}

	relpath := path
	if i.strippublic {
		relpath = moodle.PublicPrefix + path
	}

	entry := map[string]any{
		"name":    name,
		"relpath": relpath,
		"source":  map[string]any{"git": gitBlock(remotes, ref)},
	}

	if localbranch != "" && localbranch != branchOf(ref, remotes) {
		entry["localbranch"] = localbranch
	}

	return entry, nil
}

// pluginPaths lists the plugin checkouts under the root — every git checkout in
// the tree except core itself — ancestors first, so a subplugin is recorded
// after the plugin that contains it.
func (i *initialiser) pluginPaths() ([]string, error) {
	found, err := discover(i.root)
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

// recordedRef works out the ref and local branch to write down for a checkout,
// from the state git reports.
//
// A checkout on a branch is recorded by that branch's upstream ("origin/…"),
// the same remote-tracking spelling clone writes, so status can later tell a
// checkout that strayed onto another branch. A detached checkout on a tag is
// recorded by the tag; one on neither is recorded by its commit — the honest
// baseline for a tree pinned to a commit. A branch with no upstream is a local
// branch mudev cannot spell against a remote, so its commit is recorded and the
// developer is told what was assumed.
func (i *initialiser) recordedRef(ctx context.Context, dir string, remotes map[string]string) (ref string, localbranch string, unborn bool, err error) {
	st, err := i.client.Status(ctx, dir)
	if err != nil {
		return "", "", false, err
	}

	if st.Unborn {
		return "", "", true, nil
	}

	if st.Detached {
		if len(st.Tags) > 0 {
			return st.Tags[0], "", false, nil
		}

		sha, err := i.client.CommitSHA(ctx, dir)

		return sha, "", false, err
	}

	if st.Tracking != "" {
		return st.Tracking, st.Branch, false, nil
	}

	// On a branch with no upstream: there is no remote-tracking ref to point at,
	// so record the commit and say so, rather than invent an origin/<branch>
	// that may not exist.
	sha, err := i.client.CommitSHA(ctx, dir)
	if err != nil {
		return "", "", false, err
	}

	i.out.warnf("%s is on local branch %q with no upstream — recorded its commit; set a tracking branch and re-run to record it by branch",
		relOrDot(i.root, dir), st.Branch)

	return sha, "", false, nil
}

// layoutNote spells out the code-root layout, which decides every plugin path.
func (i *initialiser) layoutNote() string {
	if i.strippublic {
		return ", no public/ prefix"
	}

	return ", public/ layout"
}

// gitBlock is a source.git block: every remote the checkout has, and the ref to
// record. All remotes are kept, not just origin, so a fork's upstream or a LAN
// mirror is ready for fetch straight away.
func gitBlock(remotes map[string]string, ref string) map[string]any {
	named := make(map[string]any, len(remotes))
	for name, url := range remotes {
		named[name] = url
	}

	return map[string]any{"remotes": named, "ref": ref}
}

// branchOf is the branch part of a "<remote>/<branch>" ref, or empty for a tag
// or commit. It decides whether a localbranch needs recording at all: when it
// equals the local branch, the default already covers it.
func branchOf(ref string, remotes map[string]string) string {
	if _, branch, ok := git.SplitBranchRef(ref, remotes); ok {
		return branch
	}

	return ""
}

// reconstructName forms a vendor/package identifier from what the checkout can
// tell about itself: its frankenstyle component (the authoritative package
// name) under the owner of its origin remote (the vendor). Where the component
// is unreadable it falls back to the repository's own name.
func reconstructName(owner string, component string, origin string) (string, error) {
	pkg := component
	if pkg == "" {
		pkg = remoteRepo(origin)
	}

	vendor := strings.ToLower(owner)
	if vendor == "" {
		vendor = "local"
	}

	name := vendor + "/" + pkg

	if !identifierPattern.MatchString(name) {
		return "", fmt.Errorf(
			"cannot form a vendor/package name from component %q and remote owner %q",
			component, owner,
		)
	}

	return name, nil
}

// remotePath is the owner/repo path of a git URL, with any scheme, host,
// credentials and trailing ".git" removed. It handles the three shapes a remote
// takes: scheme URLs (https://, ssh://, file://), scp-like SSH
// (git@host:owner/repo), and bare local paths.
func remotePath(url string) string {
	u := strings.TrimSuffix(strings.TrimSpace(url), ".git")

	if idx := strings.Index(u, "://"); idx >= 0 {
		rest := u[idx+3:]

		if slash := strings.Index(rest, "/"); slash >= 0 {
			return strings.Trim(rest[slash+1:], "/")
		}

		return ""
	}

	if colon := strings.Index(u, ":"); colon >= 0 {
		return strings.Trim(u[colon+1:], "/")
	}

	return strings.Trim(u, "/")
}

// remoteOwner is the owner (vendor) segment of a git URL: the second-to-last
// path element, e.g. "mutms" in git@github.com:mutms/moodle-tool_mulib.git.
func remoteOwner(url string) string {
	parts := strings.Split(remotePath(url), "/")

	if len(parts) >= 2 && parts[len(parts)-2] != "" {
		return parts[len(parts)-2]
	}

	return ""
}

// remoteRepo is the repository name from a git URL — the last path element,
// with a leading "moodle-" dropped, since Moodle plugin repos are conventionally
// named moodle-<component>. It is the fallback package name when a checkout has
// no readable component.
func remoteRepo(url string) string {
	parts := strings.Split(remotePath(url), "/")

	return strings.TrimPrefix(parts[len(parts)-1], "moodle-")
}

// relOrDot renders dir relative to root for a message, falling back to "." for
// the root itself.
func relOrDot(root string, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return CoreDir
	}

	return filepath.ToSlash(rel)
}
