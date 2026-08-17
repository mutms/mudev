package workspace

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mutms/mudev/go/internal/config"
)

// schemaURL is the published recipe schema, written as the first line of an
// exported file so an editor validates and completes it the same way it does
// the hand-authored recipes in the catalogues.
const schemaURL = "https://raw.githubusercontent.com/mutms/mudev/main/go/schema/recipe.schema.json"

// yamlIndent matches the catalogue files, which are written by hand and read
// far more often than they are edited.
const yamlIndent = 2

// ExportOptions configure writing a workspace's recipe out as YAML.
type ExportOptions struct {
	// Root is the workspace to export.
	Root string

	// File is where to write it. Empty means standard output.
	File string

	// Sort orders the plugin entries by install path instead of keeping the
	// order the workspace was assembled in.
	Sort bool

	// Out receives the recipe when File is empty, and mudev's own confirmation
	// line when it is not.
	Out io.Writer
}

// Export writes the workspace's live recipe out as recipe YAML.
//
// It is a straight rendering of .mudev.json, which is already a recipe document
// — every plugin flattened, every source narrowed to the git kind that was
// used. That is what makes the result portable: it carries no dependency on a
// plugin catalogue, so it describes the same tree wherever it is taken.
//
// What it exports is what the workspace was *assembled* to be, not the state
// its checkouts happen to be in now. A branch that has moved on, or a checkout
// somebody switched by hand, is `mudev list`'s business — pinning the tree to
// exact commits would be a different command, and a different promise.
func Export(opts ExportOptions) error {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}

	if err := checkYAMLName(opts.File); err != nil {
		return err
	}

	live, err := LoadLive(root)
	if err != nil {
		return err
	}

	if live == nil {
		return fmt.Errorf(
			"%s has no %s — there is nothing to export", root, config.LiveRecipeFile,
		)
	}

	if opts.Sort {
		// On a copy: the exported order is a rendering choice, and .mudev.json
		// keeps recording the order the tree was assembled in.
		sorted := *live
		sorted.Plugins = byRelpath(live.Plugins)
		live = &sorted
	}

	document, err := render(live)
	if err != nil {
		return err
	}

	out := newOutput(opts.Out)

	if opts.File == "" {
		_, err := opts.Out.Write(document)

		return err
	}

	// Say so rather than replacing a file silently: the target is quite likely
	// a recipe somebody wrote by hand, and comments do not survive the trip.
	replaced := false

	if _, err := os.Stat(opts.File); err == nil {
		replaced = true
	}

	if err := os.WriteFile(opts.File, document, 0o644); err != nil {
		return err
	}

	verb := "wrote"
	if replaced {
		verb = "replaced"
	}

	out.printf("%s %s (%d plugin(s))", verb, opts.File, len(live.Plugins))

	return nil
}

// byRelpath returns the plugin entries ordered by where they install, which is
// how a reader looks a plugin up in a long recipe — and it makes two exports of
// the same set of plugins diff cleanly, whatever order each tree was assembled
// in. The natural order is assembly order, so this is what --sort asks for.
//
// An entry with no relpath (nothing mudev writes, but a recipe is hand-editable)
// sorts under its name instead, so the result stays deterministic.
func byRelpath(entries []map[string]any) []map[string]any {
	sorted := make([]map[string]any, len(entries))

	copy(sorted, entries)

	sort.SliceStable(sorted, func(i, j int) bool {
		left, right := sortKey(sorted[i]), sortKey(sorted[j])

		if left != right {
			return left < right
		}

		return field(sorted[i], "name") < field(sorted[j], "name")
	})

	return sorted
}

// sortKey is the path an entry installs at, falling back to its identifier.
func sortKey(entry map[string]any) string {
	if relpath := field(entry, "relpath"); relpath != "" {
		return relpath
	}

	return field(entry, "name")
}

