package workspace

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// campPluginSpec describes one plugin to stage into a test workspace.
type campPluginSpec struct {
	name      string
	relpath   string
	remotes   map[string]string
	pluginPHP bool
	langName  string
	readme    string
}

// campWorkspace stages a workspace with a valid live recipe and the plugin
// checkouts it records, and returns its root.
func campWorkspace(t *testing.T, plugins ...campPluginSpec) string {
	t.Helper()

	root := t.TempDir()

	live := map[string]any{
		"base": map[string]any{
			"mdlbranch":   "405",
			"strippublic": true,
			"source": map[string]any{
				"git": map[string]any{
					"ref":     "origin/MOODLE_405_STABLE",
					"remotes": map[string]any{"origin": "git@github.com:moodle/moodle.git"},
				},
			},
		},
		"plugins": []any{},
	}

	entries := []any{}

	for _, spec := range plugins {
		remotes := spec.remotes
		if remotes == nil {
			remotes = map[string]string{"origin": "git@github.com:mutms/moodle-" + campComponentFromName(spec.name) + ".git"}
		}

		asAny := map[string]any{}
		for key, value := range remotes {
			asAny[key] = value
		}

		entries = append(entries, map[string]any{
			"name":    spec.name,
			"relpath": spec.relpath,
			"source": map[string]any{
				"git": map[string]any{"ref": "origin/MOODLE_405_STABLE", "remotes": asAny},
			},
		})

		component := campComponentFromName(spec.name)

		// Relpaths are recorded in the newest layout; this recipe strips
		// public/, so that is where the checkout goes on disk.
		dir := filepath.Join(root, strings.TrimPrefix(spec.relpath, "public/"))

		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}

		if spec.pluginPHP {
			body := "<?php\n$plugin->component = '" + component + "';\n"

			if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		if spec.langName != "" {
			langDir := filepath.Join(dir, "lang", "en")

			if err := os.MkdirAll(langDir, 0o755); err != nil {
				t.Fatal(err)
			}

			body := "<?php\n$string['pluginname'] = '" + spec.langName + "';\n"

			if err := os.WriteFile(filepath.Join(langDir, component+".php"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		if spec.readme != "" {
			if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(spec.readme), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	live["plugins"] = entries

	data, err := json.Marshal(live)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(LivePath(root), data, 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}

// campRun runs CampInit and returns what it printed.
func campRun(t *testing.T, opts CampInitOptions) string {
	t.Helper()

	var out bytes.Buffer

	opts.Out = &out

	if err := CampInit(opts); err != nil {
		t.Fatalf("CampInit: %v", err)
	}

	return out.String()
}

// campRead parses a generated listing.
func campRead(t *testing.T, root, relpath string) map[string]any {
	t.Helper()

	path := filepath.Join(root, relpath, campDir, campListing)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var doc map[string]any

	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	return doc
}

// muprog is the standard fixture: a MuTMS plugin with everything present.
func muprog() campPluginSpec {
	return campPluginSpec{
		name:      "mutms/tool_muprog",
		relpath:   "public/admin/tool/muprog",
		pluginPHP: true,
		langName:  "Programs",
		readme: "# Programs plugin for Moodle™ LMS\n\n" +
			"[![MDL Shield](https://img.shields.io/endpoint?url=x)](https://mdlshield.com/plugins/tool_muprog) " +
			"![CI](https://github.com/mutms/moodle-tool_muprog/actions/workflows/ci.yml/badge.svg)\n\n" +
			"Structured learning programs for standard Moodle™ LMS installations.\n" +
			"Part of the [MuTMS suite](https://github.com/mutms).\n\n" +
			"## Installation\n\nSomething else entirely.\n",
	}
}

func TestCampInitWritesListing(t *testing.T) {
	root := campWorkspace(t, muprog())

	campRun(t, CampInitOptions{Root: root})

	doc := campRead(t, root, "admin/tool/muprog")

	if doc["name"] != "Programs" {
		t.Errorf("name = %v, want %q", doc["name"], "Programs")
	}

	want := "Structured learning programs for standard Moodle™ LMS installations. " +
		"Part of the MuTMS suite."

	if doc["summary"] != want {
		t.Errorf("summary = %v, want %q", doc["summary"], want)
	}

	labels, _ := doc["labels"].([]any)
	if len(labels) != 1 || labels[0] != "fully-free" {
		t.Errorf("labels = %v, want [fully-free]", doc["labels"])
	}

	links, _ := doc["links"].(map[string]any)
	if links["docs"] != "https://github.com/mutms/moodle-tool_muprog#readme" {
		t.Errorf("links.docs = %v", links["docs"])
	}

	if links["issues"] != "https://github.com/mutms/moodle-tool_muprog/issues" {
		t.Errorf("links.issues = %v", links["issues"])
	}

	badges, _ := doc["badges"].([]any)
	if len(badges) != 1 {
		t.Fatalf("badges = %v, want one entry", doc["badges"])
	}

	badge, _ := badges[0].(map[string]any)
	if badge["endpoint"] != "https://mdlshield.com/api/badge/tool_muprog" {
		t.Errorf("badge endpoint = %v", badge["endpoint"])
	}

	// The generated file carries only what mudev can know; prose and the
	// unsettled category vocabulary are left to the author.
	for _, absent := range []string{"description", "category", "screenshots"} {
		if _, ok := doc[absent]; ok {
			t.Errorf("%s should not be generated", absent)
		}
	}
}

func TestCampInitWritesSchemaModeline(t *testing.T) {
	root := campWorkspace(t, muprog())

	campRun(t, CampInitOptions{Root: root})

	data, err := os.ReadFile(filepath.Join(root, "admin/tool/muprog", campDir, campListing))
	if err != nil {
		t.Fatal(err)
	}

	first, _, _ := strings.Cut(string(data), "\n")

	if want := "# yaml-language-server: $schema=" + campSchemaURL; first != want {
		t.Errorf("first line = %q, want %q", first, want)
	}
}

// The name comes from the language file rather than the component, which is
// the whole reason for generating this in mudev instead of running camp's own
// scaffolder.
func TestCampInitFallsBackToComponentName(t *testing.T) {
	spec := muprog()
	spec.langName = ""

	root := campWorkspace(t, spec)

	printed := campRun(t, CampInitOptions{Root: root})

	doc := campRead(t, root, "admin/tool/muprog")

	if doc["name"] != "tool_muprog" {
		t.Errorf("name = %v, want the component", doc["name"])
	}

	if !strings.Contains(printed, "no pluginname string") {
		t.Errorf("expected a warning about the missing pluginname, got:\n%s", printed)
	}
}

func TestCampInitWithoutReadme(t *testing.T) {
	spec := muprog()
	spec.readme = ""

	root := campWorkspace(t, spec)

	printed := campRun(t, CampInitOptions{Root: root})

	doc := campRead(t, root, "admin/tool/muprog")

	if doc["summary"] != "One line about what this plugin does." {
		t.Errorf("summary = %v, want the placeholder", doc["summary"])
	}

	if !strings.Contains(printed, "no README lead paragraph") {
		t.Errorf("expected a warning about the missing summary, got:\n%s", printed)
	}
}

func TestCampInitSkipsForks(t *testing.T) {
	root := campWorkspace(t, campPluginSpec{
		name:      "mutms/tool_certificate",
		relpath:   "public/admin/tool/certificate",
		pluginPHP: true,
		langName:  "Certificates",
		remotes: map[string]string{
			"origin":   "git@github.com:mutms/moodle-tool_certificate.git",
			"upstream": "https://github.com/moodleworkplace/moodle-tool_certificate.git",
		},
	})

	printed := campRun(t, CampInitOptions{Root: root})

	if !strings.Contains(printed, "fork of moodleworkplace/moodle-tool_certificate") {
		t.Errorf("expected a fork warning naming the upstream, got:\n%s", printed)
	}

	path := filepath.Join(root, "admin/tool/certificate", campDir, campListing)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("a fork must not get a listing (stat err = %v)", err)
	}
}

func TestCampInitDoesNotClobber(t *testing.T) {
	root := campWorkspace(t, muprog())

	campRun(t, CampInitOptions{Root: root})

	path := filepath.Join(root, "admin/tool/muprog", campDir, campListing)
	mine := []byte("name: Edited by hand\n")

	if err := os.WriteFile(path, mine, 0o644); err != nil {
		t.Fatal(err)
	}

	printed := campRun(t, CampInitOptions{Root: root})

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(after, mine) {
		t.Errorf("the hand-edited listing was overwritten:\n%s", after)
	}

	if !strings.Contains(printed, "exists — left alone") {
		t.Errorf("expected an 'exists' report, got:\n%s", printed)
	}

	campRun(t, CampInitOptions{Root: root, Force: true})

	forced, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(forced, mine) {
		t.Error("--force should have regenerated the listing")
	}
}

func TestCampInitAppendsGitattributesOnce(t *testing.T) {
	root := campWorkspace(t, muprog())
	path := filepath.Join(root, "admin/tool/muprog", ".gitattributes")

	if err := os.WriteFile(path, []byte("*.php text"), 0o644); err != nil {
		t.Fatal(err)
	}

	campRun(t, CampInitOptions{Root: root})

	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// The existing rule survives, and the missing newline before the appended
	// block is supplied.
	if !strings.HasPrefix(string(first), "*.php text\n") {
		t.Errorf("existing rules lost:\n%s", first)
	}

	if got := strings.Count(string(first), "export-ignore"); got != len(campExportIgnore)-1 {
		t.Errorf("export-ignore lines = %d, want %d", got, len(campExportIgnore)-1)
	}

	campRun(t, CampInitOptions{Root: root, Force: true})

	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(first, second) {
		t.Errorf("rules were appended twice:\n%s", second)
	}
}

func TestCampInitSelectsOnePlugin(t *testing.T) {
	other := campPluginSpec{
		name:      "mutms/mod_mubook",
		relpath:   "public/mod/mubook",
		pluginPHP: true,
		langName:  "Interactive book",
	}

	root := campWorkspace(t, muprog(), other)

	campRun(t, CampInitOptions{Root: root, Relpath: "admin/tool/muprog"})

	if _, err := os.Stat(filepath.Join(root, "admin/tool/muprog", campDir, campListing)); err != nil {
		t.Errorf("the selected plugin was not written: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "mod/mubook", campDir, campListing)); !os.IsNotExist(err) {
		t.Errorf("an unselected plugin was written (stat err = %v)", err)
	}
}

func TestCampInitRejectsCore(t *testing.T) {
	root := campWorkspace(t, muprog())

	err := CampInit(CampInitOptions{Root: root, Relpath: CoreDir, Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("core should have no listing")
	}

	if !strings.Contains(err.Error(), "Moodle core") {
		t.Errorf("error = %v, want it to name core", err)
	}
}

func TestCampInitRejectsUnknownRelpath(t *testing.T) {
	root := campWorkspace(t, muprog())

	err := CampInit(CampInitOptions{Root: root, Relpath: "mod/nope", Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("an unrecorded path should be an error")
	}
}

func TestCampInitRejectsUnknownLabel(t *testing.T) {
	root := campWorkspace(t, muprog())

	err := CampInit(CampInitOptions{
		Root:   root,
		Labels: []string{"fully-free", "free-beer"},
		Out:    &bytes.Buffer{},
	})

	if err == nil {
		t.Fatal("an unknown disclosure label should be an error")
	}

	if !strings.Contains(err.Error(), "free-beer") {
		t.Errorf("error = %v, want it to name the bad label", err)
	}
}

func TestCampInitLabelsAndNoBadges(t *testing.T) {
	root := campWorkspace(t, muprog())

	campRun(t, CampInitOptions{
		Root:     root,
		Labels:   []string{"fully-free", "commercial-support-available"},
		NoBadges: true,
	})

	doc := campRead(t, root, "admin/tool/muprog")

	labels, _ := doc["labels"].([]any)
	if len(labels) != 2 || labels[1] != "commercial-support-available" {
		t.Errorf("labels = %v", doc["labels"])
	}

	if _, ok := doc["badges"]; ok {
		t.Errorf("--no-badges should leave badges out: %v", doc["badges"])
	}
}

func TestCampInitWithoutWorkspace(t *testing.T) {
	err := CampInit(CampInitOptions{Root: t.TempDir(), Out: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("a directory with no live recipe should be an error")
	}

	if !strings.Contains(err.Error(), "mudev clone") {
		t.Errorf("error = %v, want it to point at `mudev clone`", err)
	}
}

func TestCampInitSkipsMissingCheckout(t *testing.T) {
	root := campWorkspace(t, muprog())

	if err := os.RemoveAll(filepath.Join(root, "admin/tool/muprog")); err != nil {
		t.Fatal(err)
	}

	printed := campRun(t, CampInitOptions{Root: root})

	if !strings.Contains(printed, "not checked out") {
		t.Errorf("expected a not-checked-out warning, got:\n%s", printed)
	}
}

func TestCampSummarySkipsChrome(t *testing.T) {
	cases := []struct {
		name   string
		readme string
		want   string
	}{
		{
			name:   "badge row",
			readme: "# Title\n\n![a](b) ![c](d)\n\nThe real summary.\n",
			want:   "The real summary.",
		},
		{
			name:   "linked badge row",
			readme: "# Title\n\n[![a](b)](c)\n\nThe real summary.\n",
			want:   "The real summary.",
		},
		{
			name:   "blockquote and list",
			readme: "# Title\n\n> A note.\n\n- one\n- two\n\nThe real summary.\n",
			want:   "The real summary.",
		},
		{
			name:   "html banner",
			readme: "<p align=\"center\">banner</p>\n\nThe real summary.\n",
			want:   "The real summary.",
		},
		{
			name:   "wrapped paragraph joins",
			readme: "# Title\n\nOne line\nand its wrap.\n\nLater paragraph.\n",
			want:   "One line and its wrap.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := campScanSummary(strings.NewReader(tc.readme))

			if got != tc.want {
				t.Errorf("campScanSummary = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCampTruncateCountsCharactersNotBytes(t *testing.T) {
	// 120 multibyte characters: well inside the limit, but 360 bytes.
	value := strings.Repeat("™", 120)

	if got := campTruncate(value, campSummaryMax); got != value {
		t.Errorf("a %d-character value was truncated to %d", len([]rune(value)), len([]rune(got)))
	}

	long := strings.Repeat("word ", 80)

	got := campTruncate(long, campSummaryMax)

	if len([]rune(got)) > campSummaryMax {
		t.Errorf("truncated to %d characters, want <= %d", len([]rune(got)), campSummaryMax)
	}

	if !strings.HasSuffix(got, "…") {
		t.Errorf("a truncated value should be marked: %q", got)
	}
}

func TestCampGitHubSlug(t *testing.T) {
	cases := map[string]string{
		"git@github.com:mutms/moodle-tool_muprog.git":      "mutms/moodle-tool_muprog",
		"git@github.com:mutms/moodle-tool_muprog":          "mutms/moodle-tool_muprog",
		"https://github.com/mutms/moodle-mod_mubook.git":   "mutms/moodle-mod_mubook",
		"https://github.com/mutms/moodle-mod_mubook":       "mutms/moodle-mod_mubook",
		"https://github.com/mutms/moodle-mod_mubook/":      "mutms/moodle-mod_mubook",
		"ssh://git@github.com/mutms/moodle-mod_mubook.git": "mutms/moodle-mod_mubook",
		"git@gitlab.com:mutms/moodle-tool_muprog.git":      "",
		"git@forge.mpd.test:mutms/patches.git":             "",
		"":                                                 "",
	}

	for remote, want := range cases {
		if got := campGitHubSlug(remote); got != want {
			t.Errorf("campGitHubSlug(%q) = %q, want %q", remote, got, want)
		}
	}
}

// A plugin whose origin is not GitHub still gets a listing — just without the
// links mudev cannot spell.
func TestCampInitWithoutGitHubOrigin(t *testing.T) {
	spec := muprog()
	spec.remotes = map[string]string{"origin": "git@gitlab.com:mutms/moodle-tool_muprog.git"}

	root := campWorkspace(t, spec)

	printed := campRun(t, CampInitOptions{Root: root})

	doc := campRead(t, root, "admin/tool/muprog")

	if _, ok := doc["links"]; ok {
		t.Errorf("links should be omitted: %v", doc["links"])
	}

	if doc["name"] != "Programs" {
		t.Errorf("the rest of the listing should still be written: %v", doc)
	}

	if !strings.Contains(printed, "no GitHub origin remote") {
		t.Errorf("expected a warning about the links, got:\n%s", printed)
	}
}
