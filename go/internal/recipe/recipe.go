// Package recipe loads Moodle site definitions: which core to build from and
// which plugins to put on it.
//
// The same schema — and therefore the same types — describes a hand-authored
// source recipe (mdl-recipes/<vendor>/<stream>/<version>.yaml) and the live
// recipe mudev writes into a workspace (.mudev.json), the difference being
// that the live one has every plugin definition flattened inline.
//
// The plugin-shaped fields here (relpath, source, requirements) intentionally
// mirror internal/plugin rather than importing it: recipe and plugin are leaf
// packages and stay independent of each other. The merge between a catalogue
// entry and a recipe entry happens on the decoded documents (see
// internal/workspace), not by converting between the two struct types.
package recipe

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mutms/mudev/go/internal/schema"
)

// Recipe is a complete site definition.
type Recipe struct {
	// Name is a human label.
	Name string `json:"name,omitempty"`

	// Description is a one-line human summary.
	Description string `json:"description,omitempty"`

	// Catalog is where bare plugin references resolve; relative values are
	// anchored to the directory of the recipe file. Empty = the configured
	// plugins directory.
	Catalog string `json:"catalog,omitempty"`

	// ContributedBy is the courtesy-credit party (a name or a list; the
	// catalogues are CC0, so it is credit, not a license condition). On a
	// live recipe it identifies the project owner.
	ContributedBy any `json:"contributed_by,omitempty"`

	// BasedOnRecipe is live-recipe provenance: the identifier or file path
	// that `mudev clone` was given.
	BasedOnRecipe any `json:"based_on_recipe,omitempty"`

	// Extra is the composer-style, tool-namespaced bag. mudev reads exactly
	// one key from it: extra.mudev.release.
	Extra map[string]any `json:"extra,omitempty"`

	// Base is the code tree the plugins are installed into.
	Base Base `json:"base"`

	// Plugins are the entries to install, in the author's (human) order —
	// which is not the assembly order; mudev sorts by path.
	Plugins []Entry `json:"plugins"`

	// Raw is the complete decoded document, preserving fields mudev ignores.
	Raw map[string]any `json:"-"`

	// File is where this recipe was loaded from.
	File string `json:"-"`

	// Identifier is the catalogue identifier (vendor/stream/version) when the
	// recipe was named rather than given as a path.
	Identifier string `json:"-"`
}

// Base describes the checkout the plugins are installed into.
//
// It is deliberately not called "moodle": what a recipe names here is Moodle
// or a patched derivative of it, and MuTMS recipes point it at a pre-merged
// patch branch. Calling a modified tree "Moodle" would be wrong on the facts
// and wrong about the trademark.
type Base struct {
	// Mdlbranch is Moodle's $branch code as a string, e.g. "502". It drives
	// per-plugin branch resolution.
	Mdlbranch string `json:"mdlbranch"`

	// Source is how to acquire core; mudev reads the git kind.
	Source *Source `json:"source,omitempty"`

	// Localbranch is the local branch to create from a branch ref (default:
	// the remote branch name). Handy for the patched core, whose remote branch
	// is patch/mutms/MOODLE_502_STABLE but which reads better locally as
	// MOODLE_502_STABLE.
	Localbranch string `json:"localbranch,omitempty"`

	// Strippublic drops the leading public/ from every plugin path, for Moodle
	// branches older than 5.1.
	Strippublic bool `json:"strippublic,omitempty"`

	// Patches are extra branches to merge over the ref, in order. mudev does
	// not apply them yet — MuTMS ships a pre-merged core branch instead.
	Patches []Patch `json:"patches,omitempty"`

	// Raw is the decoded base block, preserved for the live recipe.
	Raw map[string]any `json:"-"`
}

// Patch is one extra branch merged over the core ref.
type Patch struct {
	Repo string `json:"repo"`
	Ref  string `json:"ref"`
}

// Source mirrors the plugin data model: acquisition kinds keyed by name.
type Source struct {
	Git      *GitSource `json:"git,omitempty"`
	Composer string     `json:"composer,omitempty"`
}

// GitSource names the remotes and the ref to check out.
type GitSource struct {
	Remotes map[string]string `json:"remotes,omitempty"`
	Ref     string            `json:"ref,omitempty"`
}

// Requirement mirrors the plugin data model: what one git branch serves and
// depends on. A recipe carries it only on flattened (inlined) entries.
type Requirement struct {
	Mdlbranches []string `json:"mdlbranches"`
	Plugins     []string `json:"plugins,omitempty"`
}

