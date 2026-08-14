package workspace

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mutms/mudev/internal/config"
	"github.com/mutms/mudev/internal/moodle"
	"github.com/mutms/mudev/internal/recipe"
)

// The camp registry (https://camp-registry.org) keeps a plugin's descriptive
// content in the plugin's own repository, at .camp/listing.yml, and ingests it
// at each tagged release. These are its published constants.
const (
	// campDir and campListing are where a listing manifest lives, relative to
	// a plugin's repository root.
	campDir     = ".camp"
	campListing = "listing.yml"

	// campSchemaURL is written as a modeline so an editor validates the file,
	// the same service `mudev recipe export` does for a recipe.
	campSchemaURL = "https://camp-registry.org/schema/listing.schema.json"

	// campSummaryMax and campNameMax are the schema's own limits. Exceeding
	// either is a validation failure at the registry, so a generated summary
	// is truncated rather than written long.
	campNameMax    = 80
	campSummaryMax = 200

	// campBadgeHost serves the MDL Shield security grade. It is on camp's
	// badge allowlist, and the endpoint document is keyed by component.
	campBadgeHost = "https://mdlshield.com"
)

// campLabels is the closed set of disclosure labels the schema accepts.
//
// fully-free, freemium, paid-service and external-account are disclosures —
// what the plugin costs the user. donation-supported and
// commercial-support-available are promotional, so leaving them off is a
// choice rather than an omission.
var campLabels = []string{
	"fully-free",
	"freemium",
	"paid-service",
	"external-account",
	"donation-supported",
	"commercial-support-available",
}

// campExportIgnore keeps development files out of camp's distribution ZIPs,
// which are built with git archive semantics. Reproduced from camp's own
// author-side scaffolding so a repository set up by mudev and one set up by
// `camp scaffold` carry the same rules.
var campExportIgnore = []string{
	"# Keep development files out of camp distribution ZIPs (git archive semantics)",
	".github export-ignore",
	".gitattributes export-ignore",
	".gitignore export-ignore",
	".camp export-ignore",
	"tests export-ignore",
	"node_modules export-ignore",
}

// CampInitOptions configure writing camp listing manifests into a workspace's
// plugin checkouts.
type CampInitOptions struct {
	// Root is the workspace to work in.
	Root string

	// Relpath selects a single plugin, spelled as `mudev list` prints it.
	// Empty means every plugin the live recipe records.
	Relpath string

	// Labels are the disclosure labels to declare. Empty means fully-free.
	Labels []string

	// NoBadges leaves the MDL Shield badge out.
	NoBadges bool

	// Force rewrites a listing manifest that is already there.
	Force bool

	// Out receives the progress lines.
	Out io.Writer
}

// CampInit writes a .camp/listing.yml into each plugin checkout, and the
// export-ignore rules that go with it.
//
// The listing is generated rather than copied from a template because the
// workspace already knows everything it needs: the display name is the
// plugin's own pluginname string, the summary is its README's opening line,
// and the repository links come from the remotes the live recipe recorded.
// camp's own `camp scaffold` has none of that context and falls back to the
// component name, which is an identifier and not a title.
//
// It never overwrites an existing manifest. The file is meant to be edited by
// hand after generation — that is where the prose goes — so a second run must
// not quietly undo the author's work.
func CampInit(opts CampInitOptions) error {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}

	labels, err := campCheckLabels(opts.Labels)
	if err != nil {
		return err
	}

	live, err := recipe.Load(LivePath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf(
				"%s has no %s — assemble a workspace there with `mudev clone <recipe>` first",
				root, config.LiveRecipeFile,
			)
		}

		return err
	}

	selected, err := campSelect(live, opts.Relpath)
	if err != nil {
		return err
	}

	out := newOutput(opts.Out)

	for _, plugin := range selected {
		if err := campWritePlugin(root, plugin, labels, opts, out); err != nil {
			return err
		}
	}

	return nil
}

// campPlugin is one plugin of the live recipe, resolved to where it sits in
// this tree.
type campPlugin struct {
	// Entry is the recorded recipe entry.
	Entry recipe.Entry

	// Relpath is the path in the tree, in this recipe's layout.
	Relpath string
}

