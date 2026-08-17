package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Open resolves a plugin argument and loads it.
//
// The argument is either a file path — an existing file, or anything ending in
// .yaml/.yml/.json, which makes a plugin that is not (yet) in the catalogue
// usable straight away — or a vendor/package catalogue identifier resolved
// under pluginsDir.
func Open(pluginsDir string, arg string) (*Plugin, error) {
	if arg == "" {
		return nil, fmt.Errorf("no plugin given")
	}

	if !isPathArg(arg) {
		return NewCatalog(pluginsDir).Get(arg)
	}

	path, err := filepath.Abs(arg)
	if err != nil {
		return nil, err
	}

	p, err := Load(path)
	if err != nil {
		return nil, err
	}

	// A file the user pointed at stands on its own, so it has to identify
	// itself: the identifier is the key the live recipe is written under.
	if p.Name == "" {
		return nil, fmt.Errorf("%s: no name — a plugin file must declare its vendor/package identifier", path)
	}

	return p, nil
}

// isPathArg reports whether the argument should be treated as a file path
// rather than a catalogue identifier.
//
// It mirrors the recipe package's rule rather than sharing it: plugin and
// recipe are independent leaves, and a few lines of duplication cost less than
// a dependency between them.
func isPathArg(arg string) bool {
	switch {
	case strings.HasSuffix(arg, ".yaml"), strings.HasSuffix(arg, ".yml"), strings.HasSuffix(arg, ".json"):
		return true
	case strings.HasPrefix(arg, "./"), strings.HasPrefix(arg, "../"), filepath.IsAbs(arg):
		return true
	}

	// A plain name that happens to exist on disk is a file too.
	if info, err := os.Stat(arg); err == nil && !info.IsDir() {
		return true
	}

	return false
}
