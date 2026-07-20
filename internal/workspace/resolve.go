package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mutms/mudev/internal/git"
	"github.com/mutms/mudev/internal/moodle"
	"github.com/mutms/mudev/internal/plugin"
	"github.com/mutms/mudev/internal/recipe"
)

// resolvedPlugin is one recipe entry with everything mudev needs to check it
// out decided: where it goes, what to fetch it from, and which ref to take.
type resolvedPlugin struct {
	// Name is the vendor/package identifier.
	Name string

	// Path is the install path relative to the workspace root, with the
	// public/ prefix already stripped when the recipe asks for it.
	Path string

	// Remotes are all the remotes to configure, not just the clone one.
	Remotes map[string]string

	// Ref is what to check out: "<remote>/<branch>", or a tag/commit.
	Ref string

	// Localbranch is the local branch to create from a branch ref. Empty for a
	// tag or commit, which is checked out detached.
	Localbranch string

	// Release is the plugin's own release flavour, once the recipe-level
	// flavour has had its say (a mismatched one is dropped).
	Release string

	// Requires are the plugin identifiers the resolved branch declares it
	// depends on. mudev never acts on them — composing a site is the recipe's
	// job, or the developer's — but `recipe add` reports the ones a workspace
	// does not have, since Moodle will otherwise refuse the install later.
	Requires []string

	// Definition is the flattened entry to record in the live recipe.
	Definition map[string]any
}

// resolvePlugins turns the recipe's entries into install-ready descriptions,
// sorted into assembly order.
//
// The recipe's own order is for humans (it groups plugins by release); the
// assembly order is by path, ancestors first, so that a subplugin lands inside
// a parent plugin that is already there.
func (c *cloner) resolvePlugins() ([]resolvedPlugin, error) {
	resolved := make([]resolvedPlugin, 0, len(c.recipe.Plugins))

	for i := range c.recipe.Plugins {
		entry := c.recipe.Plugins[i]

		p, err := c.resolvePlugin(entry)
		if err != nil {
			return nil, fmt.Errorf("plugin %s: %w", entry.Name, err)
		}

		resolved = append(resolved, p)
	}

	sort.SliceStable(resolved, func(i, j int) bool {
		return resolved[i].Path < resolved[j].Path
	})

	return resolved, nil
}

// resolvePlugin flattens one entry against the catalogue and decides its ref.
func (c *cloner) resolvePlugin(entry recipe.Entry) (resolvedPlugin, error) {
	if entry.Name == "" {
		return resolvedPlugin{}, fmt.Errorf("entry has no name")
	}

	base, err := c.catalogueEntry(entry)
	if err != nil {
		return resolvedPlugin{}, err
	}

	return c.resolveEntry(entry, base)
}

// resolveEntry flattens one entry over an already-chosen base definition.
//
// The base is normally the catalogue file, but `mudev recipe add` may have been
// handed a plugin file directly — the flattening, path and branch resolution
// are the same either way.
func (c *cloner) resolveEntry(entry recipe.Entry, base map[string]any) (resolvedPlugin, error) {
	var out resolvedPlugin

	definition := deepMerge(base, deepCopy(entry.Raw))

	// Decode the flattened document into the plugin data model — that is what
	// it is, whether it came from the catalogue, the recipe, or both.
	def, err := decodePlugin(definition)
	if err != nil {
		return out, err
	}

	if def.Source == nil || def.Source.Git == nil || def.Source.Git.Remotes["origin"] == "" {
		return out, fmt.Errorf("no source.git.remotes.origin to clone from")
	}

	path, err := moodle.PluginPath(def.Relpath, c.recipe.Base.Strippublic)
	if err != nil {
		return out, err
	}

	remotes := def.Source.Git.Remotes

	ref := def.Source.Git.Ref

	if ref == "" {
		// No pin: resolve the branch that serves this recipe's Moodle version.
		branch, err := def.BranchFor(c.recipe.Base.Mdlbranch)
		if err != nil {
			return out, err
		}

		ref = "origin/" + branch
	}

	localbranch := localbranchOf(definition)

	var requires []string

	if _, branch, isBranch := git.SplitBranchRef(ref, remotes); isBranch {
		if localbranch == "" {
			localbranch = branch
		}

		// Dependencies are a property of a branch, not of the plugin, so they
		// are read from the branch that was actually resolved. A tag or commit
		// names no branch, so it declares no dependencies mudev can see.
		requires = def.Requirements[branch].Plugins
	} else {
		// A tag or commit has no branch to follow.
		localbranch = ""
	}

	narrowSourceToGit(definition, remotes, ref)

	return resolvedPlugin{
		Name:        def.Name,
		Path:        path,
		Remotes:     remotes,
		Ref:         ref,
		Localbranch: localbranch,
		Release:     c.pluginRelease(entry),
		Requires:    requires,
		Definition:  definition,
	}, nil
}

