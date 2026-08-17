package workspace

import (
	"encoding/json"
	"fmt"
)

// deepCopy duplicates a decoded document. Catalogue entries are cached and
// shared, so anything mudev is about to merge into must be copied first.
func deepCopy(doc map[string]any) map[string]any {
	if doc == nil {
		return map[string]any{}
	}

	data, err := json.Marshal(doc)
	if err != nil {
		// A document that came from JSON cannot fail to go back to JSON.
		panic(fmt.Sprintf("copy document: %v", err))
	}

	var out map[string]any

	if err := json.Unmarshal(data, &out); err != nil {
		panic(fmt.Sprintf("copy document: %v", err))
	}

	return out
}

// deepMerge overlays over onto base and returns the result.
//
// Nested mappings merge key by key — that is what makes a recipe entry able to
// pin just source.git.ref while the remotes come from the catalogue. Anything
// that is not a mapping (a string, a list) is replaced wholesale: a recipe that
// restates a list means to replace it, not to extend it.
func deepMerge(base map[string]any, over map[string]any) map[string]any {
	out := deepCopy(base)

	for key, value := range over {
		existing, inBase := out[key]

		baseMap, baseIsMap := existing.(map[string]any)
		overMap, overIsMap := value.(map[string]any)

		if inBase && baseIsMap && overIsMap {
			out[key] = deepMerge(baseMap, overMap)

			continue
		}

		out[key] = value
	}

	return out
}

// narrowSourceToGit rewrites a flattened definition's source down to the one
// kind mudev resolved, with the ref it actually checked out.
//
// A catalogue entry may advertise several ways to fetch the same code; the
// live recipe records only the way this checkout was made — the source recipe
// is composer.json, the live recipe is the lock.
func narrowSourceToGit(definition map[string]any, remotes map[string]string, ref string) {
	git := map[string]any{}

	// Preserve any git fields mudev does not model (they came from the
	// catalogue and may matter to another consumer).
	if source, ok := definition["source"].(map[string]any); ok {
		if existing, ok := source["git"].(map[string]any); ok {
			git = deepCopy(existing)
		}
	}

	named := make(map[string]any, len(remotes))
	for name, url := range remotes {
		named[name] = url
	}

	git["remotes"] = named
	git["ref"] = ref

	definition["source"] = map[string]any{"git": git}
}
