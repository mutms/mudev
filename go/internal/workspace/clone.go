// Package workspace assembles a Moodle code tree from a recipe: core first,
// then every plugin as its own git checkout at its resolved path.
//
// Assembly is deliberately incremental and idempotent. Each step that touches
// git records itself in the live recipe (.mudev.json) before the next one
// starts, and every step first asks whether it has already been done. So an
// interrupted clone is resumed simply by running the same command again, and a
// recipe that has grown a new plugin is caught up the same way — nothing that
// already exists in the tree is disturbed.
package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mutms/mudev/go/internal/config"
	"github.com/mutms/mudev/go/internal/git"
	"github.com/mutms/mudev/go/internal/moodle"
	"github.com/mutms/mudev/go/internal/plugin"
	"github.com/mutms/mudev/go/internal/recipe"
)

// knownFlavours are the release rulesets compiled into this build. A recipe
// naming anything else still assembles; only its release automation is inert.
var knownFlavours = []string{"mutms"}

// KnownFlavours returns the release flavours compiled into this build, for
// `mudev --version` — the place people look to identify a binary, and so the
// place that answers "does this mudev know my project's release rules?".
func KnownFlavours() []string {
	return append([]string(nil), knownFlavours...)
}

// Options configure a clone run.
type Options struct {
	// Config is the resolved configuration (the catalogue directories).
	Config config.Config

	// Recipe is what the user asked for: a file path or a vendor/stream/version
	// catalogue identifier.
	Recipe string

	// Root is the workspace directory to assemble into. mudev never creates
	// it — the caller has already made it with the right owner and permissions.
	Root string

	// Out receives mudev's own progress lines. git's output goes straight to
	// the terminal, as usual.
	Out io.Writer
}

// cloner carries the state of one clone run.
type cloner struct {
	cfg     config.Config
	client  *git.Client
	catalog *plugin.Catalog
	recipe  *recipe.Recipe
	live    *Live
	root    string
	out     output

	// flavour is the recipe's release flavour, resolved once up front and used
	// for the whole run.
	flavour string

	// resuming records that a live recipe was already there — this run is
	// continuing or catching up an existing workspace, not starting one.
	resuming bool
}

// Clone assembles the recipe into the workspace root.
func Clone(ctx context.Context, opts Options) error {
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
	}

	if err := c.load(opts.Recipe); err != nil {
		return err
	}

	c.banner()

	if err := c.assembleBase(ctx); err != nil {
		return fmt.Errorf("base: %w", err)
	}

	plugins, err := c.resolvePlugins()
	if err != nil {
		return err
	}

	for _, p := range plugins {
		if err := c.assemblePlugin(ctx, p); err != nil {
			return fmt.Errorf("plugin %s: %w", p.Name, err)
		}
	}

	c.printf("workspace ready: %d plugin(s) in %s", len(plugins), c.root)

	return nil
}

// load reads the recipe, resolves the release flavour, opens the catalogue and
// picks up any live recipe an earlier run left behind.
func (c *cloner) load(arg string) error {
	r, err := recipe.Open(c.cfg.RecipesDir, arg)
	if err != nil {
		return err
	}

	if len(r.Base.Patches) > 0 {
		return fmt.Errorf(
			"%s: base.patches is not implemented — point base.source at a pre-merged core branch",
			r.File,
		)
	}

	catalogDir := c.cfg.PluginsDir

	if r.Catalog != "" {
		// A path inside a YAML file is anchored to that file's directory.
		anchored, err := config.Anchor(r.File, r.Catalog)
		if err != nil {
			return err
		}

		catalogDir = anchored
	}

	c.use(r, catalogDir)

	return c.loadLive()
}

// use settles the run around one recipe document: its release flavour, the git
// client that flavour implies, and where bare plugin references resolve.
//
// Both entry points come through here — `clone`, from the source recipe it was
// given, and `recipe add`, from the live recipe already in the workspace — so
// the flavour is decided in exactly one place, before any git write.
func (c *cloner) use(r *recipe.Recipe, catalogDir string) {
	c.recipe = r

	c.flavour = r.Release()

	if c.flavour != "" && !known(c.flavour) {
		c.warnf("unknown release flavour %q — this build supports: %s; release commands are unavailable",
			c.flavour, strings.Join(knownFlavours, ", "))
	}

	c.client = git.New(c.cfg)

	c.catalog = plugin.NewCatalog(catalogDir)
}

