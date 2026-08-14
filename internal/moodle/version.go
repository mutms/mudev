package moodle

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Version is what a version.php declares about the code next to it.
type Version struct {
	// Version is the numeric version stamp ($plugin->version, or $version in
	// core) — MuTMS spells it as an ISO date plus a two-digit branch suffix.
	Version string

	// Release is the human release string ($plugin->release, or $release in
	// core), e.g. "v5.2.2.01".
	Release string
}

// Empty reports whether nothing was found.
func (v Version) Empty() bool {
	return v.Version == "" && v.Release == ""
}

// Patterns for the two shapes a version.php takes: a plugin's $plugin->…
// assignments, and core's plain $version/$release. Both are matched so that
// the core row of a listing is as informative as a plugin row.
var (
	pluginVersionPattern   = regexp.MustCompile(`\$plugin->version\s*=\s*(\d+(?:\.\d+)?)`)
	pluginReleasePattern   = regexp.MustCompile(`\$plugin->release\s*=\s*'([^']*)'`)
	pluginComponentPattern = regexp.MustCompile(`\$plugin->component\s*=\s*['"]([a-z][a-z0-9_]*)['"]`)
	coreVersionPattern     = regexp.MustCompile(`(?m)^\s*\$version\s*=\s*(\d+(?:\.\d+)?)`)
	coreReleasePattern     = regexp.MustCompile(`(?m)^\s*\$release\s*=\s*'([^']*)'`)
)

// Component reads a plugin's frankenstyle component name from its version.php,
// e.g. "mod_customcert" or "tool_certificate".
//
// It is the plugin's own authoritative identity — Moodle keys everything on it —
// so `mudev recipe init` uses it as the package half of a reconstructed
// identifier rather than guessing from the directory name. A directory with no
// version.php, or one that declares no component (core has $version, not
// $plugin->component), yields an empty string and no error: the caller falls
// back to another source of the name.
func Component(dir string) (string, error) {
	content, err := readVersionFile(dir)
	if err != nil {
		return "", err
	}

	match := pluginComponentPattern.FindStringSubmatch(content)
	if match == nil {
		return "", nil
	}

	return match[1], nil
}

// ReadVersion reads the version.php of a plugin — or of a Moodle code root —
// and returns what it declares. A directory without one yields an empty
// Version and no error: plenty of checkouts (a theme fork, a library) have no
// version.php at all.
func ReadVersion(dir string) (Version, error) {
	var v Version

	content, err := readVersionFile(dir)
	if err != nil {
		return v, err
	}

	if content == "" {
		return v, nil
	}

	if match := pluginVersionPattern.FindStringSubmatch(content); match != nil {
		v.Version = match[1]
	} else if match := coreVersionPattern.FindStringSubmatch(content); match != nil {
		v.Version = match[1]
	}

	if match := pluginReleasePattern.FindStringSubmatch(content); match != nil {
		v.Release = match[1]
	} else if match := coreReleasePattern.FindStringSubmatch(content); match != nil {
		// Core's release carries a build stamp — "5.2.1+ (Build: 20260716)" —
		// which says nothing a listing needs and would widen the column for
		// every other row.
		v.Release = strings.TrimSpace(strings.Split(match[1], " (Build:")[0])
	}

	return v, nil
}

// readVersionFile finds the version.php belonging to dir, trying the public/
// layout too so a Moodle 5.1+ code root works as well as a plugin directory.
func readVersionFile(dir string) (string, error) {
	candidates := []string{
		filepath.Join(dir, "version.php"),
		filepath.Join(dir, "public", "version.php"),
	}

	for _, path := range candidates {
		content, err := os.ReadFile(path)
		if err == nil {
			return string(content), nil
		}

		if !os.IsNotExist(err) {
			return "", err
		}
	}

	return "", nil
}
