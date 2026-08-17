package recipe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const devYAML = `
name: MuTMS dev on Moodle 5.2
catalog: ../../../mdl-plugins
extra:
  mudev: {release: mutms}
base:
  mdlbranch: "502"
  source:
    git:
      remotes:
        origin: https://github.com/mutms/patches.git
      ref: origin/patch/mutms/MOODLE_502_STABLE
  localbranch: MOODLE_502_STABLE
plugins:
  - mutms/tool_certificate
  - name: mutms/tool_mulib
    extra: {mudev: {release: mutms}}
  - name: mutms/tool_mutenancy
    source: {git: {ref: v5.2.1.01}}
`

func TestParse(t *testing.T) {
	r, err := Parse([]byte(devYAML))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if r.Base.Mdlbranch != "502" || r.Base.Localbranch != "MOODLE_502_STABLE" {
		t.Errorf("unexpected base block: %+v", r.Base)
	}

	if r.Release() != "mutms" {
		t.Errorf("Release = %q, want mutms", r.Release())
	}

	if len(r.Plugins) != 3 {
		t.Fatalf("got %d plugins", len(r.Plugins))
	}

	// A bare identifier and an object entry decode into the same type.
	if r.Plugins[0].Name != "mutms/tool_certificate" || r.Plugins[0].Ref() != "" {
		t.Errorf("bare entry: %+v", r.Plugins[0])
	}

	if r.Plugins[0].Raw["name"] != "mutms/tool_certificate" {
		t.Errorf("bare entry Raw: %v", r.Plugins[0].Raw)
	}

	if r.Plugins[1].Release() != "mutms" {
		t.Errorf("plugin release flag: %+v", r.Plugins[1].Extra)
	}

	if r.Plugins[2].Ref() != "v5.2.1.01" {
		t.Errorf("pinned ref: %+v", r.Plugins[2].Source)
	}
}

func TestReleaseFlavourIgnoresOtherNamespaces(t *testing.T) {
	// extra is a bag of tool namespaces; another tool's config is not ours.
	if got := ReleaseFlavour(map[string]any{"othertool": map[string]any{"release": "acme"}}); got != "" {
		t.Errorf("ReleaseFlavour = %q, want empty", got)
	}
}

func TestLocate(t *testing.T) {
	dir := t.TempDir()

	recipes := filepath.Join(dir, "mutms", "release")

	if err := os.MkdirAll(recipes, 0o755); err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(recipes, "5.2.1.01.yaml")

	if err := os.WriteFile(file, []byte(devYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// A catalogue identifier resolves under the recipes directory…
	path, id, err := Locate(dir, "mutms/release/5.2.1.01")
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}

	if path != file || id != "mutms/release/5.2.1.01" {
		t.Errorf("Locate = %q, %q", path, id)
	}

	// …while a path is loaded directly, with no identifier.
	path, id, err = Locate(dir, file)
	if err != nil {
		t.Fatalf("Locate: %v", err)
	}

	if path != file || id != "" {
		t.Errorf("Locate = %q, %q", path, id)
	}

	if _, _, err := Locate(dir, "mutms/release/nope"); err == nil {
		t.Error("expected an error for a missing recipe")
	}

	if _, _, err := Locate(dir, "not-an-identifier"); err == nil {
		t.Error("expected an error for a malformed identifier")
	}
}

func TestFetchOrder(t *testing.T) {
	r, err := Parse([]byte(`
extra:
  mudev: {release: mutms, fetch_order: [forge, origin]}
base:
  mdlbranch: "405"
  source: {git: {remotes: {origin: git@github.com:mutms/patches.git}}}
plugins: []
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := r.FetchOrder(); len(got) != 2 || got[0] != "forge" || got[1] != "origin" {
		t.Errorf("FetchOrder = %v", got)
	}

	// A recipe that says nothing about order is the normal case.
	plain, err := Parse([]byte("base:\n  mdlbranch: \"405\"\n  source: {git: {remotes: {origin: x}}}\nplugins: []\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := plain.FetchOrder(); got != nil {
		t.Errorf("FetchOrder = %v, want none", got)
	}
}

func TestFetchOrderMustBeAList(t *testing.T) {
	// In this flow mapping `fetch_order: forge, origin` parses as
	// fetch_order: "forge" plus a stray `origin: null` key — not as a list.
	// mudev used to ignore a non-list silently and fetch in its default order,
	// which looks exactly like the setting not working; the schema now rejects
	// the document (fetch_order must be an array, and `origin` is not a key of
	// the mudev namespace).
	_, err := Parse([]byte(`
extra:
  mudev: {release: mutms, fetch_order: forge, origin}
base:
  mdlbranch: "405"
  source: {git: {remotes: {origin: x}}}
plugins: []
`))
	if err == nil {
		t.Fatal("expected a comma-separated string to be rejected")
	}

	if !strings.Contains(err.Error(), "fetch_order") {
		t.Errorf("the error should name the key: %v", err)
	}
}

func TestExtraStaysOpenForOtherTools(t *testing.T) {
	// Typing mudev's own namespace must not close the door on anyone else's:
	// extra exists so a composer assembler or a catalogue site can carry its
	// own config without a schema change.
	_, err := Parse([]byte(`
extra:
  mudev: {release: mutms}
  othertool: {whatever: [1, 2, 3], nested: {deeply: true}}
base:
  mdlbranch: "405"
  source: {git: {remotes: {origin: x}}}
plugins:
  - name: mutms/tool_mulib
    extra: {othertool: {flag: yes}}
`))
	if err != nil {
		t.Errorf("other namespaces must stay opaque: %v", err)
	}
}

func TestMudevNamespaceIsClosed(t *testing.T) {
	// mudev's own namespace is the one part of extra this schema defines, so a
	// key it does not know is a typo — the setting would otherwise do nothing
	// at all, which is exactly how a mistyped fetch_order goes unnoticed.
	cases := map[string]string{
		"recipe level": `
extra:
  mudev: {release: mutms, fetchorder: [forge, origin]}
base:
  mdlbranch: "405"
  source: {git: {remotes: {origin: x}}}
plugins: []
`,
		"plugin level": `
base:
  mdlbranch: "405"
  source: {git: {remotes: {origin: x}}}
plugins:
  - name: mutms/tool_mulib
    extra: {mudev: {releases: mutms}}
`,
	}

	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(doc)); err == nil {
				t.Error("expected an unknown mudev key to be rejected")
			}
		})
	}
}
