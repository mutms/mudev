package workspace

import (
	"strings"
	"testing"

	"github.com/mutms/mudev/internal/git"
)

func TestStateMarkers(t *testing.T) {
	cases := []struct {
		name string
		repo Repo
		want string
	}{
		{
			name: "clean managed checkout says nothing",
			repo: Repo{Managed: true, Status: git.Status{Branch: "MOODLE_500_STABLE"}},
			want: "",
		},
		{
			name: "unpushed and incoming commits",
			repo: Repo{Managed: true, Status: git.Status{Branch: "x", Ahead: 2, Behind: 13}},
			want: "2↑ 13↓",
		},
		{
			name: "uncommitted changes",
			repo: Repo{Managed: true, Status: git.Status{Branch: "x", Dirty: true}},
			want: "*",
		},
		{
			name: "a forgotten feature branch",
			repo: Repo{
				Managed:        true,
				RecordedBranch: "MOODLE_500_STABLE",
				Status:         git.Status{Branch: "MDL-1234-fix"},
			},
			want: "≠",
		},
		{
			name: "a checkout nobody recorded",
			repo: Repo{Status: git.Status{Branch: "main"}},
			want: "?",
		},
		{
			// Missing wins outright: there is no git state to report.
			name: "recorded but not on disk",
			repo: Repo{Managed: true, Missing: true, RecordedBranch: "MOODLE_500_STABLE"},
			want: "✗",
		},
		{
			name: "markers combine",
			repo: Repo{Status: git.Status{Branch: "main", Ahead: 1, Dirty: true}},
			want: "1↑ * ?",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := state(c.repo); got != c.want {
				t.Errorf("state = %q, want %q", got, c.want)
			}
		})
	}
}

func TestStrayedIgnoresDetachedAndPinnedCheckouts(t *testing.T) {
	// A pinned edition checks out a tag: detached is where it belongs.
	pinned := Repo{Managed: true, RecordedRef: "v5.2.1.01", Status: git.Status{Detached: true}}

	if pinned.Strayed() {
		t.Error("a detached pinned checkout is not strayed")
	}

	// No recorded branch means there is nothing to have strayed from.
	unmanaged := Repo{Status: git.Status{Branch: "whatever"}}

	if unmanaged.Strayed() {
		t.Error("an unmanaged checkout cannot stray")
	}

	onRecordedBranch := Repo{
		Managed:        true,
		RecordedBranch: "MOODLE_502_STABLE",
		Status:         git.Status{Branch: "MOODLE_502_STABLE"},
	}

	if onRecordedBranch.Strayed() {
		t.Error("a checkout on its recorded branch is not strayed")
	}
}

func TestBranchColumn(t *testing.T) {
	cases := []struct {
		name string
		repo Repo
		want string
	}{
		{
			// The common case: the upstream repeats the branch name, so it is
			// abbreviated rather than shown twice in every row.
			name: "tracking the same name upstream",
			repo: Repo{Status: git.Status{Branch: "MOODLE_502_STABLE", Tracking: "origin/MOODLE_502_STABLE"}},
			want: "MOODLE_502_STABLE...origin/*",
		},
		{
			name: "tracking something else",
			repo: Repo{Status: git.Status{Branch: "SMOKE", Tracking: "origin/MOODLE_500_STABLE"}},
			want: "SMOKE...origin/MOODLE_500_STABLE",
		},
		{
			name: "no upstream",
			repo: Repo{Status: git.Status{Branch: "local-only"}},
			want: "local-only",
		},
		{
			name: "pinned to a tag",
			repo: Repo{Status: git.Status{Detached: true}},
			want: "(detached)",
		},
		{
			name: "freshly git-inited",
			repo: Repo{Status: git.Status{Branch: "main", Unborn: true}},
			want: "(no commits)",
		},
		{
			name: "recorded but not on disk",
			repo: Repo{Managed: true, Missing: true},
			want: "not checked out",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := branch(c.repo); got != c.want {
				t.Errorf("branch = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRenderList(t *testing.T) {
	repos := []Repo{
		{
			Path:    CoreDir,
			Core:    true,
			Managed: true,
			Status:  git.Status{Branch: "MOODLE_502_STABLE", Tracking: "origin/MOODLE_502_STABLE"},
		},
		{
			Path:    "public/admin/tool/mulib",
			Name:    "mutms/tool_mulib",
			Managed: true,
			Status:  git.Status{Detached: true, Tags: []string{"v5.0.8.01"}},
		},
	}

	var out strings.Builder

	if err := RenderList(&out, repos, nil); err != nil {
		t.Fatalf("RenderList: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")

	if len(lines) != 2 {
		t.Fatalf("got %d lines:\n%s", len(lines), out.String())
	}

	// The path column is padded to the widest row, so columns line up.
	if !strings.HasPrefix(lines[0], ".                       ") {
		t.Errorf("first column not padded: %q", lines[0])
	}

	if !strings.HasSuffix(lines[1], "v5.0.8.01") {
		t.Errorf("tags not rendered: %q", lines[1])
	}

	// Rows carry no trailing padding — the core row has no tags to show.
	for _, line := range lines {
		if strings.HasSuffix(line, " ") {
			t.Errorf("trailing whitespace: %q", line)
		}
	}
}

func TestSelectColumns(t *testing.T) {
	// The default set is what the old tool showed, in that order.
	selected, err := selectColumns(nil)
	if err != nil {
		t.Fatalf("selectColumns: %v", err)
	}

	if len(selected) != len(DefaultColumns()) || selected[0].Name != "path" {
		t.Errorf("unexpected default columns: %+v", selected)
	}

	// Every registered column must be selectable — that is the whole point of
	// keeping them in a registry rather than in print statements.
	for _, name := range KnownColumns() {
		if _, err := selectColumns([]string{name}); err != nil {
			t.Errorf("column %q is registered but not selectable: %v", name, err)
		}
	}

	_, err = selectColumns([]string{"path", "bogus"})
	if err == nil {
		t.Fatal("expected an error for an unknown column")
	}

	// The message must list what is available, or it helps nobody.
	if !strings.Contains(err.Error(), "release") {
		t.Errorf("error should name the known columns: %v", err)
	}
}

func TestSortReposPutsCoreFirst(t *testing.T) {
	repos := []Repo{
		{Path: "public/admin/tool/mulib"},
		{Path: "public/admin/tool/certificate/element/muprog"},
		{Path: CoreDir},
		{Path: "public/admin/tool/certificate"},
	}

	sortRepos(repos)

	want := []string{
		CoreDir,
		"public/admin/tool/certificate",
		// A subplugin sorts directly under the plugin containing it.
		"public/admin/tool/certificate/element/muprog",
		"public/admin/tool/mulib",
	}

	for i, path := range want {
		if repos[i].Path != path {
			t.Errorf("row %d = %q, want %q", i, repos[i].Path, path)
		}
	}
}