// campSelect resolves the recorded plugins, narrowed to one when a relpath was
// given.
func campSelect(live *recipe.Recipe, relpath string) ([]campPlugin, error) {
	var plugins []campPlugin

	for i := range live.Plugins {
		entry := live.Plugins[i]

		path, err := moodle.PluginPath(entry.Relpath, live.Base.Strippublic)
		if err != nil {
			return nil, fmt.Errorf("plugin %s: %w", entry.Name, err)
		}

		plugins = append(plugins, campPlugin{Entry: entry, Relpath: path})
	}

	if relpath == "" {
		return plugins, nil
	}

	wanted, err := cleanRelpath(relpath)
	if err != nil {
		return nil, err
	}

	if wanted == CoreDir {
		return nil, fmt.Errorf(
			"a camp listing describes one plugin; %q is Moodle core", CoreDir,
		)
	}

	for _, plugin := range plugins {
		if plugin.Relpath == wanted {
			return []campPlugin{plugin}, nil
		}
	}

	return nil, fmt.Errorf("%s is not a plugin this workspace records", wanted)
}

// campWritePlugin generates one plugin's camp files.
func campWritePlugin(root string, plugin campPlugin, labels []string, opts CampInitOptions, out output) error {
	dir := filepath.Join(root, plugin.Relpath)

	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			out.printf("%s", plugin.Entry.Name)
			out.warnf("%s is not checked out — skipped", plugin.Relpath)

			return nil
		}

		return err
	}

	// A fork's listing belongs to the plugin the fork came from: camp keys an
	// entry to one canonical source repository, and ours is not it. The same
	// rule the release commands follow for tags.
	if upstream := campUpstream(plugin.Entry); upstream != "" {
		out.printf("%s", plugin.Entry.Name)
		out.warnf("skipped — fork of %s", upstream)

		return nil
	}

	component, err := moodle.Component(dir)
	if err != nil {
		return err
	}

	if component == "" {
		component = campComponentFromName(plugin.Entry.Name)
	}

	out.printf("%s", plugin.Entry.Name)

	listing, err := campBuildListing(dir, component, labels, opts.NoBadges, plugin.Entry, out)
	if err != nil {
		return err
	}

	if err := campWriteListing(dir, plugin.Relpath, listing, opts.Force, out); err != nil {
		return err
	}

	return campWriteGitattributes(dir, plugin.Relpath, out)
}

// campBuildListing gathers what the listing says about one plugin.
func campBuildListing(dir, component string, labels []string, noBadges bool, entry recipe.Entry, out output) (*campListingDoc, error) {
	doc := &campListingDoc{Labels: labels}

	name, err := moodle.PluginName(dir, component)
	if err != nil {
		return nil, err
	}

	if name == "" {
		name = component
		out.warnf("no pluginname string — using the component as the name")
	}

	doc.Name = campTruncate(name, campNameMax)

	summary, err := campSummary(dir)
	if err != nil {
		return nil, err
	}

	if summary == "" {
		summary = "One line about what this plugin does."
		out.warnf("no README lead paragraph — write the summary by hand")
	}

	doc.Summary = campTruncate(summary, campSummaryMax)

	if repoURL := campRepoURL(entry); repoURL != "" {
		doc.Docs = repoURL + "#readme"
		doc.Issues = repoURL + "/issues"
	} else {
		out.warnf("no GitHub origin remote — links omitted")
	}

	if !noBadges && component != "" {
		doc.BadgeEndpoint = campBadgeHost + "/api/badge/" + component
		doc.BadgeLink = campBadgeHost + "/plugins/" + component
	}

	return doc, nil
}

// campListingDoc is the manifest mudev generates. It is deliberately narrower
// than the schema: description, category and screenshots are the author's to
// add, and a generated guess at any of them would be worse than their absence.
type campListingDoc struct {
	Name    string
	Summary string
	Labels  []string

	Docs   string
	Issues string

	BadgeEndpoint string
	BadgeLink     string
}

