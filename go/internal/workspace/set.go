package workspace

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mutms/mudev/go/internal/config"
)

// settableKeys are the live recipe's own descriptive fields — the ones that
// say what this workspace is, rather than what it contains.
//
// Everything else in a live recipe is a record of something mudev did: the
// plugin entries, the base block, the refs. Those change by cloning, adding
// and pruning, because changing the text without changing the tree would just
// be a lie that survives an export.
var settableKeys = []string{"name", "description", "contributed_by"}

// SetOptions configure changing one field of a live recipe.
type SetOptions struct {
	// Root is the workspace whose recipe is being changed.
	Root string

	// Key is the field to set; Value is what to set it to, empty to clear it.
	Key   string
	Value string

	// Out receives mudev's own confirmation line.
	Out io.Writer
}

// Set changes one descriptive field of the workspace's live recipe.
//
// A workspace starts out describing the recipe it was cloned from, which stops
// being true the moment plugins are added or pruned — an export then carries a
// name and description belonging to something else. This is how a workspace
// gets named as the thing it has become.
//
// One key and one value per call, like `git config user.name "…"`, and an
// empty value clears the field.
func Set(opts SetOptions) error {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}

	live, err := LoadLive(root)
	if err != nil {
		return err
	}

	if live == nil {
		return fmt.Errorf(
			"%s has no %s — there is no recipe to change", root, config.LiveRecipeFile,
		)
	}

	previous, err := assign(live, opts.Key, opts.Value)
	if err != nil {
		return err
	}

	if err := live.Save(root); err != nil {
		return err
	}

	out := newOutput(opts.Out)

	switch {
	case opts.Value == "":
		out.printf("%s cleared (was %s)", opts.Key, quoted(previous))

	case previous == "":
		out.printf("%s set to %s", opts.Key, quoted(opts.Value))

	default:
		out.printf("%s set to %s (was %s)", opts.Key, quoted(opts.Value), quoted(previous))
	}

	return nil
}

// assign writes one field and reports what was there before.
func assign(live *Live, key string, value string) (previous string, err error) {
	switch key {
	case "name":
		previous, live.Name = live.Name, value

	case "description":
		previous, live.Description = live.Description, value

	case "contributed_by":
		previous = text(live.ContributedBy)

		if value == "" {
			live.ContributedBy = nil
		} else {
			live.ContributedBy = value
		}

	case "based_on_recipe":
		// Provenance, and the courtesy credit to the recipe this workspace
		// was adapted from. Rewriting it would misattribute somebody's work.
		return "", fmt.Errorf(
			"based_on_recipe records where this workspace came from and credits it — it is not editable",
		)

	default:
		return "", fmt.Errorf("unknown key %q — mudev sets: %s", key, strings.Join(settableKeys, ", "))
	}

	return previous, nil
}

// text renders a field that the schema allows to be a string or a list, for
// the "was …" half of the confirmation.
func text(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""

	case string:
		return typed

	case []any:
		parts := make([]string, 0, len(typed))

		for _, item := range typed {
			parts = append(parts, fmt.Sprint(item))
		}

		return strings.Join(parts, ", ")

	default:
		return fmt.Sprint(value)
	}
}

// quoted renders a value for the confirmation line, so an empty or
// space-padded one is visible rather than invisible.
func quoted(value string) string {
	if value == "" {
		return "nothing"
	}

	return strconv.Quote(value)
}
