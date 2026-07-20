package schema

import (
	"strings"
	"testing"
)

// validRecipe is the baseline every rejection case below mutates. Keeping the
// cases as one-key deviations from a document that *does* validate is what
// makes them evidence about a single keyword.
const validRecipe = `
name: baseline
moodle:
  mdlbranch: "502"
  source:
    git:
      remotes:
        origin: https://github.com/mutms/patches.git
      ref: origin/patch/mutms/MOODLE_502_STABLE
plugins:
  - mutms/tool_mulib
  - name: mutms/tool_mutenancy
    source: {git: {ref: v5.2.1.01}}
`

// sourceBlock is the core's whole source block, so a case can replace it as a
// unit rather than patching lines out of the middle of it.
const sourceBlock = `  source:
    git:
      remotes:
        origin: https://github.com/mutms/patches.git
      ref: origin/patch/mutms/MOODLE_502_STABLE
`

func validate(t *testing.T, kind Kind, raw string) error {
	t.Helper()

	doc, _, err := Decode([]byte(raw))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	return Validate(kind, doc)
}

func TestValidRecipePasses(t *testing.T) {
	if err := validate(t, KindRecipe, validRecipe); err != nil {
		t.Fatalf("baseline recipe should validate: %v", err)
	}
}

// TestKeywordsAreEnforced checks every JSON Schema keyword mudev's schemas
// actually rely on. The validating library is a dependency we may swap again
// (the previous one dragged in a Go 1.25 requirement), and a validator that
// silently ignored a keyword would let malformed catalogue data through — so
// each keyword is pinned by a case that must fail.
func TestKeywordsAreEnforced(t *testing.T) {
	cases := []struct {
		keyword string
		kind    Kind
		doc     string
	}{
		{
			// plugins[] is oneOf: a bare identifier string, or an object.
			keyword: "oneOf",
			kind:    KindRecipe,
			doc:     strings.Replace(validRecipe, "  - mutms/tool_mulib", "  - 42", 1),
		},
		{
			// Identifiers are lower-case vendor/package.
			keyword: "pattern",
			kind:    KindRecipe,
			doc:     strings.Replace(validRecipe, "mutms/tool_mulib", "MuTMS/Tool_Mulib", 1),
		},
		{
			// mdlbranch is a quoted string of digits, never a number.
			keyword: "type",
			kind:    KindRecipe,
			doc:     strings.Replace(validRecipe, `mdlbranch: "502"`, "mdlbranch: 502", 1),
		},
		{
			// moodle requires mdlbranch and source.
			keyword: "required",
			kind:    KindRecipe,
			doc:     strings.Replace(validRecipe, "  mdlbranch: \"502\"\n", "", 1),
		},
		{
			// A remote URL may not be the empty string.
			keyword: "minLength",
			kind:    KindRecipe,
			doc:     strings.Replace(validRecipe, "origin: https://github.com/mutms/patches.git", `origin: ""`, 1),
		},
		{
			// source must advertise at least one acquisition kind.
			keyword: "minProperties",
			kind:    KindRecipe,
			doc:     strings.Replace(validRecipe, sourceBlock, "  source: {}\n", 1),
		},
		{
			// moodle.patches entries take repo and ref only.
			keyword: "additionalProperties",
			kind:    KindRecipe,
			doc: strings.Replace(validRecipe, "plugins:",
				"  patches:\n    - {repo: https://example.org/p.git, ref: patch/x, oops: true}\nplugins:", 1),
		},
	}

	for _, c := range cases {
		t.Run(c.keyword, func(t *testing.T) {
			if err := validate(t, c.kind, c.doc); err == nil {
				t.Errorf("%s is not enforced — the document was accepted", c.keyword)
			}
		})
	}
}

// TestPluginSchemaKeywords covers the keywords that only the plugin schema
// exercises: a closed sub-object and a non-empty list.
func TestPluginSchemaKeywords(t *testing.T) {
	const valid = `
name: mutms/tool_mulib
title: MuTMS shared library
relpath: public/admin/tool/mulib
requirements:
  MOODLE_500_STABLE:
    mdlbranches: ["500", "501"]
`

	if err := validate(t, KindPlugin, valid); err != nil {
		t.Fatalf("baseline plugin should validate: %v", err)
	}

	// additionalProperties: false — a typo'd key inside a requirements entry
	// is exactly the authoring mistake schema validation exists to catch.
	typo := strings.Replace(valid, "    mdlbranches:", "    mdlbranch:", 1)

	if err := validate(t, KindPlugin, typo); err == nil {
		t.Error("additionalProperties is not enforced — a typo'd key was accepted")
	}

	// minItems: a branch that serves no Moodle branch is meaningless.
	empty := strings.Replace(valid, `["500", "501"]`, "[]", 1)

	if err := validate(t, KindPlugin, empty); err == nil {
		t.Error("minItems is not enforced — an empty mdlbranches list was accepted")
	}

	// $ref/$defs are only in the recipe schema; the plugin schema repeats the
	// source shape inline, so its own coverage stops here.
}

// TestRefAndDefs checks the one $ref in the recipe schema resolves — moodle's
// source points at $defs/source, so a bad source there proves both keywords.
func TestRefAndDefs(t *testing.T) {
	// remotes.origin is required by $defs/source; drop it.
	doc := strings.Replace(validRecipe, "        origin: https://github.com/mutms/patches.git", "        backup: https://example.org/x.git", 1)

	if err := validate(t, KindRecipe, doc); err == nil {
		t.Error("$ref/$defs did not resolve — a source without origin was accepted")
	}
}

func TestDecodeRejectsBrokenYAML(t *testing.T) {
	if _, _, err := Decode([]byte("name: [unterminated\n")); err == nil {
		t.Error("expected a parse error")
	}
}