// campWriteListing writes .camp/listing.yml, leaving an existing one alone.
func campWriteListing(dir, relpath string, doc *campListingDoc, force bool, out output) error {
	path := filepath.Join(dir, campDir, campListing)
	shown := filepath.Join(relpath, campDir, campListing)

	if _, err := os.Stat(path); err == nil && !force {
		out.stepf("%s exists — left alone (--force rewrites it)", shown)

		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	document, err := campRender(doc)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(dir, campDir), 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(path, document, 0o644); err != nil {
		return err
	}

	out.stepf("wrote %s", shown)

	return nil
}

// campRender turns a listing into the bytes of the file.
//
// The document is built as YAML nodes rather than formatted as a string: it
// gets correct quoting for any name or summary for free, and nodes carry the
// comments — the label vocabulary and the description hint — that make the
// generated file editable without opening the schema.
func campRender(doc *campListingDoc) ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}

	campPut(root, campScalar("name"), campScalar(doc.Name))
	campPut(root, campScalar("summary"), campScalar(doc.Summary))

	labels := &yaml.Node{Kind: yaml.SequenceNode}
	for _, label := range doc.Labels {
		labels.Content = append(labels.Content, campScalar(label))
	}

	labelsKey := campScalar("labels")
	labelsKey.HeadComment = "Disclosure labels — all that apply:\n  " +
		strings.Join(campLabels[:4], " | ") + "\n  " +
		strings.Join(campLabels[4:], " | ")

	campPut(root, labelsKey, labels)

	if doc.Docs != "" || doc.Issues != "" {
		links := &yaml.Node{Kind: yaml.MappingNode}

		if doc.Docs != "" {
			campPut(links, campScalar("docs"), campScalar(doc.Docs))
		}

		if doc.Issues != "" {
			campPut(links, campScalar("issues"), campScalar(doc.Issues))
		}

		campPut(root, campScalar("links"), links)
	}

	if doc.BadgeEndpoint != "" {
		badge := &yaml.Node{Kind: yaml.MappingNode}

		campPut(badge, campScalar("endpoint"), campScalar(doc.BadgeEndpoint))

		if doc.BadgeLink != "" {
			campPut(badge, campScalar("link"), campScalar(doc.BadgeLink))
		}

		badges := &yaml.Node{Kind: yaml.SequenceNode, Content: []*yaml.Node{badge}}

		campPut(root, campScalar("badges"), badges)
	}

	var body bytes.Buffer

	encoder := yaml.NewEncoder(&body)
	encoder.SetIndent(yamlIndent)

	if err := encoder.Encode(root); err != nil {
		return nil, err
	}

	if err := encoder.Close(); err != nil {
		return nil, err
	}

	header := "# yaml-language-server: $schema=" + campSchemaURL + "\n" +
		"# camp listing manifest — this plugin's page at https://camp-registry.org/\n" +
		"# Generated by `mudev camp init`. Edit freely and commit: camp ingests\n" +
		"# this file at each tagged release.\n" +
		"#\n" +
		"# Add prose with a description block (markdown, no raw HTML):\n" +
		"#   description: |\n" +
		"#     What the plugin does, and who it is for.\n"

	return append([]byte(header), body.Bytes()...), nil
}

// campScalar is one plain YAML scalar node.
func campScalar(value string) *yaml.Node {
	node := &yaml.Node{}

	// Encode rather than setting Value, so a summary holding a colon, a quote
	// or a leading dash is quoted the way it needs to be.
	_ = node.Encode(value)

	return node
}

// campPut appends one key/value pair to a mapping node.
func campPut(mapping, key, value *yaml.Node) {
	mapping.Content = append(mapping.Content, key, value)
}

// campWriteGitattributes appends camp's export-ignore rules, unless the
// repository already declares some of its own.
func campWriteGitattributes(dir, relpath string, out output) error {
	path := filepath.Join(dir, ".gitattributes")
	shown := filepath.Join(relpath, ".gitattributes")

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if bytes.Contains(existing, []byte("export-ignore")) {
		out.stepf("%s already has export-ignore rules", shown)

		return nil
	}

	body := string(existing)

	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}

	if body != "" {
		body += "\n"
	}

	body += strings.Join(campExportIgnore, "\n") + "\n"

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}

	verb := "wrote"
	if len(existing) > 0 {
		verb = "appended to"
	}

	out.stepf("%s %s", verb, shown)

	return nil
}

// campCheckLabels validates the requested disclosure labels, defaulting to the
// one that describes a GPL plugin with nothing behind a paywall.
//
// Mislabeling is a trust violation at the registry, so an unknown label is an
// error rather than something passed through for CI to reject later.
func campCheckLabels(labels []string) ([]string, error) {
	if len(labels) == 0 {
		return []string{"fully-free"}, nil
	}

	var checked []string

	for _, label := range labels {
		label = strings.TrimSpace(label)

		if label == "" {
			continue
		}

		known := false

		for _, valid := range campLabels {
			if label == valid {
				known = true

				break
			}
		}

		if !known {
			return nil, fmt.Errorf(
				"%s is not a camp disclosure label; valid labels are %s",
				label, strings.Join(campLabels, ", "),
			)
		}

		checked = append(checked, label)
	}

	if len(checked) == 0 {
		return nil, fmt.Errorf("at least one disclosure label is required")
	}

	return checked, nil
}

// campUpstream names the upstream a plugin was forked from, empty when it is
// our own. A recipe entry records every remote it was cloned with, and an
// `upstream` among them is exactly what marks a fork.
func campUpstream(entry recipe.Entry) string {
	if entry.Source == nil || entry.Source.Git == nil {
		return ""
	}

	upstream, ok := entry.Source.Git.Remotes["upstream"]
	if !ok {
		return ""
	}

	if slug := campGitHubSlug(upstream); slug != "" {
		return slug
	}

	return upstream
}

