package workspace

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exportOf renders a workspace's recipe the way `mudev export` does.
func exportOf(t *testing.T, root string) string {
	t.Helper()

	var out strings.Builder

	if err := Export(ExportOptions{Root: root, Out: &out}); err != nil {
		t.Fatalf("export: %v", err)
	}

	return out.String()
}

func TestExportWritesARecipe(t *testing.T) {
	root, pluginsDir := addFixture(t)

	if err := addInto(t, root, pluginsDir, "mutms/tool_mulib", ""); err != nil {
		t.Fatalf("add: %v", err)
	}

	document := exportOf(t, root)

	// The schema line is what makes the result editable: an editor validates
	// and completes it exactly as it does the hand-authored catalogue files.
	if !strings.HasPrefix(document, "# yaml-language-server: $schema=") {
		t.Errorf("no schema header:\n%s", document)
	}

	for _, want := range []string{"mutms/tool_mulib", "relpath: public/admin/tool/mulib", "mdlbranch: \"502\""} {
		if !strings.Contains(document, want) {
			t.Errorf("the export is missing %q:\n%s", want, document)
		}
	}

	// mdlbranch is a string, and a recipe that says 502 unquoted is a recipe
	// that no longer means what it said.
	if strings.Contains(document, "mdlbranch: 502") {
		t.Errorf("mdlbranch lost its quotes:\n%s", document)
	}
}

// TestExportOrdersKeysForReading covers the part that only matters to people:
// an exported recipe gets committed and reviewed, so a plugin entry has to
// lead with its name rather than with whatever sorts first alphabetically.
func TestExportOrdersKeysForReading(t *testing.T) {
	root, pluginsDir := addFixture(t)

	if err := addInto(t, root, pluginsDir, "mutms/tool_mulib", ""); err != nil {
		t.Fatalf("add: %v", err)
	}

	document := exportOf(t, root)

	name := strings.Index(document, "- name: mutms/tool_mulib")
	if name < 0 {
		t.Fatalf("the plugin entry does not start with its name:\n%s", document)
	}

	if relpath := strings.Index(document, "    relpath:"); relpath < name {
		t.Errorf("relpath comes before the name:\n%s", document)
	}
}

// TestExportRoundTripsThroughClone is the promise the command makes: what it
// writes is a recipe, and a recipe assembles the same tree — with no plugin
// catalogue in reach, since every entry is flattened.
func TestExportRoundTripsThroughClone(t *testing.T) {
	root, pluginsDir := addFixture(t)

	if err := addInto(t, root, pluginsDir, "mutms/tool_mulib", ""); err != nil {
		t.Fatalf("add: %v", err)
	}

	exported := filepath.Join(t.TempDir(), "exported.yaml")

	if err := Export(ExportOptions{Root: root, File: exported, Out: io.Discard}); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Somewhere entirely fresh, and deliberately without pointing the config at
	// the catalogue the original was assembled from.
	second := filepath.Join(t.TempDir(), "workspace")

	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := cloneInto(t, exported, second); err != nil {
		t.Fatalf("cloning the exported recipe: %v", err)
	}

	if _, err := os.Stat(filepath.Join(second, "public", "admin", "tool", "mulib", ".git")); err != nil {
		t.Errorf("the plugin was not assembled from the exported recipe: %v", err)
	}

	// The two workspaces must describe the same tree. Their provenance differs
	// by definition — one was built from a catalogue identifier, the other from
	// the exported file — so that field is not part of the comparison.
	first := liveOf(t, root)
	rebuilt := liveOf(t, second)

	first.BasedOnRecipe, rebuilt.BasedOnRecipe = nil, nil

	if before, after := jsonOf(t, first), jsonOf(t, rebuilt); before != after {
		t.Errorf("the rebuilt workspace differs:\n%s\n\nwant:\n%s", after, before)
	}
}

func TestExportWritesAndReplacesAFile(t *testing.T) {
	root, _ := addFixture(t)

	path := filepath.Join(t.TempDir(), "recipe.yaml")

	var first strings.Builder

	if err := Export(ExportOptions{Root: root, File: path, Out: &first}); err != nil {
		t.Fatalf("export: %v", err)
	}

	if !strings.Contains(first.String(), "wrote") {
		t.Errorf("the write was not reported: %q", first.String())
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(string(written), "# yaml-language-server:") {
		t.Errorf("the file is not a recipe:\n%s", written)
	}

	// Replacing a file that may well have been hand-written is worth saying out
	// loud — comments do not survive the trip.
	var second strings.Builder

	if err := Export(ExportOptions{Root: root, File: path, Out: &second}); err != nil {
		t.Fatalf("re-export: %v", err)
	}

	if !strings.Contains(second.String(), "replaced") {
		t.Errorf("replacing an existing recipe was not reported: %q", second.String())
	}
}

func TestExportRejectsANameThatIsNotARecipe(t *testing.T) {
	root, _ := addFixture(t)

	err := Export(ExportOptions{
		Root: root,
		File: filepath.Join(t.TempDir(), "recipe.txt"),
		Out:  io.Discard,
	})
	if err == nil {
		t.Fatal("a non-YAML output name was accepted")
	}

	if !strings.Contains(err.Error(), ".yaml") {
		t.Errorf("the error does not say what is wanted: %v", err)
	}
}

func TestExportRefusesADirectoryThatIsNotAWorkspace(t *testing.T) {
	if err := Export(ExportOptions{Root: t.TempDir(), Out: io.Discard}); err == nil {
		t.Fatal("exporting a directory with no live recipe succeeded")
	}
}

// jsonOf renders a live recipe for comparison.
func jsonOf(t *testing.T, live *Live) string {
	t.Helper()

	data, err := json.MarshalIndent(live, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}
