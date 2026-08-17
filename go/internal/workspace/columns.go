package workspace

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode/utf8"
)

// The markers a listing uses. They are single glyphs so that a row stays
// scannable: the eye picks out the shape, not the wording.
const (
	MarkerAhead     = "↑" // commits not pushed yet
	MarkerBehind    = "↓" // commits waiting to be pulled
	MarkerDirty     = "*" // uncommitted changes
	MarkerStrayed   = "≠" // on a different branch from the recorded one
	MarkerUnmanaged = "?" // a checkout the live recipe does not record
	MarkerMissing   = "✗" // recorded by the live recipe but not on disk
)

// A Column is one field of a listing row.
//
// Columns are values in a registry rather than fixed print statements, so the
// set a listing shows can be chosen at call time — today by the --columns
// flag, later by a per-user default — without touching how rows are gathered.
// Adding a column means adding one entry here.
type Column struct {
	// Name is how the column is selected (the --columns spelling).
	Name string

	// Title is the human heading, for callers that render one.
	Title string

	// Value renders the column for one repository.
	Value func(Repo) string
}

// columns is the registry. Everything a listing can show lives here, whether
// or not it is shown by default.
var columns = map[string]Column{
	"path": {
		Name:  "path",
		Title: "Path",
		Value: func(r Repo) string {
			return r.Path
		},
	},
	"state": {
		Name:  "state",
		Title: "State",
		Value: state,
	},
	"branch": {
		Name:  "branch",
		Title: "Branch",
		Value: branch,
	},
	"version": {
		Name:  "version",
		Title: "Version",
		Value: func(r Repo) string {
			return r.Version.Version
		},
	},
	"release": {
		Name:  "release",
		Title: "Release",
		Value: func(r Repo) string {
			return r.Version.Release
		},
	},
	"tags": {
		Name:  "tags",
		Title: "Tags",
		Value: func(r Repo) string {
			return strings.Join(r.Status.Tags, ", ")
		},
	},
	"name": {
		Name:  "name",
		Title: "Plugin",
		Value: func(r Repo) string {
			return r.Name
		},
	},
	"recorded": {
		Name:  "recorded",
		Title: "Recorded",
		Value: func(r Repo) string {
			return r.RecordedRef
		},
	},
	"head": {
		Name:  "head",
		Title: "Head",
		Value: func(r Repo) string {
			return r.Status.Head
		},
	},
}

// defaultColumns is the daily-driver layout, carried over from the old tool:
// where it is, what state it is in, what branch, and which release.
var defaultColumns = []string{"path", "state", "branch", "version", "release", "tags"}

// DefaultColumns returns the columns a listing shows when none are chosen.
func DefaultColumns() []string {
	return append([]string(nil), defaultColumns...)
}

// KnownColumns returns every registered column name, sorted — for help text
// and for validating a selection.
func KnownColumns() []string {
	names := make([]string, 0, len(columns))

	for name := range columns {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

// state renders the marker cluster: unpushed and incoming commits, a dirty
// tree, a strayed branch, and whether the checkout is recorded at all.
func state(r Repo) string {
	if r.Missing {
		return MarkerMissing
	}

	var parts []string

	if r.Status.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("%d%s", r.Status.Ahead, MarkerAhead))
	}

	if r.Status.Behind > 0 {
		parts = append(parts, fmt.Sprintf("%d%s", r.Status.Behind, MarkerBehind))
	}

	if r.Status.Dirty {
		parts = append(parts, MarkerDirty)
	}

	if r.Strayed() {
		parts = append(parts, MarkerStrayed)
	}

	if !r.Managed {
		parts = append(parts, MarkerUnmanaged)
	}

	return strings.Join(parts, " ")
}

// branch renders the branch and what it tracks, in git's own spelling. An
// upstream of the same name is abbreviated to origin/* — the common case,
// which would otherwise repeat the branch name in every row.
func branch(r Repo) string {
	switch {
	case r.Missing:
		return "not checked out"

	case r.Status.Unborn:
		return "(no commits)"

	case r.Status.Detached:
		return "(detached)"

	case r.Status.Branch == "":
		return ""

	case r.Status.Tracking == "":
		return r.Status.Branch

	case r.Status.Tracking == "origin/"+r.Status.Branch:
		return r.Status.Branch + "...origin/*"
	}

	return r.Status.Branch + "..." + r.Status.Tracking
}

// RenderList writes the repositories as an aligned table.
//
// Column widths come from the content, so the table is as narrow as the data
// allows, and a trailing empty column adds nothing to a row.
func RenderList(w io.Writer, repos []Repo, names []string) error {
	selected, err := selectColumns(names)
	if err != nil {
		return err
	}

	rows := make([][]string, 0, len(repos))

	for _, repo := range repos {
		row := make([]string, len(selected))

		for i, column := range selected {
			row[i] = column.Value(repo)
		}

		rows = append(rows, row)
	}

	widths := make([]int, len(selected))

	for _, row := range rows {
		for i, cell := range row {
			if n := utf8.RuneCountInString(cell); n > widths[i] {
				widths[i] = n
			}
		}
	}

	for _, row := range rows {
		var line strings.Builder

		for i, cell := range row {
			if i > 0 {
				line.WriteString("  ")
			}

			line.WriteString(cell)

			// Everything after the last non-empty cell is padding nobody sees.
			if i < len(row)-1 {
				line.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)))
			}
		}

		if _, err := fmt.Fprintln(w, strings.TrimRight(line.String(), " ")); err != nil {
			return err
		}
	}

	return nil
}

// selectColumns resolves column names to their definitions.
func selectColumns(names []string) ([]Column, error) {
	if len(names) == 0 {
		names = DefaultColumns()
	}

	selected := make([]Column, 0, len(names))

	for _, name := range names {
		column, ok := columns[strings.TrimSpace(name)]
		if !ok {
			return nil, fmt.Errorf("unknown column %q — known columns: %s",
				name, strings.Join(KnownColumns(), ", "))
		}

		selected = append(selected, column)
	}

	return selected, nil
}