// loadLive picks up an existing live recipe, or starts a fresh one.
func (c *cloner) loadLive() error {
	live, err := LoadLive(c.root)
	if err != nil {
		return err
	}

	source := c.source()

	if live != nil {
		// Assembling a different recipe into an existing workspace would mix
		// two trees; switching between recipes is a separate operation.
		if existing, ok := live.BasedOnRecipe.(string); ok && existing != source {
			return fmt.Errorf(
				"%s was assembled from %s, not %s — refusing to mix recipes in one workspace",
				LivePath(c.root), existing, source,
			)
		}

		c.live = live
		c.resuming = true

		return nil
	}

	c.live = &Live{
		Name:          c.recipe.Name,
		Description:   c.recipe.Description,
		BasedOnRecipe: source,
		Extra:         c.recipe.Extra,
		Plugins:       []map[string]any{},
	}

	return nil
}

// banner states what this run is about to build, before any git traffic.
//
// It exists because a fetch of a million objects tells a developer nothing
// about which recipe, which Moodle, or which remote they are waiting for.
func (c *cloner) banner() {
	name := c.recipe.Name
	if name == "" {
		name = c.source()
	}

	c.printf("recipe:      %s (%s)", name, c.source())
	c.printf("base:        branch %s%s", c.recipe.Base.Mdlbranch, c.layoutNote())

	if c.flavour != "" {
		c.printf("release:     %s", c.flavour)
	}

	if order := c.recipe.FetchOrder(); len(order) > 0 {
		c.printf("fetch order: %s (remotes not listed are fetched after these)",
			strings.Join(order, ", "))
	}

	c.printf("plugins:     %d", len(c.recipe.Plugins))

	if c.resuming {
		c.printf("continuing the workspace already recorded in %s", config.LiveRecipeFile)
	}
}

// layoutNote spells out the code-root layout, since it decides every plugin
// path in the tree.
func (c *cloner) layoutNote() string {
	if c.recipe.Base.Strippublic {
		return ", no public/ prefix"
	}

	return ", public/ layout"
}

// source is the provenance recorded in the live recipe: the catalogue
// identifier when the recipe was named, otherwise the file it came from.
func (c *cloner) source() string {
	if c.recipe.Identifier != "" {
		return c.recipe.Identifier
	}

	return c.recipe.File
}

// assembleBase puts Moodle core in the workspace root, verifies it, and
// records it.
//
// Nothing else in the workspace makes sense without a sound core — plugins
// installed into a tree that is not Moodle are just directories — so this step
// either leaves a verified checkout behind or fails the whole run.
func (c *cloner) assembleBase(ctx context.Context) error {
	gs, err := c.recipe.Base.GitSource()
	if err != nil {
		return err
	}

	ref := gs.Ref
	if ref == "" {
		return fmt.Errorf("base.source.git.ref is required")
	}

	switch {
	case !git.IsRepo(c.root):
		c.printf("base: new checkout at %s", ref)

		if err := c.client.Init(ctx, c.root); err != nil {
			return err
		}

		if err := c.acquire(ctx, c.root, gs.Remotes, ref, c.recipe.Base.Localbranch); err != nil {
			return err
		}

	case !c.client.HasHead(ctx, c.root):
		// A repository with no commit is an acquisition that was interrupted —
		// the fetch never finished. Finish it rather than mistaking the bare
		// .git directory for a checkout, which is how a workspace ends up with
		// plugins installed on top of nothing.
		c.printf("base: incomplete checkout — completing it at %s", ref)

		if err := c.acquire(ctx, c.root, gs.Remotes, ref, c.recipe.Base.Localbranch); err != nil {
			return err
		}

	default:
		c.printf("base: already checked out")

		if err := c.setRemotes(ctx, c.root, gs.Remotes); err != nil {
			return err
		}
	}

	// mudev's own state file is not part of Moodle, so keep it out of core's
	// git status.
	if err := git.AddExclude(c.root, "/"+config.LiveRecipeFile); err != nil {
		return err
	}

	if err := c.verifyBase(); err != nil {
		return err
	}

	c.live.Base = c.baseDefinition(gs, ref)

	return c.live.Save(c.root)
}

