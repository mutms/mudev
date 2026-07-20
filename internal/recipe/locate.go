package recipe

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Open resolves a recipe argument and loads it.
//
// The argument is either a file path — an existing file, or anything ending in
// .yaml/.yml, which makes ad-hoc private recipes ("somecustomer-4.5.12.yaml")
// work without a catalogue — or a catalogue identifier vendor/stream/version
// resolved under recipesDir.
func Open(recipesDir string, arg string) (*Recipe, error) {
	path, identifier, err := Locate(recipesDir, arg)
	if err != nil {
		return nil, err
	}

	r, err := Load(path)
	if err != nil {
		return nil, err
	}

	r.Identifier = identifier

	return r, nil
}

// Locate maps a recipe argument onto a file. The returned identifier is empty
// when the argument was a path (in which case the path itself is the
// provenance recorded in the live recipe).
func Locate(recipesDir string, arg string) (path string, identifier string, err error) {
	if arg == "" {
		return "", "", fmt.Errorf("no recipe given")
	}

	if isPathArg(arg) {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return "", "", err
		}

		return abs, "", nil
	}

	id := strings.Trim(arg, "/")

	if strings.Count(id, "/") != 2 {
		return "", "", fmt.Errorf(
			"recipe %q is neither an existing file nor a vendor/stream/version identifier", arg,
		)
	}

	base := filepath.Join(recipesDir, filepath.FromSlash(id))

	for _, candidate := range []string{base + ".yaml", base + ".yml"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, id, nil
		}
	}

	return "", "", fmt.Errorf("recipe %s not found in %s", id, recipesDir)
}

// isPathArg reports whether the argument should be treated as a file path
// rather than a catalogue identifier.
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
