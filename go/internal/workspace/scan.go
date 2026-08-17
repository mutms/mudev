package workspace

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mutms/mudev/go/internal/git"
	"github.com/mutms/mudev/go/internal/moodle"
)

// scanner reads the git and version.php facts off an existing tree and turns a
// checkout into a live-recipe block. It is the shared reading layer under
// `recipe init` (which scans the whole tree at once) and `recipe update` (which
// scans one checkout to adopt or refresh it).
//
// It changes no working tree: it only asks git what each checkout is and reads
// each plugin's version.php, then hands back the block to record.
type scanner struct {
	client *git.Client
	root   string
	out    output

	// strippublic is read off the tree by detectLayout: core with a public/
	// code root is 5.1+, without it an older branch. Every plugin relpath is
	// stored in the newest (public/) layout, so a stripped tree's paths get the
	// prefix put back.
	strippublic bool
}

// Conditions under which a checkout cannot be recorded. `init` treats them as a
// reason to skip one checkout and carry on; `update`, given one checkout by
// name, reports them as the reason it could not act.
var (
	errNoOrigin   = errors.New("has no origin remote to record")
	errUnborn     = errors.New("has no commit yet")
	errUnnameable = errors.New("could not be given a vendor/package name")
)

// identifierPattern is the vendor/package spelling the recipe schema accepts
// for a plugin name. A reconstructed name is built by hand from on-disk facts,
// so it must be something the schema will validate when the file is loaded.
var identifierPattern = regexp.MustCompile(`^[a-z0-9]([_.-]?[a-z0-9]+)*/[a-z0-9](([_.]?|-{0,2})[a-z0-9]+)*$`)

// detectLayout reads core's version.php to learn the Moodle $branch code and
// whether the tree uses the public/ layout, setting strippublic as a side
// effect. A root that is not a Moodle code tree is refused here, before
// anything is recorded.
func (s *scanner) detectLayout() (mdlbranch string, err error) {
	mdlbranch, err = moodle.Branch(s.root, false)
	if err != nil {
		if mdlbranch, err = moodle.Branch(s.root, true); err != nil {
			return "", fmt.Errorf(
				"%s is not a Moodle core tree (no readable version.php with a $branch): %w",
				s.root, err,
			)
		}

		s.strippublic = true
	}

	return mdlbranch, nil
}

// baseBlock builds the recipe's base block from the core checkout at the root.
// detectLayout must have run first, so strippublic is known.
func (s *scanner) baseBlock(ctx context.Context, mdlbranch string) (map[string]any, error) {
	remotes, ref, localbranch, err := s.identity(ctx, s.root)
	if err != nil {
		return nil, fmt.Errorf("core checkout %w", err)
	}

	base := map[string]any{
		"mdlbranch": mdlbranch,
		"source":    map[string]any{"git": gitBlock(remotes, ref)},
	}

	if s.strippublic {
		base["strippublic"] = true
	}

	applyLocalbranch(base, localbranch, ref, remotes)

	return base, nil
}

// pluginEntry builds a flattened plugin entry for the checkout at path,
// deriving its <owner>/<component> identity from disk. detectLayout must have
// run first. A checkout that cannot be named or recorded comes back as one of
// the sentinel errors above.
func (s *scanner) pluginEntry(ctx context.Context, path string) (map[string]any, error) {
	dir := filepath.Join(s.root, path)

	remotes, ref, localbranch, err := s.identity(ctx, dir)
	if err != nil {
		return nil, err
	}

	component, err := moodle.Component(dir)
	if err != nil {
		return nil, err
	}

	name, err := reconstructName(remoteOwner(remotes["origin"]), component, remotes["origin"])
	if err != nil {
		return nil, err
	}

	relpath := path
	if s.strippublic {
		relpath = moodle.PublicPrefix + path
	}

	entry := map[string]any{
		"name":    name,
		"relpath": relpath,
		"source":  map[string]any{"git": gitBlock(remotes, ref)},
	}

	applyLocalbranch(entry, localbranch, ref, remotes)

	return entry, nil
}

// identity reads the git facts a live-recipe entry records for a checkout: its
// remotes, the ref to record, and the local branch it is on.
func (s *scanner) identity(ctx context.Context, dir string) (remotes map[string]string, ref string, localbranch string, err error) {
	remotes, err = s.client.Remotes(ctx, dir)
	if err != nil {
		return nil, "", "", err
	}

	if remotes["origin"] == "" {
		return nil, "", "", errNoOrigin
	}

	ref, localbranch, unborn, err := s.recordedRef(ctx, dir, remotes)
	if err != nil {
		return nil, "", "", err
	}

	if unborn {
		return nil, "", "", errUnborn
	}

	return remotes, ref, localbranch, nil
}

