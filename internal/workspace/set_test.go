package workspace

import (
	"io"
	"strings"
	"testing"
)

// setInto changes one field the way the command does, returning what it said.
func setInto(t *testing.T, root string, key string, value string) (string, error) {
	t.Helper()

	var out strings.Builder

	err := Set(SetOptions{Root: root, Key: key, Value: value, Out: &out})

	return out.String(), err
}

func TestSetRenamesTheWorkspace(t *testing.T) {
	root, _ := addFixture(t)

	out, err := setInto(t, root, "name", "MuTMS dev 5.2")
	if err != nil {
		t.Fatalf("set: %v", err)
	}

	if got := liveOf(t, root).Name; got != "MuTMS dev 5.2" {
		t.Errorf("name is %q, want %q", got, "MuTMS dev 5.2")
	}

	// The old value is worth saying: it is how you notice you renamed the
	// wrong workspace.
	if !strings.Contains(out, "plain moodle") {
		t.Errorf("the previous name was not reported: %q", out)
	}

	// And it has to survive into an export, which is the whole point.
	if document := exportOf(t, root); !strings.Contains(document, "name: MuTMS dev 5.2") {
		t.Errorf("the new name did not reach the export:\n%s", document)
	}
}

func TestSetClearsWithAnEmptyValue(t *testing.T) {
	root, _ := addFixture(t)

	if _, err := setInto(t, root, "description", "for a while"); err != nil {
		t.Fatalf("set: %v", err)
	}

	out, err := setInto(t, root, "description", "")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}

	if got := liveOf(t, root).Description; got != "" {
		t.Errorf("description is %q, want it cleared", got)
	}

	if !strings.Contains(out, "cleared") {
		t.Errorf("clearing was not reported: %q", out)
	}

	// An omitted field must not come back as an empty line in the export.
	if document := exportOf(t, root); strings.Contains(document, "description:") {
		t.Errorf("a cleared field still appears in the export:\n%s", document)
	}
}

func TestSetRejectsAnUnknownKey(t *testing.T) {
	root, _ := addFixture(t)

	_, err := setInto(t, root, "moodle", "5.2")
	if err == nil {
		t.Fatal("an unknown key was accepted")
	}

	// The error has to teach: an unknown key is nearly always a guess.
	for _, key := range settableKeys {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("the error does not mention %q: %v", key, err)
		}
	}
}

// TestSetRefusesToRewriteProvenance guards the attribution: based_on_recipe
// credits the recipe this workspace was adapted from, so it is not a label to
// be edited.
func TestSetRefusesToRewriteProvenance(t *testing.T) {
	root, _ := addFixture(t)

	before := liveOf(t, root).BasedOnRecipe

	if _, err := setInto(t, root, "based_on_recipe", "someone/else/1.0"); err == nil {
		t.Fatal("provenance was rewritten")
	}

	if after := liveOf(t, root).BasedOnRecipe; after != before {
		t.Errorf("provenance changed anyway: %v → %v", before, after)
	}
}

func TestSetRefusesADirectoryThatIsNotAWorkspace(t *testing.T) {
	err := Set(SetOptions{Root: t.TempDir(), Key: "name", Value: "x", Out: io.Discard})
	if err == nil {
		t.Fatal("setting a field with no live recipe succeeded")
	}
}
