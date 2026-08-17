package moodle

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// pluginNamePattern matches the one language string every Moodle plugin
// declares:
//
//	$string['pluginname'] = 'Programs';
//
// Both quote styles are spelled out as alternatives rather than matched with a
// back-reference, which Go's regexp engine does not have. Each alternative
// excludes only its own delimiter, so a name may contain the other one.
var pluginNamePattern = regexp.MustCompile(
	`\$string\[\s*['"]pluginname['"]\s*\]\s*=\s*(?:'([^']*)'|"([^"]*)")\s*;`,
)

// PluginName reads a plugin's display name from its English language file.
//
// This is the name Moodle itself shows in the plugin overview, so it is the
// closest thing a plugin has to a human title — `$plugin->component` is an
// identifier, not a name. A directory with no language file, or one that
// declares no pluginname, yields an empty string and no error: the caller
// falls back to the component.
//
// PHP escape sequences are not interpreted. A name containing one is rare
// enough, and rendering `\'` as a literal backslash in a listing is a visible
// mistake the author can fix, where silently mangling a name would not be.
func PluginName(dir, component string) (string, error) {
	if component == "" {
		return "", nil
	}

	var content []byte

	for _, name := range langFileNames(component) {
		data, err := os.ReadFile(filepath.Join(dir, "lang", "en", name))
		if err == nil {
			content = data

			break
		}

		if !os.IsNotExist(err) {
			return "", err
		}
	}

	match := pluginNamePattern.FindSubmatch(content)
	if match == nil {
		return "", nil
	}

	// Exactly one of the two alternatives matched; the other group is nil.
	value := match[1]
	if value == nil {
		value = match[2]
	}

	return strings.TrimSpace(string(value)), nil
}

// langFileNames lists the names a plugin's English language file may take,
// most specific first.
//
// Nearly every plugin type names the file after its component. Activity
// modules are the exception Moodle carries from before frankenstyle: mod_mubook
// keeps its strings in lang/en/mubook.php, and $string['pluginname'] there is
// what the activity chooser shows.
func langFileNames(component string) []string {
	names := []string{component + ".php"}

	if modname, found := strings.CutPrefix(component, "mod_"); found && modname != "" {
		names = append(names, modname+".php")
	}

	return names
}
