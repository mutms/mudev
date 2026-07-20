// Package schema validates hand-authored YAML (and the JSON live recipe)
// against mudev's embedded JSON Schemas.
//
// The plugin and recipe files are edited by people, in separate repositories,
// so structural mistakes (a typo'd key, a string where a list belongs) are
// caught here with a precise message. Cross-file logic — does a referenced
// plugin exist, does a branch resolve — stays in Go.
package schema

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"gopkg.in/yaml.v3"

	schemadata "github.com/mutms/mudev/schema"
)

// Kind names one of the embedded schemas.
type Kind string

// The schemas mudev ships.
const (
	KindPlugin Kind = "plugin.schema.json"
	KindRecipe Kind = "recipe.schema.json"
)

// compiled caches one compiled schema per kind — compiling is not free and a
// single run validates many files.
var (
	compiledMu sync.Mutex
	compiled   = map[Kind]*jsonschema.Resolved{}
)

// Decode reads YAML (JSON is a subset, so a .mudev.json parses too) and
// returns the document in the shape JSON Schema validation expects.
//
// It returns both the generic document — for Validate — and the equivalent
// JSON bytes, which callers unmarshal into their typed structs. Going through
// JSON is deliberate: it normalises YAML's own types (dates, integer flavours)
// into what the validator and encoding/json both understand, and it means the
// data model needs only one set of struct tags.
func Decode(raw []byte) (doc any, jsonBytes []byte, err error) {
	var value any

	if err := yaml.Unmarshal(raw, &value); err != nil {
		return nil, nil, fmt.Errorf("parse yaml: %w", err)
	}

	jsonBytes, err = json.Marshal(value)
	if err != nil {
		return nil, nil, fmt.Errorf("normalise to json: %w", err)
	}

	// Unmarshal again from JSON so numbers, keys and nesting are exactly what
	// the validator will see.
	if err := json.Unmarshal(jsonBytes, &doc); err != nil {
		return nil, nil, fmt.Errorf("normalise to json: %w", err)
	}

	return doc, jsonBytes, nil
}

// Validate checks a decoded document (see Decode) against one of the schemas.
func Validate(kind Kind, doc any) error {
	s, err := load(kind)
	if err != nil {
		return err
	}

	if err := s.Validate(doc); err != nil {
		return fmt.Errorf("schema validation failed:\n%w", err)
	}

	return nil
}

// load parses and resolves the named schema once, then caches it — resolving
// is not free and a single run validates many files.
func load(kind Kind) (*jsonschema.Resolved, error) {
	compiledMu.Lock()
	defer compiledMu.Unlock()

	if s, ok := compiled[kind]; ok {
		return s, nil
	}

	raw, err := schemadata.Files.ReadFile(string(kind))
	if err != nil {
		return nil, fmt.Errorf("embedded schema %s: %w", kind, err)
	}

	var parsed jsonschema.Schema

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("embedded schema %s: %w", kind, err)
	}

	// The schemas are self-contained (no remote $ref), so no loader is needed.
	resolved, err := parsed.Resolve(nil)
	if err != nil {
		return nil, fmt.Errorf("embedded schema %s: %w", kind, err)
	}

	compiled[kind] = resolved

	return resolved, nil
}