// Entry is one plugin in a recipe. It may be a bare identifier, a reference
// with overrides, or a complete inline definition — all three decode into this
// one type, with Raw holding whatever was written.
type Entry struct {
	Name         string                 `json:"name"`
	Relpath      string                 `json:"relpath,omitempty"`
	Localbranch  string                 `json:"localbranch,omitempty"`
	Source       *Source                `json:"source,omitempty"`
	Requirements map[string]Requirement `json:"requirements,omitempty"`

	// Extra is the per-plugin tool-namespaced bag; mudev reads
	// extra.mudev.release to decide whether the plugin is release-managed.
	Extra map[string]any `json:"extra,omitempty"`

	// Raw is the entry exactly as written (a bare string becomes {"name": …}).
	Raw map[string]any `json:"-"`
}

// UnmarshalJSON accepts both spellings the schema allows: a bare identifier
// string, or an object.
func (e *Entry) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))

	if strings.HasPrefix(trimmed, "\"") {
		var name string

		if err := json.Unmarshal(data, &name); err != nil {
			return err
		}

		*e = Entry{
			Name: name,
			Raw:  map[string]any{"name": name},
		}

		return nil
	}

	// A named type avoids recursing back into this method.
	type entry Entry

	var decoded entry

	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	*e = Entry(decoded)

	if err := json.Unmarshal(data, &e.Raw); err != nil {
		return err
	}

	return nil
}

// Load reads, schema-validates and decodes a recipe file (YAML or the JSON
// live recipe).
func Load(path string) (*Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	r, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	r.File = path

	return r, nil
}

// Parse decodes recipe data that has already been read.
func Parse(data []byte) (*Recipe, error) {
	doc, jsonBytes, err := schema.Decode(data)
	if err != nil {
		return nil, err
	}

	if err := schema.Validate(schema.KindRecipe, doc); err != nil {
		return nil, err
	}

	var r Recipe

	if err := json.Unmarshal(jsonBytes, &r); err != nil {
		return nil, fmt.Errorf("decode recipe: %w", err)
	}

	raw, ok := doc.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("decode recipe: expected a mapping at the top level")
	}

	r.Raw = raw

	if base, ok := raw["base"].(map[string]any); ok {
		r.Base.Raw = base
	}

	return &r, nil
}

// ReleaseFlavour returns extra.mudev.release from a tool-namespaced bag — the
// flavour name naming the project's release ruleset. An empty result means the
// recipe (or plugin) is not release-managed.
func ReleaseFlavour(extra map[string]any) string {
	mudev, ok := extra["mudev"].(map[string]any)
	if !ok {
		return ""
	}

	flavour, ok := mudev["release"].(string)
	if !ok {
		return ""
	}

	return flavour
}

// Release is the recipe-level release flavour: it names what project this
// workspace is, and so which release ruleset applies to it.
func (r *Recipe) Release() string {
	return ReleaseFlavour(r.Extra)
}

// FetchOrder returns extra.mudev.fetch_order: the remote names to contact
// first, in that order.
//
// mudev fetches every remote a checkout has. The order matters when one of
// them is faster than the others — a mirror on the local network, say. Git
// objects are content-addressed, so priming a repository from the near copy
// first leaves the slow remote with only the difference to send, which for a
// full Moodle core is the difference between a LAN copy and a 1.4 GB download.
//
// Remotes not named here are still fetched, after the ones that are. Names
// that a particular checkout does not have are skipped — a plugin catalogue
// entry usually has only origin, while a development recipe adds mirrors.
func FetchOrder(extra map[string]any) []string {
	mudev, ok := extra["mudev"].(map[string]any)
	if !ok {
		return nil
	}

	listed, ok := mudev["fetch_order"].([]any)
	if !ok {
		return nil
	}

	order := make([]string, 0, len(listed))

	for _, name := range listed {
		if remote, ok := name.(string); ok && remote != "" {
			order = append(order, remote)
		}
	}

	return order
}

// FetchOrder is the recipe-level remote fetch order, applied to every checkout
// in the workspace.
func (r *Recipe) FetchOrder() []string {
	return FetchOrder(r.Extra)
}

// Release is the plugin-level release flag: its presence selects the plugin as
// release-managed, under the recipe's flavour.
func (e *Entry) Release() string {
	return ReleaseFlavour(e.Extra)
}

// Ref returns the git ref this entry pins, if any. An empty result means the
// branch is resolved from the plugin's requirements at the recipe's mdlbranch.
func (e *Entry) Ref() string {
	if e.Source == nil || e.Source.Git == nil {
		return ""
	}

	return e.Source.Git.Ref
}

// GitSource returns the core's git acquisition block, or an error when the
// recipe advertises no git source (mudev assembles from git only).
func (b *Base) GitSource() (*GitSource, error) {
	if b.Source == nil || b.Source.Git == nil {
		return nil, fmt.Errorf("base.source has no git kind — mudev assembles from git")
	}

	if b.Source.Git.Remotes["origin"] == "" {
		return nil, fmt.Errorf("base.source.git.remotes.origin is required")
	}

	return b.Source.Git, nil
}