// field reads one string field of a decoded entry, empty if it is absent or is
// not a string.
func field(entry map[string]any, key string) string {
	value, _ := entry[key].(string)

	return value
}

// render turns the live recipe into the bytes of a recipe file.
func render(live *Live) ([]byte, error) {
	// Through JSON first, so what is written is exactly the document that was
	// validated on load — the same normalisation every other reader gets.
	data, err := json.Marshal(live)
	if err != nil {
		return nil, err
	}

	var document any

	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}

	node, err := ordered(document)
	if err != nil {
		return nil, err
	}

	var body bytes.Buffer

	encoder := yaml.NewEncoder(&body)
	encoder.SetIndent(yamlIndent)

	if err := encoder.Encode(node); err != nil {
		return nil, err
	}

	if err := encoder.Close(); err != nil {
		return nil, err
	}

	return append([]byte("# yaml-language-server: $schema="+schemaURL+"\n"), body.Bytes()...), nil
}

// keyOrder is the order an exported recipe writes its keys in: what a reader
// wants first, first.
//
// yaml.v3 sorts map keys alphabetically, which for a plugin entry buries the
// name somewhere after `license` — tolerable in a file nobody reads, but an
// exported recipe is meant to be committed, reviewed and edited by hand. One
// list serves every level of the document: a recipe, the base block, a plugin
// entry and a git source each use the subset of it that applies to them —
// which is why a git source reads `remotes` before `ref`, where it comes from
// before which part of it to check out, rather than alphabetically.
var keyOrder = []string{
	"name",
	"title",
	"description",
	"contributed_by",
	"homepage",
	"license",
	"based_on_recipe",
	"catalog",
	"mdlbranch",
	"strippublic",
	"relpath",
	"localbranch",
	"source",
	"remotes",
	"ref",
	"requirements",
	"patches",
	"extra",
	"base",
	"plugins",
}

// ordered renders a decoded document as YAML nodes, putting mapping keys in
// keyOrder and leaving anything unlisted in alphabetical order behind them.
func ordered(value any) (*yaml.Node, error) {
	switch typed := value.(type) {
	case map[string]any:
		node := &yaml.Node{Kind: yaml.MappingNode}

		for _, key := range orderKeys(typed) {
			name := &yaml.Node{}

			if err := name.Encode(key); err != nil {
				return nil, err
			}

			child, err := ordered(typed[key])
			if err != nil {
				return nil, err
			}

			node.Content = append(node.Content, name, child)
		}

		return node, nil

	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode}

		for _, item := range typed {
			child, err := ordered(item)
			if err != nil {
				return nil, err
			}

			node.Content = append(node.Content, child)
		}

		return node, nil

	default:
		node := &yaml.Node{}

		if err := node.Encode(value); err != nil {
			return nil, err
		}

		return node, nil
	}
}

// orderKeys sorts one mapping's keys: the ones mudev knows in keyOrder, then
// everything else alphabetically — a catalogue field mudev does not model is
// still carried, it just does not get to jump the queue.
func orderKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))

	for key := range m {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(i, j int) bool {
		left, right := rank(keys[i]), rank(keys[j])

		if left != right {
			return left < right
		}

		return keys[i] < keys[j]
	})

	return keys
}

// rank is a key's position in keyOrder; anything unlisted sorts after all of
// them.
func rank(key string) int {
	for i, known := range keyOrder {
		if known == key {
			return i
		}
	}

	return len(keyOrder)
}

// checkYAMLName rejects an output name that is not a recipe file. A recipe
// written to some other extension is one an editor will not validate and the
// catalogues will not accept, so the mistake is worth catching at the flag.
func checkYAMLName(file string) error {
	if file == "" {
		return nil
	}

	switch strings.ToLower(filepath.Ext(file)) {
	case ".yaml", ".yml":
		return nil
	}

	return fmt.Errorf("%s: a recipe file must be named .yaml or .yml", file)
}