// baseDefinition is the base block as recorded in the live recipe: the
// recipe's own block, with source narrowed to the git kind that was used.
func (c *cloner) baseDefinition(gs *recipe.GitSource, ref string) map[string]any {
	definition := deepCopy(c.recipe.Base.Raw)

	definition["mdlbranch"] = c.recipe.Base.Mdlbranch

	narrowSourceToGit(definition, gs.Remotes, ref)

	return definition
}

// verifyBase checks that the workspace root really holds the Moodle the recipe
// asked for, by reading the code tree's own version.php.
//
// This is a hard failure, not a warning. Everything after it — the plugin
// paths, the layout, the branch each plugin resolves to — is derived from the
// recipe's claim about which Moodle this is, so a claim that does not hold
// means the rest of the run would be building something nobody asked for.
func (c *cloner) verifyBase() error {
	strippublic := c.recipe.Base.Strippublic

	branch, err := moodle.Branch(c.root, strippublic)
	if err != nil {
		return fmt.Errorf(
			"the workspace root is not a usable Moodle %s code tree: %w",
			c.recipe.Base.Mdlbranch, err,
		)
	}

	if branch != c.recipe.Base.Mdlbranch {
		return fmt.Errorf(
			"%s says this is Moodle branch %s, but the recipe builds branch %s",
			moodle.CoreVersionFile(strippublic), branch, c.recipe.Base.Mdlbranch,
		)
	}

	return nil
}

// assemblePlugin checks one plugin out at its path and records it.
func (c *cloner) assemblePlugin(ctx context.Context, p resolvedPlugin) error {
	dir := filepath.Join(c.root, p.Path)

	switch {
	case git.IsRepo(dir):
		// Already there: fix up the remotes, but never touch the working tree
		// — the developer may well be on a feature branch.
		c.printf("%s: already checked out", p.Path)

		if err := c.setRemotes(ctx, dir, p.Remotes); err != nil {
			return err
		}

	default:
		if _, err := os.Stat(dir); err == nil {
			return fmt.Errorf("%s exists but is not a git checkout", dir)
		}

		c.printf("%s: new checkout at %s", p.Path, p.Ref)

		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}

		if err := c.client.Init(ctx, dir); err != nil {
			return err
		}

		if err := c.acquire(ctx, dir, p.Remotes, p.Ref, p.Localbranch); err != nil {
			return err
		}

		// Site composition is mudev's job, but submodules inside a plugin are
		// that plugin's own business.
		if err := c.client.SubmoduleUpdate(ctx, dir); err != nil {
			return err
		}
	}

	repo, relative := containingRepo(c.root, p.Path)

	if err := git.AddExclude(repo, "/"+relative); err != nil {
		return err
	}

	c.live.SetPlugin(p.Definition)

	return c.live.Save(c.root)
}

// setRemotes configures every remote the entry names — not just the one code
// is cloned from, so `upstream` (a fork's source) and `backup` (a mirror) are
// ready to use, and a moved repository is corrected in place.
func (c *cloner) setRemotes(ctx context.Context, dir string, remotes map[string]string) error {
	for _, name := range sortedNames(remotes) {
		if err := c.client.SetRemote(ctx, dir, name, remotes[name]); err != nil {
			return err
		}
	}

	return nil
}

