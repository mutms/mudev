package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mutms/mudev/go/internal/config"
	"github.com/mutms/mudev/go/internal/moodle"
	"github.com/mutms/mudev/go/internal/recipe"
)

// minReleaseBranch is the first Moodle branch whose MuTMS releases mudev
// makes. Releases up to 5.2 are made with the older release tool, whose
// version and branch conventions differ.
const minReleaseBranch = 503

// TodayVersion is the value `release set version` expands to today's stamp.
const TodayVersion = "today"

// SetVersionOptions configure setting the version of every release-managed
// plugin in a workspace.
type SetVersionOptions struct {
	// Config is the resolved configuration.
	Config config.Config

	// Root is the workspace directory.
	Root string

	// Value is the version stamp to set, e.g. 2026110553, or TodayVersion.
	Value string

	// Now is the clock TodayVersion is read from; nil means time.Now.
	Now func() time.Time

	// Out receives mudev's own lines.
	Out io.Writer
}

// versionPlan is the change to one plugin's version.php, computed in full
// before any file is written.
type versionPlan struct {
	repo         Repo
	path         string
	mode         os.FileMode
	content      []byte
	from         string
	dependencies []moodle.DependencyChange
}

// SetVersion sets $plugin->version in every release-managed plugin of the
// workspace, and points their dependencies on each other at the same version.
//
// MuTMS plugins are released together under one version stamp: the date, then
// the two-digit branch suffix (2026110553 is 5 November 2026 on 503). A
// release-managed plugin is one whose live recipe entry carries the recipe's
// own extra.mudev.release flavour.
//
// A plugin is passed over, with the reason printed, when its checkout is
// missing, when the recipe pins it to a tag or commit, when its recorded branch
// belongs to another Moodle branch (a plugin shared with an older release
// line), or when it is checked out on another branch than the recorded one.
//
// Every change is worked out before anything is written. Moodle refuses to
// downgrade a plugin, so if the new version is lower than any plugin's current
// one, nothing is written at all. Nothing is committed either: the diff is the
// developer's to review.
func SetVersion(ctx context.Context, opts SetVersionOptions) error {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}

	live, err := loadReleaseRecipe(root)
	if err != nil {
		return err
	}

	mdlbranch := live.Base.Mdlbranch
	flavour := live.Release()

	version, err := resolveVersion(opts, mdlbranch)
	if err != nil {
		return err
	}

	repos, err := List(ctx, ListOptions{Config: opts.Config, Root: root})
	if err != nil {
		return err
	}

	out := newOutput(opts.Out)

	selected, components, err := selectReleasePlugins(root, repos, flavour, mdlbranch, out)
	if err != nil {
		return err
	}

	if len(selected) == 0 {
		out.printf("no release-managed plugins to change in %s", root)

		return nil
	}

	plans, err := planVersions(root, selected, components, version)
	if err != nil {
		return err
	}

	unchanged := 0

	for _, plan := range plans {
		if plan.content == nil {
			unchanged++

			continue
		}

		if err := os.WriteFile(plan.path, plan.content, plan.mode); err != nil {
			return fmt.Errorf("%s: %w", plan.repo.Path, err)
		}

		out.printf("%s", header(plan.repo.Path))

		if plan.from != version {
			out.stepf("version %s → %s", plan.from, version)
		}

		for _, dependency := range plan.dependencies {
			out.stepf("dependency %s %s → %s", dependency.Component, dependency.From, dependency.To)
		}
	}

	if unchanged > 0 {
		out.printf("%d plugin(s) already at %s", unchanged, version)
	}

	out.printf("version %s set in %d plugin(s); review and commit the changes", version, len(plans)-unchanged)

	return nil
}

// loadReleaseRecipe reads the live recipe and checks that the workspace is one
// a release command applies to.
func loadReleaseRecipe(root string) (*recipe.Recipe, error) {
	path := LivePath(root)

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s has no %s — run this in a workspace made by mudev clone", root, config.LiveRecipeFile)
		}

		return nil, err
	}

	live, err := recipe.Load(path)
	if err != nil {
		return nil, err
	}

	flavour := live.Release()

	if flavour == "" {
		return nil, fmt.Errorf("the workspace recipe sets no extra.mudev.release, so it has no release-managed plugins")
	}

	if !known(flavour) {
		return nil, fmt.Errorf("unknown release flavour %q — this build supports: %s",
			flavour, strings.Join(knownFlavours, ", "))
	}

	branch, err := strconv.Atoi(live.Base.Mdlbranch)
	if err != nil {
		return nil, fmt.Errorf("mdlbranch %q is not a Moodle branch code", live.Base.Mdlbranch)
	}

	if branch < minReleaseBranch {
		return nil, fmt.Errorf(
			"release commands need Moodle 5.3 or later; this workspace is mdlbranch %s — use the old release tool",
			live.Base.Mdlbranch,
		)
	}

	return live, nil
}

