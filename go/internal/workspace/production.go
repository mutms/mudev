package workspace

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mutms/mudev/go/internal/config"
	"github.com/mutms/mudev/go/internal/exec"
	"github.com/mutms/mudev/go/internal/git"
	"github.com/mutms/mudev/go/internal/moodle"
)

// ProductionExportOptions configure building a deployable tarball from a
// workspace.
type ProductionExportOptions struct {
	// Config is the resolved configuration.
	Config config.Config

	// Root is the workspace directory to export.
	Root string

	// Target is where the resulting .tgz is written.
	Target string

	// Out receives mudev's own progress lines.
	Out io.Writer
}

// ProductionExport assembles the workspace into a single gzipped tar at Target.
//
// It exports the committed state of every checkout the live recipe *records*,
// laid out at its real path in the tree — unmanaged repositories that happen to
// be in the working directory are left out, because a production artifact is
// exactly the recipe and nothing a developer cloned alongside it.
//
// The tree must be clean: a managed checkout with uncommitted changes (tracked,
// staged or untracked) stops the run before anything is built, so what ships is
// exactly what `mudev list` shows. When the exported tree carries the 5.1+
// public/ layout, Composer's production dependencies are installed at the root
// before the tarball is made.
func ProductionExport(ctx context.Context, opts ProductionExportOptions) error {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return err
	}

	target, err := filepath.Abs(opts.Target)
	if err != nil {
		return err
	}

	if err := checkTarballName(target); err != nil {
		return err
	}

	out := newOutput(opts.Out)

	ws, err := Enumerate(root)
	if err != nil {
		return err
	}

	client := git.New(opts.Config)

	managed, err := cleanManaged(ctx, client, ws)
	if err != nil {
		return err
	}

	// A staging directory materialises the whole tree so Composer has a place to
	// run and tar has a single root to pack. It is temporary: the artifact is the
	// only thing that outlives the command.
	staging, err := os.MkdirTemp("", "mudev-export-*")
	if err != nil {
		return err
	}

	defer func() {
		_ = os.RemoveAll(staging)
	}()

	out.printf("exporting %d checkout(s) from %s", len(managed), root)

	if err := exportTree(ctx, client, root, staging, managed, out); err != nil {
		return err
	}

	if needsComposer(staging) {
		// Resolve the project's PHP from the workspace root, not the /tmp staging
		// dir: on the mpd runtime the `php` dispatcher only reads a project's
		// version when its cwd is under /srv/projects/<name>, which is where mudev
		// was invoked. That resolved version is then forced onto composer so it
		// installs under the same PHP `mudev list` would run, not the runtime's
		// fallback.
		phpVersion, err := detectPHP(ctx, root)
		if err != nil {
			return err
		}

		out.printf("public/ layout — composer install (php %s)", phpVersion)

		if err := composerInstall(ctx, staging, phpVersion); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	out.printf("writing %s", target)

	if err := tarGzip(ctx, staging, target); err != nil {
		return err
	}

	return nil
}

// cleanManaged returns the recipe-recorded checkouts, having verified that each
// one is present and clean. It is a pre-flight: the whole tree is checked before
// any of it is exported, so a dirty checkout stops the run before work begins
// rather than halfway through.
func cleanManaged(ctx context.Context, client *git.Client, ws *Workspace) ([]Repo, error) {
	var managed []Repo
	var dirty []string
	var missing []string

	for _, repo := range ws.Repos {
		if !repo.Managed {
			continue
		}

		if repo.Missing {
			missing = append(missing, repo.Path)

			continue
		}

		status, err := client.Status(ctx, filepath.Join(ws.Root, repo.Path))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", repo.Path, err)
		}

		if status.Dirty {
			dirty = append(dirty, repo.Path)
		}

		managed = append(managed, repo)
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf(
			"cannot export — recorded checkout(s) missing from the tree: %s",
			strings.Join(missing, ", "),
		)
	}

	if len(dirty) > 0 {
		return nil, fmt.Errorf(
			"cannot export — uncommitted changes in: %s (commit or discard them first)",
			strings.Join(dirty, ", "),
		)
	}

	if len(managed) == 0 {
		return nil, fmt.Errorf(
			"nothing to export — no checkouts recorded in %s under %s",
			config.LiveRecipeFile, ws.Root,
		)
	}

	return managed, nil
}