// campRepoURL is the browsable https URL of a plugin's origin remote, empty
// when the origin is not GitHub — camp's author flow is GitHub and GitLab, and
// mudev only knows how to spell GitHub's web URLs.
func campRepoURL(entry recipe.Entry) string {
	if entry.Source == nil || entry.Source.Git == nil {
		return ""
	}

	slug := campGitHubSlug(entry.Source.Git.Remotes["origin"])
	if slug == "" {
		return ""
	}

	return "https://github.com/" + slug
}

// gitHubRemotePattern matches the two spellings a GitHub remote takes, ssh and
// https, capturing owner/repo.
var gitHubRemotePattern = regexp.MustCompile(
	`^(?:git@github\.com:|(?:https|ssh)://(?:[^@/]+@)?github\.com/)([^/]+/[^/]+?)(?:\.git)?/?$`,
)

// campGitHubSlug extracts owner/repo from a GitHub remote URL.
func campGitHubSlug(remote string) string {
	match := gitHubRemotePattern.FindStringSubmatch(strings.TrimSpace(remote))
	if match == nil {
		return ""
	}

	return match[1]
}

// campComponentFromName falls back to the package half of a recipe identifier
// when a checkout has no readable version.php — `mutms/tool_muprog` is the
// component `tool_muprog`.
func campComponentFromName(name string) string {
	if _, pkg, found := strings.Cut(name, "/"); found {
		return pkg
	}

	return name
}

// badgeLinePattern matches a markdown line that is only images and links —
// the badge row almost every plugin README opens with, which says nothing
// about what the plugin does.
var badgeLinePattern = regexp.MustCompile(`^\s*(?:\[?!\[[^\]]*\]\([^)]*\)(?:\]\([^)]*\))?\s*)+$`)

// campSummary reads a plugin's README for its opening line of prose.
//
// It takes the first line that is not the title, not the badge row, and not a
// list item or a fence — which is how a human skims a README for "what is
// this?" and, in practice, exactly the sentence the author already wrote for
// that purpose.
func campSummary(dir string) (string, error) {
	for _, name := range []string{"README.md", "README.txt", "README"} {
		file, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return "", err
		}

		summary := campScanSummary(file)

		if err := file.Close(); err != nil {
			return "", err
		}

		return summary, nil
	}

	return "", nil
}

// campScanSummary picks the lead paragraph out of a README.
func campScanSummary(r io.Reader) string {
	var paragraph []string

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// A blank line ends the paragraph — but only once one has started, so
		// the blank lines around the title and badges are skipped over.
		if line == "" {
			if len(paragraph) > 0 {
				break
			}

			continue
		}

		if len(paragraph) == 0 && campSkipLine(line) {
			continue
		}

		paragraph = append(paragraph, line)
	}

	return campFlatten(strings.Join(paragraph, " "))
}

// campSkipLine reports whether a README line is chrome rather than prose.
func campSkipLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "#"):
		return true
	case strings.HasPrefix(line, "```"):
		return true
	case strings.HasPrefix(line, ">"):
		return true
	case strings.HasPrefix(line, "- "), strings.HasPrefix(line, "* "):
		return true
	case strings.HasPrefix(line, "<"):
		return true
	case badgeLinePattern.MatchString(line):
		return true
	}

	return false
}

// campTruncate keeps a generated value inside the schema's limit, cutting at a
// word boundary and marking the cut so it reads as deliberate.
//
// The limit is counted in characters, not bytes: that is what JSON Schema's
// maxLength means, and a MuTMS summary is full of multibyte "™" and "—" — a
// byte count would cut short summaries that are comfortably within the limit.
func campTruncate(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")

	runes := []rune(value)
	if len(runes) <= max {
		return value
	}

	cut := string(runes[:max-1])

	if space := strings.LastIndex(cut, " "); space > 0 && len([]rune(cut[:space])) > max/2 {
		cut = cut[:space]
	}

	return strings.TrimRight(cut, " ,;:—-") + "…"
}

// markdownLinkPattern matches an inline markdown link, capturing its text and
// its target.
var markdownLinkPattern = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)

// campFlatten renders a line of markdown as the plain text a summary field
// wants: camp shows the summary on search results and cards, where `[MuTMS
// suite](https://github.com/mutms)` would be read literally. A link with no
// text at all leaves its target behind, which is what a reader would have
// followed anyway.
func campFlatten(line string) string {
	return markdownLinkPattern.ReplaceAllStringFunc(line, func(match string) string {
		parts := markdownLinkPattern.FindStringSubmatch(match)

		if text := strings.TrimSpace(parts[1]); text != "" {
			return text
		}

		return parts[2]
	})
}
