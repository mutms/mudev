package workspace

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDeepMerge(t *testing.T) {
	catalogue := map[string]any{
		"name":    "mutms/tool_mutenancy",
		"relpath": "public/admin/tool/mutenancy",
		"source": map[string]any{
			"git": map[string]any{
				"remotes": map[string]any{"origin": "https://example.org/mutenancy.git"},
			},
			"composer": "mutms/moodle-tool_mutenancy",
		},
	}

	// A recipe entry that pins only the ref must keep the catalogue's remotes.
	entry := map[string]any{
		"name":   "mutms/tool_mutenancy",
		"source": map[string]any{"git": map[string]any{"ref": "v5.2.1.01"}},
	}

	merged := deepMerge(catalogue, entry)

	git := merged["source"].(map[string]any)["git"].(map[string]any)

	if git["ref"] != "v5.2.1.01" {
		t.Errorf("ref not applied: %v", git)
	}

	if git["remotes"].(map[string]any)["origin"] != "https://example.org/mutenancy.git" {
		t.Errorf("catalogue remotes lost: %v", git)
	}

	// The catalogue entry is cached and shared, so it must not be touched.
	if _, polluted := catalogue["source"].(map[string]any)["git"].(map[string]any)["ref"]; polluted {
		t.Error("deepMerge mutated the catalogue document")
	}
}

func TestNarrowSourceToGit(t *testing.T) {
	definition := map[string]any{
		"source": map[string]any{
			"git":      map[string]any{"remotes": map[string]any{"origin": "old"}},
			"composer": "mutms/moodle-tool_mulib",
		},
	}

	narrowSourceToGit(definition, map[string]string{"origin": "new"}, "origin/MOODLE_500_STABLE")

	source := definition["source"].(map[string]any)

	// The live recipe records only the kind that was actually used.
	if _, ok := source["composer"]; ok {
		t.Errorf("composer kind should be dropped: %v", source)
	}

	git := source["git"].(map[string]any)

	if git["ref"] != "origin/MOODLE_500_STABLE" {
		t.Errorf("ref not recorded: %v", git)
	}

	if git["remotes"].(map[string]any)["origin"] != "new" {
		t.Errorf("remotes not recorded: %v", git)
	}
}

func TestSortedNamesPutsOriginFirst(t *testing.T) {
	got := sortedNames(map[string]string{"upstream": "u", "backup": "b", "origin": "o"})

	if want := []string{"origin", "backup", "upstream"}; !reflect.DeepEqual(got, want) {
		t.Errorf("sortedNames = %v, want %v", got, want)
	}
}

func TestLiveRoundTrip(t *testing.T) {
	root := t.TempDir()

	live := &Live{
		Name:          "smoke",
		BasedOnRecipe: "mutms/full/5.2.1.01",
		Moodle: map[string]any{
			"mdlbranch": "502",
			"source": map[string]any{
				"git": map[string]any{
					"remotes": map[string]any{"origin": "https://example.org/moodle.git"},
					"ref":     "v5.2.1",
				},
			},
		},
	}

	live.SetPlugin(map[string]any{
		"name":    "mutms/tool_mulib",
		"title":   "MuTMS shared library",
		"relpath": "public/admin/tool/mulib",
		"requirements": map[string]any{
			"MOODLE_500_STABLE": map[string]any{"mdlbranches": []any{"502"}},
		},
		"source": map[string]any{
			"git": map[string]any{
				"remotes": map[string]any{"origin": "https://example.org/mulib.git"},
				"ref":     "origin/MOODLE_500_STABLE",
			},
		},
	})

	if err := live.Save(root); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The live recipe validates against the recipe schema, like its source.
	reloaded, err := LoadLive(root)
	if err != nil {
		t.Fatalf("LoadLive: %v", err)
	}

	if reloaded.BasedOnRecipe != "mutms/full/5.2.1.01" {
		t.Errorf("provenance lost: %+v", reloaded)
	}

	entry, ok := reloaded.Plugin("mutms/tool_mulib")
	if !ok {
		t.Fatalf("plugin entry lost: %+v", reloaded.Plugins)
	}

	if entry["title"] != "MuTMS shared library" {
		t.Errorf("flattened fields lost: %v", entry)
	}

	// Re-recording a plugin replaces it rather than appending a duplicate.
	reloaded.SetPlugin(map[string]any{"name": "mutms/tool_mulib", "title": "changed"})

	if len(reloaded.Plugins) != 1 {
		t.Errorf("SetPlugin appended a duplicate: %v", reloaded.Plugins)
	}
}

func TestLoadLiveMissingIsNotAnError(t *testing.T) {
	live, err := LoadLive(t.TempDir())
	if err != nil {
		t.Fatalf("LoadLive: %v", err)
	}

	if live != nil {
		t.Errorf("expected nil for a workspace that has not been started, got %+v", live)
	}
}

func TestDeepCopyIsIndependent(t *testing.T) {
	original := map[string]any{"nested": map[string]any{"key": "value"}}

	copied := deepCopy(original)
	copied["nested"].(map[string]any)["key"] = "changed"

	if original["nested"].(map[string]any)["key"] != "value" {
		t.Error("deepCopy shares nested state")
	}

	// A copied document must still marshal identically.
	if _, err := json.Marshal(copied); err != nil {
		t.Fatalf("marshal: %v", err)
	}
}
