package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mutms/mudev/internal/config"
	"github.com/mutms/mudev/internal/schema"
)

// Live is the live recipe — the single mudev-managed state file at the root of
// a workspace (.mudev.json).
//
// It is the composer.lock to a source recipe's composer.json: every plugin
// entry is flattened (the catalogue definition merged inline, its source
// narrowed to the git kind that was actually used, and source.git.ref set to
// what was checked out), so the file describes the whole tree on its own — no
// plugins directory, no catalog, wherever it travels.
//
// It is written *incrementally*: core first, then one plugin at a time in
// install order. An interrupted clone therefore leaves an accurate, if
// partial, record — and re-running clone simply continues from it.
//
// Plugin entries are kept as decoded documents rather than typed structs so
// that catalogue fields mudev does not model survive the flattening.
type Live struct {
	Name          string           `json:"name,omitempty"`
	Description   string           `json:"description,omitempty"`
	ContributedBy any              `json:"contributed_by,omitempty"`
	BasedOnRecipe any              `json:"based_on_recipe,omitempty"`
	Extra         map[string]any   `json:"extra,omitempty"`
	Base          map[string]any   `json:"base"`
	Plugins       []map[string]any `json:"plugins"`
}

// LivePath is the live recipe's location inside a workspace.
func LivePath(root string) string {
	return filepath.Join(root, config.LiveRecipeFile)
}

// LoadLive reads the live recipe from a workspace root. A missing file is not
// an error: it simply means the workspace has not been started yet, and nil is
// returned.
func LoadLive(root string) (*Live, error) {
	path := LivePath(root)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, err
	}

	doc, jsonBytes, err := schema.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	if err := schema.Validate(schema.KindRecipe, doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	var live Live

	if err := json.Unmarshal(jsonBytes, &live); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return &live, nil
}

// Plugin returns the recorded entry for an identifier, and whether it is there.
func (l *Live) Plugin(name string) (map[string]any, bool) {
	for _, entry := range l.Plugins {
		if entry["name"] == name {
			return entry, true
		}
	}

	return nil, false
}

// SetPlugin records an entry, replacing an earlier record of the same plugin.
// New entries append, so the file keeps the order the tree was assembled in.
func (l *Live) SetPlugin(entry map[string]any) {
	name := entry["name"]

	for i, existing := range l.Plugins {
		if existing["name"] == name {
			l.Plugins[i] = entry

			return
		}
	}

	l.Plugins = append(l.Plugins, entry)
}

// RemovePlugin drops an entry, reporting whether it was recorded at all.
func (l *Live) RemovePlugin(name string) bool {
	for i, entry := range l.Plugins {
		if entry["name"] == name {
			l.Plugins = append(l.Plugins[:i], l.Plugins[i+1:]...)

			return true
		}
	}

	return false
}

// Save writes the live recipe to the workspace root.
//
// The write goes through a temporary file in the same directory and a rename,
// so an interrupted run can never leave a half-written state file behind — the
// whole point of writing it step by step.
func (l *Live) Save(root string) error {
	if l.Plugins == nil {
		l.Plugins = []map[string]any{}
	}

	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}

	data = append(data, '\n')

	path := LivePath(root)

	tmp, err := os.CreateTemp(root, config.LiveRecipeFile+".*")
	if err != nil {
		return err
	}

	// Best effort: on any failure below the temporary file must not survive.
	defer func() {
		_ = os.Remove(tmp.Name())
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()

		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), path)
}