// recordedRef works out the ref and local branch to write down for a checkout,
// from the state git reports.
//
// A checkout on a branch is recorded by that branch's upstream ("origin/…"),
// the same remote-tracking spelling clone writes, so status can later tell a
// checkout that strayed onto another branch. A detached checkout on a tag is
// recorded by the tag; one on neither is recorded by its commit — the honest
// baseline for a tree pinned to a commit. A branch with no upstream is a local
// branch mudev cannot spell against a remote, so its commit is recorded and the
// developer is told what was assumed.
func (s *scanner) recordedRef(ctx context.Context, dir string, remotes map[string]string) (ref string, localbranch string, unborn bool, err error) {
	st, err := s.client.Status(ctx, dir)
	if err != nil {
		return "", "", false, err
	}

	if st.Unborn {
		return "", "", true, nil
	}

	if st.Detached {
		if len(st.Tags) > 0 {
			return st.Tags[0], "", false, nil
		}

		sha, err := s.client.CommitSHA(ctx, dir)

		return sha, "", false, err
	}

	if st.Tracking != "" {
		return st.Tracking, st.Branch, false, nil
	}

	// On a branch with no upstream: there is no remote-tracking ref to point at,
	// so record the commit and say so, rather than invent an origin/<branch>
	// that may not exist.
	sha, err := s.client.CommitSHA(ctx, dir)
	if err != nil {
		return "", "", false, err
	}

	s.out.warnf("%s is on local branch %q with no upstream — recorded its commit; set a tracking branch and re-run to record it by branch",
		relOrDot(s.root, dir), st.Branch)

	return sha, "", false, nil
}

// layoutNote spells out the code-root layout, which decides every plugin path.
func (s *scanner) layoutNote() string {
	if s.strippublic {
		return ", no public/ prefix"
	}

	return ", public/ layout"
}

// applyLocalbranch records a localbranch on a base/plugin definition only when
// it adds something the default does not already give: the ref's own branch
// name. On a refresh it also clears a localbranch that is no longer needed.
func applyLocalbranch(definition map[string]any, localbranch string, ref string, remotes map[string]string) {
	if localbranch != "" && localbranch != branchOf(ref, remotes) {
		definition["localbranch"] = localbranch

		return
	}

	delete(definition, "localbranch")
}

// gitBlock is a source.git block: every remote the checkout has, and the ref to
// record. All remotes are kept, not just origin, so a fork's upstream or a LAN
// mirror is ready for fetch straight away.
func gitBlock(remotes map[string]string, ref string) map[string]any {
	named := make(map[string]any, len(remotes))
	for name, url := range remotes {
		named[name] = url
	}

	return map[string]any{"remotes": named, "ref": ref}
}

// branchOf is the branch part of a "<remote>/<branch>" ref, or empty for a tag
// or commit. It decides whether a localbranch needs recording at all: when it
// equals the local branch, the default already covers it.
func branchOf(ref string, remotes map[string]string) string {
	if _, branch, ok := git.SplitBranchRef(ref, remotes); ok {
		return branch
	}

	return ""
}

// reconstructName forms a vendor/package identifier from what the checkout can
// tell about itself: its frankenstyle component (the authoritative package
// name) under the owner of its origin remote (the vendor). Where the component
// is unreadable it falls back to the repository's own name.
func reconstructName(owner string, component string, origin string) (string, error) {
	pkg := component
	if pkg == "" {
		pkg = remoteRepo(origin)
	}

	vendor := strings.ToLower(owner)
	if vendor == "" {
		vendor = "local"
	}

	name := vendor + "/" + pkg

	if !identifierPattern.MatchString(name) {
		return "", fmt.Errorf(
			"%w from component %q and remote owner %q",
			errUnnameable, component, owner,
		)
	}

	return name, nil
}

// remotePath is the owner/repo path of a git URL, with any scheme, host,
// credentials and trailing ".git" removed. It handles the three shapes a remote
// takes: scheme URLs (https://, ssh://, file://), scp-like SSH
// (git@host:owner/repo), and bare local paths.
func remotePath(url string) string {
	u := strings.TrimSuffix(strings.TrimSpace(url), ".git")

	if idx := strings.Index(u, "://"); idx >= 0 {
		rest := u[idx+3:]

		if slash := strings.Index(rest, "/"); slash >= 0 {
			return strings.Trim(rest[slash+1:], "/")
		}

		return ""
	}

	if colon := strings.Index(u, ":"); colon >= 0 {
		return strings.Trim(u[colon+1:], "/")
	}

	return strings.Trim(u, "/")
}

// remoteOwner is the owner (vendor) segment of a git URL: the second-to-last
// path element, e.g. "mutms" in git@github.com:mutms/moodle-tool_mulib.git.
func remoteOwner(url string) string {
	parts := strings.Split(remotePath(url), "/")

	if len(parts) >= 2 && parts[len(parts)-2] != "" {
		return parts[len(parts)-2]
	}

	return ""
}

// remoteRepo is the repository name from a git URL — the last path element,
// with a leading "moodle-" dropped, since Moodle plugin repos are conventionally
// named moodle-<component>. It is the fallback package name when a checkout has
// no readable component.
func remoteRepo(url string) string {
	parts := strings.Split(remotePath(url), "/")

	return strings.TrimPrefix(parts[len(parts)-1], "moodle-")
}

// relOrDot renders dir relative to root for a message, falling back to "." for
// the root itself.
func relOrDot(root string, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return CoreDir
	}

	return filepath.ToSlash(rel)
}