// resolveVersion turns the requested value into a checked version stamp.
func resolveVersion(opts SetVersionOptions, mdlbranch string) (string, error) {
	version := opts.Value

	if version == TodayVersion {
		now := opts.Now
		if now == nil {
			now = time.Now
		}

		stamp, err := moodle.DateVersion(now(), mdlbranch)
		if err != nil {
			return "", err
		}

		version = stamp
	}

	if err := moodle.CheckPluginVersion(version, mdlbranch); err != nil {
		return "", err
	}

	return version, nil
}

// selectReleasePlugins picks the plugins a release changes, reporting each
// release-managed one it passes over, and returns their components so their
// dependencies on each other can follow.
func selectReleasePlugins(root string, repos []Repo, flavour, mdlbranch string, out output) ([]Repo, []string, error) {
	var (
		selected   []Repo
		components []string
	)

	branchPrefix := "MOODLE_" + mdlbranch + "_"

	skip := func(repo Repo, format string, args ...any) {
		out.printf("%s", header(repo.Path))
		out.stepf("skipped: "+format, args...)
	}

	for _, repo := range repos {
		if repo.Core || repo.Release != flavour {
			continue
		}

		switch {
		case repo.Missing:
			skip(repo, "not checked out")

			continue

		case repo.RecordedRemoteBranch == "":
			skip(repo, "the recipe pins %s rather than a branch", repo.RecordedRef)

			continue

		case !strings.HasPrefix(repo.RecordedRemoteBranch, branchPrefix):
			skip(repo, "branch %s is not a %s branch", repo.RecordedRemoteBranch, mdlbranch)

			continue

		case repo.Strayed():
			skip(repo, "on branch %s, the recipe expects %s", repo.Status.Branch, repo.RecordedBranch)

			continue

		case repo.Status.Detached:
			skip(repo, "HEAD is detached, the recipe expects branch %s", repo.RecordedBranch)

			continue
		}

		dir := filepath.Join(root, repo.Path)

		if _, err := os.Stat(filepath.Join(dir, "version.php")); err != nil || repo.Version.Version == "" {
			skip(repo, "no $plugin->version in version.php")

			continue
		}

		component, err := moodle.Component(dir)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", repo.Path, err)
		}

		if component != "" {
			components = append(components, component)
		}

		selected = append(selected, repo)
	}

	return selected, components, nil
}

// planVersions works out every version.php rewrite, and refuses the whole run
// if any plugin would go down in version.
func planVersions(root string, selected []Repo, components []string, version string) ([]versionPlan, error) {
	target, err := moodle.VersionNumber(version)
	if err != nil {
		return nil, err
	}

	var (
		plans      []versionPlan
		downgrades []string
	)

	for _, repo := range selected {
		path := filepath.Join(root, repo.Path, "version.php")

		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", repo.Path, err)
		}

		original, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", repo.Path, err)
		}

		current, err := moodle.VersionNumber(repo.Version.Version)
		if err != nil {
			return nil, fmt.Errorf("%s: version.php: %w", repo.Path, err)
		}

		if current > target {
			downgrades = append(downgrades, fmt.Sprintf("%s is at %s", repo.Path, repo.Version.Version))

			continue
		}

		content, err := moodle.SetPluginVersion(original, version)
		if err != nil {
			return nil, fmt.Errorf("%s: version.php: %w", repo.Path, err)
		}

		content, dependencies, err := moodle.SetDependencyVersions(content, components, version)
		if err != nil {
			return nil, fmt.Errorf("%s: version.php: %w", repo.Path, err)
		}

		plan := versionPlan{
			repo:         repo,
			path:         path,
			mode:         info.Mode().Perm(),
			from:         repo.Version.Version,
			dependencies: dependencies,
		}

		if string(content) != string(original) {
			plan.content = content
		}

		plans = append(plans, plan)
	}

	if len(downgrades) > 0 {
		return nil, fmt.Errorf(
			"version %s is lower than the current version of:\n  %s\nMoodle refuses downgrades; nothing was changed",
			version, strings.Join(downgrades, "\n  "),
		)
	}

	return plans, nil
}
