// Package plugin loads Moodle plugin catalogue entries — the public, generic,
// stable facts about a plugin: its identifier, where it installs, where the
// code lives, and which git branch serves which Moodle version.
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/mutms/mudev/internal/schema"
)

// Plugin is one catalogue entry (mdl-plugins/<vendor>/<package>.yaml).
//
// The schema is deliberately open — a catalogue website may add presentation
// metadata mudev knows nothing about — so the whole decoded document is kept
// in Raw alongside the typed fields. Flattening a plugin into a live recipe
// copies Raw, which means unknown fields survive the round trip.
type Plugin struct {
	// Name is the composer-style vendor/package identifier and the map key
	// everywhere (e.g. "mutms/tool_mulib").
	Name string `json:"name"`

	// Title is the human display name for a catalogue UI.
	Title string `json:"title,omitempty"`

	// Description is a one-line human summary.
	Description string `json:"description,omitempty"`

	// Relpath is the install path relative to the Moodle code root, stored in
	// the newest (public/) layout; older branches strip leading segments.
	Relpath string `json:"relpath,omitempty"`

	// Source lists the acquisition kinds this plugin advertises. mudev reads
	// the git kind; a composer-based assembler would read composer.
	Source *Source `json:"source,omitempty"`

	Homepage string `json:"homepage,omitempty"`
	License  string `json:"license,omitempty"`

	// ContributedBy is the courtesy-credit party — a name or a list of them,
	// hence the open type. The catalogue is CC0; the credit is not a license
	// condition.
	ContributedBy any `json:"contributed_by,omitempty"`

	// Requirements maps a git branch name to what that branch serves and needs.
	Requirements map[string]Requirement `json:"requirements,omitempty"`

	// Raw is the complete decoded document, including fields mudev ignores.
	Raw map[string]any `json:"-"`

	// File is the path this entry was loaded from (empty when inline).
	File string `json:"-"`
}

// Source is a map of coexisting acquisition kinds, keyed by kind rather than
// tagged with a type: one entry can advertise several ways to fetch the same
// code, and each consumer reads the kind it supports.
type Source struct {
	// Git is the kind mudev uses.
	Git *GitSource `json:"git,omitempty"`

	// Composer is the Packagist package name (absence = not published there).
	Composer string `json:"composer,omitempty"`
}

// GitSource names where the code is and which version to take.
type GitSource struct {
	// Remotes maps a remote name to its URL; "origin" is the clone remote.
	Remotes map[string]string `json:"remotes,omitempty"`

	// Ref is what to check out: git's "<remote>/<branch>" spelling for a
	// branch, or a bare tag/commit for a pinned edition. Usually absent in the
	// catalogue and supplied (or resolved) per recipe.
	Ref string `json:"ref,omitempty"`
}

// Requirement describes one git branch of a plugin: the Moodle branches it
// serves, and the plugins it depends on there.
type Requirement struct {
	// Mdlbranches are Moodle $branch codes as strings, e.g. ["500","501"].
	Mdlbranches []string `json:"mdlbranches"`

	// Plugins are identifiers this branch depends on, resolved at the same
	// mdlbranch (the branch context supplies the version).
	Plugins []string `json:"plugins,omitempty"`
}

// Load reads, schema-validates and decodes a plugin catalogue file.
func Load(path string) (*Plugin, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	p, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	p.File = path

	return p, nil
}

// Parse decodes plugin YAML (or JSON) that has already been read.
func Parse(data []byte) (*Plugin, error) {
	doc, jsonBytes, err := schema.Decode(data)
	if err != nil {
		return nil, err
	}

	if err := schema.Validate(schema.KindPlugin, doc); err != nil {
		return nil, err
	}

	var p Plugin

	if err := json.Unmarshal(jsonBytes, &p); err != nil {
		return nil, fmt.Errorf("decode plugin: %w", err)
	}

	raw, ok := doc.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("decode plugin: expected a mapping at the top level")
	}

	p.Raw = raw

	return &p, nil
}

// BranchMap inverts Requirements into the lookup mudev actually needs:
// mdlbranch code → git branch name. The catalogue is keyed the other way round
// because a branch typically serves several Moodle versions (and because
// numeric object keys are unsafe across languages).
//
// A Moodle branch claimed by two git branches is an authoring error — mudev
// cannot pick — so it is reported rather than silently resolved.
func (p *Plugin) BranchMap() (map[string]string, error) {
	out := make(map[string]string, len(p.Requirements))

	// Branch names are walked in sorted order so a duplicate is reported the
	// same way on every run (Go map iteration order is random).
	branches := make([]string, 0, len(p.Requirements))
	for branch := range p.Requirements {
		branches = append(branches, branch)
	}
	sort.Strings(branches)

	for _, branch := range branches {
		for _, mdlbranch := range p.Requirements[branch].Mdlbranches {
			if other, clash := out[mdlbranch]; clash {
				return nil, fmt.Errorf(
					"plugin %s: Moodle branch %s is served by both %s and %s",
					p.Name, mdlbranch, other, branch,
				)
			}

			out[mdlbranch] = branch
		}
	}

	return out, nil
}

// BranchFor resolves the git branch serving the given Moodle $branch code.
// The pick is advisory — Moodle validates compatibility at install time, and a
// recipe that pins source.git.ref skips resolution entirely.
func (p *Plugin) BranchFor(mdlbranch string) (string, error) {
	branches, err := p.BranchMap()
	if err != nil {
		return "", err
	}

	branch, ok := branches[mdlbranch]
	if !ok {
		return "", fmt.Errorf(
			"plugin %s: no branch declared for Moodle branch %s (has: %s)",
			p.Name, mdlbranch, strings.Join(sortedKeys(branches), ", "),
		)
	}

	return branch, nil
}

// sortedKeys returns the map's keys in a stable order, for error messages.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))

	for k := range m {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}
