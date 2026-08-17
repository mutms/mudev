// Package moodle holds the knowledge that is specific to a Moodle code tree:
// where plugins live in it, and what its version.php says.
//
// It is a leaf package of pure functions — acquiring the core checkout is git
// work, orchestrated by internal/workspace.
package moodle

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// PublicPrefix is the code-root subdirectory Moodle 5.1 introduced. Catalogue
// relpaths are stored in this newest layout.
const PublicPrefix = "public/"

// PluginPath resolves a catalogue relpath to the path inside a code tree.
//
// Relpaths are always stored in the newest layout (currently public/…), and
// older Moodle branches are served by *stripping* leading segments, never by
// prepending them — so a future layout change only ever adds a new prefix to
// strip, and the catalogue keeps one source of truth per plugin.
func PluginPath(relpath string, strippublic bool) (string, error) {
	clean := strings.Trim(filepath.ToSlash(relpath), "/")

	if clean == "" {
		return "", fmt.Errorf("plugin has an empty relpath")
	}

	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("relpath %q escapes the code root", relpath)
	}

	if strippublic {
		clean = strings.TrimPrefix(clean, PublicPrefix)
	}

	return clean, nil
}

// branchPattern matches Moodle's core version.php declaration, e.g.
//
//	$branch = '502';
var branchPattern = regexp.MustCompile(`\$branch\s*=\s*['"](\d+)['"]`)

// CoreVersionFile is where a code tree's own version.php lives, relative to
// the tree root.
//
// The layout is not guessed: a recipe already states it. Moodle 5.1 moved the
// code root under public/, and strippublic marks a recipe that targets an
// older branch without it — so the recipe's own answer decides which file is
// the right one to read, and a missing file at that exact path is a real
// problem rather than a reason to go looking elsewhere.
func CoreVersionFile(strippublic bool) string {
	if strippublic {
		return "version.php"
	}

	return filepath.Join(PublicPrefix, "version.php")
}

// Branch reads the $branch code from a code tree's version.php, so a checkout
// can be checked against the mdlbranch its recipe claims. strippublic must be
// the recipe's own value, so the file is read from exactly where that recipe
// says the code root is.
func Branch(dir string, strippublic bool) (string, error) {
	path := filepath.Join(dir, CoreVersionFile(strippublic))

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}

	match := branchPattern.FindSubmatch(content)
	if match == nil {
		return "", fmt.Errorf("%s declares no $branch — is this a Moodle code tree?", path)
	}

	return string(match[1]), nil
}