// catalogueEntry looks the plugin up in the catalogue, which supplies
// everything a bare reference leaves out.
//
// A self-contained entry does not go near the catalogue at all. That is what an
// exported recipe is made of — every field flattened inline — and consulting a
// local catalogue underneath it would let whatever happens to be installed on
// this machine add fields the recipe never asked for. A flattened recipe has to
// assemble the same tree everywhere, so it is used exactly as written.
func (c *cloner) catalogueEntry(entry recipe.Entry) (map[string]any, error) {
	if selfContained(entry) {
		return nil, nil
	}

	p, err := c.catalog.Get(entry.Name)
	if err != nil {
		return nil, err
	}

	return p.Raw, nil
}

// selfContained reports whether an entry already says everything mudev needs
// to check the plugin out: where it installs, and where the code comes from.
func selfContained(entry recipe.Entry) bool {
	if entry.Relpath == "" || entry.Source == nil || entry.Source.Git == nil {
		return false
	}

	return entry.Source.Git.Remotes["origin"] != ""
}

// pluginRelease applies the single-flavour rule: a plugin is release-managed
// only when its own flavour matches the recipe's. A plugin naming a different
// flavour has its flag dropped (never honoured under the other ruleset) and is
// reported — mixing tagging rulesets in one tree makes silent messes.
func (c *cloner) pluginRelease(entry recipe.Entry) string {
	flavour := entry.Release()

	switch {
	case flavour == "":
		return ""

	case c.flavour == "":
		c.warnf("plugin %s is flagged for release flavour %q but the recipe declares none — ignored",
			entry.Name, flavour)

		return ""

	case flavour != c.flavour:
		c.warnf("plugin %s names release flavour %q but the recipe is %q — ignored",
			entry.Name, flavour, c.flavour)

		return ""
	}

	return flavour
}

// decodePlugin reads a flattened definition into the plugin data model.
func decodePlugin(definition map[string]any) (*plugin.Plugin, error) {
	data, err := json.Marshal(definition)
	if err != nil {
		return nil, err
	}

	var p plugin.Plugin

	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode plugin definition: %w", err)
	}

	return &p, nil
}

// localbranchOf reads the local-branch override, which lives outside source
// because it is a local checkout ergonomic, not an acquisition detail.
func localbranchOf(definition map[string]any) string {
	name, _ := definition["localbranch"].(string)

	return name
}

// containingRepo finds the git repository a path will sit inside: the nearest
// ancestor that is itself a checkout — a parent plugin, or the core tree at
// the workspace root.
//
// The path is excluded there, so the surrounding repository does not report the
// nested checkout as untracked.
func containingRepo(root string, path string) (repo string, relative string) {
	parts := strings.Split(path, "/")

	for i := len(parts) - 1; i > 0; i-- {
		ancestor := root + "/" + strings.Join(parts[:i], "/")

		if info, err := os.Stat(ancestor); err != nil || !info.IsDir() {
			continue
		}

		if git.IsRepo(ancestor) {
			return ancestor, strings.Join(parts[i:], "/")
		}
	}

	return root, path
}