// exportTree materialises every managed checkout's committed HEAD into staging
// at its recorded path, so the staging directory becomes the assembled tree.
func exportTree(ctx context.Context, client *git.Client, root string, staging string, managed []Repo, out output) error {
	tarball := filepath.Join(staging, ".mudev-archive.tar")

	// One archive file, reused for every checkout — it is extracted and gone
	// before the next one overwrites it, and must not land in the artifact.
	defer func() {
		_ = os.Remove(tarball)
	}()

	for _, repo := range managed {
		src := filepath.Join(root, repo.Path)
		dest := filepath.Join(staging, filepath.FromSlash(repo.Path))

		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}

		if err := client.Archive(ctx, src, "HEAD", tarball); err != nil {
			return fmt.Errorf("%s: %w", repo.Path, err)
		}

		if err := extractTar(ctx, tarball, dest); err != nil {
			return fmt.Errorf("%s: %w", repo.Path, err)
		}

		out.stepf("%s", repo.Path)
	}

	return nil
}

// needsComposer reports whether the assembled tree uses Moodle's 5.1+ public/
// layout, which is the tree that carries a root composer.json to install. The
// presence of public/version.php is the tell, exactly as the layout is detected
// everywhere else.
func needsComposer(root string) bool {
	_, err := os.Stat(filepath.Join(root, moodle.PublicPrefix, "version.php"))

	return err == nil
}

// phpVersionPattern pulls the major.minor version out of `php -v`'s banner
// ("PHP 8.4.9 (cli) …") — the form the mpd php dispatcher wants for
// MPD_PHP_FORCE_VERSION and the versioned /usr/bin/phpX.Y it execs.
var phpVersionPattern = regexp.MustCompile(`PHP (\d+\.\d+)`)

// detectPHP reports the major.minor PHP version that runs in dir.
//
// It is read from dir — the workspace root — on purpose: the mpd runtime's php
// dispatcher resolves a project's configured version only when its cwd is under
// /srv/projects/<name>, so asking there gives the version the project actually
// uses, layered defaults and all, without mudev knowing anything about mpd.env.
// A php that will not run is fatal here rather than later: composer cannot
// install without it.
func detectPHP(ctx context.Context, dir string) (string, error) {
	if !exec.Available("php") {
		return "", fmt.Errorf("php was not found on PATH — needed to run composer for a public/ layout tree")
	}

	res, err := exec.Capture(ctx, exec.Cmd{Name: "php", Args: []string{"-v"}, Dir: dir})
	if err != nil {
		return "", err
	}

	if err := res.Err(); err != nil {
		return "", fmt.Errorf("php -v: %w", err)
	}

	match := phpVersionPattern.FindStringSubmatch(res.Stdout)
	if match == nil {
		return "", fmt.Errorf("could not read a PHP version from `php -v`: %q", res.Stdout)
	}

	return match[1], nil
}

// composerInstall runs Composer's production install at the tree root.
//
// It forces the PHP version onto composer via MPD_PHP_FORCE_VERSION: composer
// runs in a /tmp staging dir, where mpd's php dispatcher would otherwise fall
// back to a pinned default rather than the project's own version. The variable
// is mpd's, harmless anywhere else — a plain composer ignores it.
func composerInstall(ctx context.Context, dir string, phpVersion string) error {
	if !exec.Available("composer") {
		return fmt.Errorf("composer was not found on PATH — needed for a public/ layout tree")
	}

	code, err := exec.Run(ctx, exec.Cmd{
		Name: "composer",
		Args: []string{"install", "--no-dev", "--classmap-authoritative"},
		Dir:  dir,
		Env:  []string{"MPD_PHP_FORCE_VERSION=" + phpVersion},
	})
	if err != nil {
		return err
	}

	if code != 0 {
		return fmt.Errorf("composer install: exit status %d", code)
	}

	return nil
}

// extractTar unpacks a tar file into dir, through the real tar so file modes and
// symlinks survive into the artifact exactly as git recorded them.
func extractTar(ctx context.Context, tarball string, dir string) error {
	if !exec.Available("tar") {
		return fmt.Errorf("tar was not found on PATH")
	}

	res, err := exec.Capture(ctx, exec.Cmd{
		Name: "tar",
		Args: []string{"-xf", tarball, "-C", dir},
	})
	if err != nil {
		return err
	}

	return res.Err()
}

// tarGzip packs the contents of dir into a gzipped tar at target.
func tarGzip(ctx context.Context, dir string, target string) error {
	if !exec.Available("tar") {
		return fmt.Errorf("tar was not found on PATH")
	}

	res, err := exec.Capture(ctx, exec.Cmd{
		Name: "tar",
		Args: []string{"-czf", target, "-C", dir, "."},
	})
	if err != nil {
		return err
	}

	return res.Err()
}

// checkTarballName rejects a target that is not a gzipped tar. The command
// produces exactly that, so a name promising something else is a mistake worth
// catching before any work is done.
func checkTarballName(target string) error {
	lower := strings.ToLower(target)

	if strings.HasSuffix(lower, ".tgz") || strings.HasSuffix(lower, ".tar.gz") {
		return nil
	}

	return fmt.Errorf("%s: the export must be named .tgz or .tar.gz", target)
}
