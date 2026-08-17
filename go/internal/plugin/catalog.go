package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Catalog is a vendor-grouped directory of plugin entries: the identifier
// mutms/tool_mulib maps straight onto <dir>/mutms/tool_mulib.yaml. Entries are
// cached, because one recipe asks for the same plugin more than once (a
// dependency of another plugin, for instance).
type Catalog struct {
	dir   string
	cache map[string]*Plugin
}

// NewCatalog returns a catalogue rooted at dir. The directory is not read
// until a plugin is actually requested.
func NewCatalog(dir string) *Catalog {
	return &Catalog{
		dir:   dir,
		cache: map[string]*Plugin{},
	}
}

// Dir is the catalogue root.
func (c *Catalog) Dir() string {
	return c.dir
}

// Get loads the entry for a vendor/package identifier.
func (c *Catalog) Get(name string) (*Plugin, error) {
	if p, ok := c.cache[name]; ok {
		return p, nil
	}

	path, err := c.Path(name)
	if err != nil {
		return nil, err
	}

	p, err := Load(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("plugin %s not found in catalogue %s", name, c.dir)
		}

		return nil, err
	}

	// A file whose contents disagree with its location would make the
	// identifier ambiguous, so treat it as a catalogue error.
	if p.Name != name {
		return nil, fmt.Errorf("%s: declares name %q but is filed as %q", path, p.Name, name)
	}

	c.cache[name] = p

	return p, nil
}

// Path maps an identifier to its file, accepting either YAML extension.
func (c *Catalog) Path(name string) (string, error) {
	vendor, pkg, ok := strings.Cut(name, "/")
	if !ok || vendor == "" || pkg == "" || strings.Contains(pkg, "/") {
		return "", fmt.Errorf("plugin identifier %q is not in vendor/package form", name)
	}

	base := filepath.Join(c.dir, vendor, pkg)

	yaml := base + ".yaml"
	if _, err := os.Stat(yaml); err == nil {
		return yaml, nil
	}

	yml := base + ".yml"
	if _, err := os.Stat(yml); err == nil {
		return yml, nil
	}

	// Return the canonical spelling; Load reports the not-found error.
	return yaml, nil
}
