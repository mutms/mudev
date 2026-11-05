package moodle

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
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

// versionDateLayout is the date half of a MuTMS version stamp: 2026110553 is
// 2026-11-05 on the 503 branch.
const versionDateLayout = "20060102"

// VersionSuffix is the two digits a MuTMS version stamp ends with on a Moodle
// branch: the first and last digit of the branch code, so 503 → "53" and
// 600 → "60". It keeps stamps of the same day on different branches apart.
func VersionSuffix(mdlbranch string) (string, error) {
	if len(mdlbranch) != 3 || strings.Trim(mdlbranch, "0123456789") != "" {
		return "", fmt.Errorf("mdlbranch %q is not a three-digit Moodle branch code", mdlbranch)
	}

	return mdlbranch[:1] + mdlbranch[2:], nil
}

// DateVersion is the MuTMS version stamp for a day on a Moodle branch.
func DateVersion(day time.Time, mdlbranch string) (string, error) {
	suffix, err := VersionSuffix(mdlbranch)
	if err != nil {
		return "", err
	}

	return day.Format(versionDateLayout) + suffix, nil
}

// CheckPluginVersion verifies a MuTMS version stamp for a Moodle branch: a
// real date as YYYYMMDD followed by the branch's suffix, nothing else.
//
// The suffix is the part that matters most. A 503 stamp set on a 504 branch
// sorts as a perfectly plausible version, and nothing would notice until two
// branches published the same number for different code.
func CheckPluginVersion(version, mdlbranch string) error {
	suffix, err := VersionSuffix(mdlbranch)
	if err != nil {
		return err
	}

	pattern := regexp.MustCompile(`^(\d{8})` + suffix + `$`)

	match := pattern.FindStringSubmatch(version)
	if match == nil {
		return fmt.Errorf("version %q must be YYYYMMDD%s for mdlbranch %s", version, suffix, mdlbranch)
	}

	if _, err := time.Parse(versionDateLayout, match[1]); err != nil {
		return fmt.Errorf("version %q does not start with a real date (YYYYMMDD)", version)
	}

	return nil
}

// VersionNumber is the integer part of a version stamp, for comparing two of
// them: "2026110553.01" and "2026110553" are the same day's version.
func VersionNumber(version string) (int64, error) {
	whole, _, _ := strings.Cut(strings.Trim(version, "'\""), ".")

	number, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a version number", version)
	}

	return number, nil
}

// pluginVersionAssignment matches the whole $plugin->version assignment value,
// quoted or not, keeping what precedes the number so the file's own alignment
// survives a rewrite.
var pluginVersionAssignment = regexp.MustCompile(`(\$plugin->version\s*=\s*)'?\d+(?:\.\d+)?'?`)

// SetPluginVersion rewrites the $plugin->version of a version.php.
//
// Exactly one assignment must be there: none means this is not a plugin's
// version.php, and two means mudev cannot know which one Moodle will see.
func SetPluginVersion(content []byte, version string) ([]byte, error) {
	matches := pluginVersionAssignment.FindAllIndex(content, -1)

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no $plugin->version assignment found")

	case 1:
		// The one expected case.

	default:
		return nil, fmt.Errorf("%d $plugin->version assignments found, expected one", len(matches))
	}

	return pluginVersionAssignment.ReplaceAll(content, []byte("${1}"+version)), nil
}

// DependencyChange is one $plugin->dependencies entry SetDependencyVersions
// rewrote.
type DependencyChange struct {
	Component string
	From      string
	To        string
}

// dependenciesStart finds where a $plugin->dependencies array opens, in either
// spelling PHP allows.
var dependenciesStart = regexp.MustCompile(`\$plugin->dependencies\s*=\s*(\[|array\s*\()`)

// SetDependencyVersions points the $plugin->dependencies entries for the given
// components at version, and reports each entry it changed.
//
// MuTMS plugins are released together, so a plugin that depends on another
// one of them depends on the version released alongside it. Only numeric
// entries inside the dependencies array are rewritten: ANY_VERSION stays, and
// a component named anywhere else in the file is not a dependency.
func SetDependencyVersions(content []byte, components []string, version string) ([]byte, []DependencyChange, error) {
	loc := dependenciesStart.FindSubmatchIndex(content)
	if loc == nil {
		return content, nil, nil
	}

	// The group is "[" or "array(": its last byte is the bracket.
	open := content[loc[3]-1]

	closer := byte(']')
	if open != '[' {
		closer = ')'
	}

	start := loc[1]

	end := matchingClose(content, start, open, closer)
	if end < 0 {
		return nil, nil, fmt.Errorf("$plugin->dependencies array is not closed")
	}

	block := content[start:end]

	var changes []DependencyChange

	for _, component := range components {
		entry := regexp.MustCompile(`(['"]` + regexp.QuoteMeta(component) + `['"]\s*=>\s*)'?(\d+(?:\.\d+)?)'?`)

		for _, match := range entry.FindAllSubmatch(block, -1) {
			if string(match[2]) != version {
				changes = append(changes, DependencyChange{Component: component, From: string(match[2]), To: version})
			}
		}

		block = entry.ReplaceAll(block, []byte("${1}"+version))
	}

	result := make([]byte, 0, len(content)-(end-start)+len(block))
	result = append(result, content[:start]...)
	result = append(result, block...)
	result = append(result, content[end:]...)

	return result, changes, nil
}

// matchingClose finds the bracket closing the one opened just before start,
// or -1. Component names and version numbers contain no brackets, so counting
// them is enough for a version.php.
func matchingClose(content []byte, start int, open, closer byte) int {
	depth := 1

	for i := start; i < len(content); i++ {
		switch content[i] {
		case open:
			depth++

		case closer:
			depth--

			if depth == 0 {
				return i
			}
		}
	}

	return -1
}
