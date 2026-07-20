// Package schema carries mudev's JSON Schema files as embedded data.
//
// The .json files are the hand-maintained originals — they are also published
// at stable URLs so editors can autocomplete YAML through a
// `# yaml-language-server: $schema=…` modeline. go:embed can only reach files
// below its own directory, which is why this tiny package sits next to them;
// the validation logic that uses it lives in internal/schema.
package schema

import "embed"

// Files holds the embedded schema documents, by file name.
//
//go:embed plugin.schema.json recipe.schema.json
var Files embed.FS
