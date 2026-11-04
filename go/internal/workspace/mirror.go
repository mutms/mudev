package workspace

import (
	"context"
	"os"
	"path/filepath"

	"github.com/mutms/mudev/go/internal/git"
)

// The local Moodle mirror (/srv/extra/moodle.git) is a bare repository that every
// new Moodle core checkout borrows objects from through .git/objects/info/alternates,
// the link `git clone --reference` creates. A new workspace then downloads and
// stores only what upstream Moodle does not have, instead of 1 GB of history each.
//
// Workspaces keep depending on the mirror, so the mirror must never lose objects:
// it is refreshed without --prune and gc.pruneExpire is "never". It is only ever
// deleted together with every workspace that uses it.
//
// Nothing about the mirror is required. Whenever it cannot be created, refreshed
// or used, the checkout is assembled the plain way.

// mirrorFetchRefspec keeps upstream branches as branches of the bare mirror;
// --mirror would also bring GitHub's refs/pull/*.
const mirrorFetchRefspec = "+refs/heads/*:refs/heads/*"

// prepareMirror makes the mirror ready and returns its path, or empty string when
// there is no mirror to use.
//
//   - disabled (MUDEV_MOODLE_MIRROR=0): nothing;
//   - existing and writable: refreshed, then used;
//   - existing and read-only, for example baked into an image: used as it is;
//   - missing: created when its directory is writable, otherwise nothing.
func (c *cloner) prepareMirror(ctx context.Context) string {
	path := c.cfg.MoodleMirror
	if path == "" {
		return ""
	}

	if isBareRepo(path) {
		if writable(path) {
			c.refreshMirror(ctx, path)
		}

		return path
	}

	if _, err := os.Lstat(path); err == nil {
		c.warnf("%s is not a git repository, ignoring it", path)

		return ""
	}

	if !writable(filepath.Dir(path)) {
		return ""
	}

	return c.createMirror(ctx, path)
}

// createMirror clones the upstream into a temporary directory next to the mirror
// and renames it into place, so an interrupted or concurrent run never leaves
// half a mirror behind.
func (c *cloner) createMirror(ctx context.Context, path string) string {
	c.stepf("mirror: creating %s from %s", path, c.cfg.MoodleUpstream)

	tmp, err := os.MkdirTemp(filepath.Dir(path), filepath.Base(path)+".tmp-")
	if err != nil {
		c.warnf("could not create mirror %s (%v), continuing without it", path, err)

		return ""
	}

	if err := c.client.CloneBare(ctx, c.cfg.MoodleUpstream, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		c.warnf("could not create mirror %s (%v), continuing without it", path, err)

		return ""
	}

	if err := c.configureMirror(ctx, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		c.warnf("could not configure mirror %s (%v), continuing without it", path, err)

		return ""
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.RemoveAll(tmp)

		// Another run may have created it in the meantime.
		if isBareRepo(path) {
			return path
		}

		c.warnf("could not create mirror %s (%v), continuing without it", path, err)

		return ""
	}

	return path
}

// refreshMirror fetches new upstream branches and tags, never pruning anything.
// It also completes the configuration of mirrors created by hand with
// `git clone --bare`. Failures, such as being offline, are only reported.
func (c *cloner) refreshMirror(ctx context.Context, path string) {
	if err := c.configureMirror(ctx, path); err != nil {
		c.warnf("could not configure mirror %s: %v", path, err)

		return
	}

	c.stepf("mirror: refreshing %s", path)

	if err := c.client.Fetch(ctx, path, OriginRemote); err != nil {
		c.warnf("could not refresh mirror %s (%v), using it as it is", path, err)
	}
}

// configureMirror makes sure the mirror fetches upstream branches into its own
// branches and that garbage collection never deletes objects.
func (c *cloner) configureMirror(ctx context.Context, path string) error {
	if c.client.GetConfig(ctx, path, "remote.origin.fetch") == "" {
		if err := c.client.SetConfig(ctx, path, "remote.origin.fetch", mirrorFetchRefspec); err != nil {
			return err
		}
	}

	if c.client.GetConfig(ctx, path, "gc.pruneExpire") != "never" {
		if err := c.client.SetConfig(ctx, path, "gc.pruneExpire", "never"); err != nil {
			return err
		}
	}

	return nil
}

// useMirror links a new core checkout to the mirror before anything is fetched.
func (c *cloner) useMirror(ctx context.Context, dir string) {
	mirror := c.prepareMirror(ctx)
	if mirror == "" {
		return
	}

	if err := git.SetAlternates(dir, filepath.Join(mirror, "objects")); err != nil {
		c.warnf("could not link mirror %s (%v), continuing without it", mirror, err)

		return
	}

	c.stepf("using mirror %s", mirror)
}

// isBareRepo reports whether path looks like a bare git repository.
func isBareRepo(path string) bool {
	if info, err := os.Stat(filepath.Join(path, "objects")); err != nil || !info.IsDir() {
		return false
	}

	_, err := os.Stat(filepath.Join(path, "HEAD"))

	return err == nil
}

// writable reports whether files can be created in dir, which is more reliable
// than reading permission bits (read-only mounts, ACLs, other owners).
func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".mudev-write-test-*")
	if err != nil {
		return false
	}

	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)

	return true
}