// acquire fills an initialised repository: remotes, one fetch, and a checkout
// of exactly the ref the recipe asked for.
//
// It deliberately does not use `git clone`, not even `clone -b <branch>`,
// which handles only two of the cases a recipe can name:
//
//   - a commit pin (a fork with no release tag) — `clone -b <sha>` fails
//     outright, leaving clone-then-checkout, which is what strands a stray
//     default branch in the checkout;
//   - Moodle core — its directory already exists (the caller made it, and by
//     now it also holds the live recipe, or a whole tree of plugins when an
//     interrupted run is being completed) and git refuses to clone into a
//     non-empty directory;
//   - a localbranch override — `clone -b patch/mutms/MOODLE_502_STABLE`
//     creates a local branch under that same prefixed name, so the recipe's
//     plain MOODLE_502_STABLE would need a rename afterwards.
//
// Fetching into an empty repository and creating exactly the one branch that
// belongs there covers all of them with a single code path, and never leaves
// behind a branch nobody asked for. The remote-tracking refs and the fetch
// refspec come out identical to a clone; only origin/HEAD is not set, which
// would cost an extra round trip to the remote for a cosmetic ref.
func (c *cloner) acquire(ctx context.Context, dir string, remotes map[string]string, ref string, localbranch string) error {
	if err := c.setRemotes(ctx, dir, remotes); err != nil {
		return err
	}

	_, branch, isBranch := git.SplitBranchRef(ref, remotes)

	// A pinned edition from a single remote needs one commit, not a decade
	// of history: Moodle core at a release tag is ~989 MB of .git full and
	// ~80 MB shallow. Three conditions, all of which must hold:
	//
	//   - the ref is a tag or commit, not a branch. A branch checkout is
	//     something a developer works in, and log/blame there must work.
	//   - exactly one remote. fetch_order exists for LAN mirrors, and a
	//     shallow repository deepened from a different remote than it was
	//     shallowed from is a bad time.
	//   - not a release workspace. That flavour tags, edits version.php and
	//     writes changelogs, all of which want the history and the tags.
	//
	// Nothing is recorded about it here: git knows (.git/shallow), and a
	// later fetch unshallows first (see fetchRemotes). Pull never meets a
	// shallow checkout — a shallowed pin is detached, so Pull skips it.
	shallow := false

	if !isBranch && len(remotes) == 1 && c.recipe.Release() == "" {
		for name := range remotes {
			c.stepf("fetch %s (shallow, %s only)", name, ref)

			if err := c.client.FetchShallowTag(ctx, dir, name, ref); err == nil {
				shallow = true
			} else {
				// Not a tag — a commit pin, which not every server will
				// serve shallowly. Fall through to the full fetch rather
				// than fail: a slower assembly beats none.
				c.out.warnf("shallow fetch of %s failed (%v) — fetching in full", ref, err)
			}
		}
	}

	if !shallow {
		if err := fetchRemotes(ctx, c.client, dir, c.recipe.FetchOrder(), c.out); err != nil {
			// Every remote is fetched, in the recipe's order — a near
			// mirror first leaves origin with only the difference to send.
			return err
		}
	}

	if !isBranch {
		// A tag or commit — a pinned edition — is checked out detached; there
		// is no branch it belongs on.
		c.stepf("checkout %s (detached)", ref)

		return c.client.CheckoutDetached(ctx, dir, ref)
	}

	if localbranch == "" {
		localbranch = branch
	}

	c.stepf("checkout %s tracking %s", localbranch, ref)

	return c.client.SwitchBranch(ctx, dir, localbranch, ref)
}

// known reports whether this build has a handler for a release flavour.
func known(flavour string) bool {
	for _, name := range knownFlavours {
		if name == flavour {
			return true
		}
	}

	return false
}

// sortedNames returns remote names in a stable order — alphabetical, but with
// "origin" first, since that is the remote code is cloned from.
func sortedNames(m map[string]string) []string {
	names := make([]string, 0, len(m))

	for name := range m {
		names = append(names, name)
	}

	sort.Strings(names)

	for i, name := range names {
		if name == "origin" {
			copy(names[1:i+1], names[:i])
			names[0] = "origin"

			break
		}
	}

	return names
}

// printf writes one of mudev's own progress lines.
func (c *cloner) printf(format string, args ...any) {
	c.out.printf(format, args...)
}

// stepf writes an indented line for one git operation within a checkout.
func (c *cloner) stepf(format string, args ...any) {
	c.out.stepf(format, args...)
}

// warnf reports something the user should know about but that does not stop
// the assembly.
func (c *cloner) warnf(format string, args ...any) {
	c.out.warnf(format, args...)
}
