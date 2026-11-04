package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutms/mudev/go/internal/config"
)

// mirrorFixture builds a fake Moodle upstream, a recipe whose core comes from it
// and an "extra" directory standing in for /srv/extra. It returns the recipe path,
// the configuration pointing the mirror into extra, and the upstream path.
func mirrorFixture(t *testing.T) (recipePath string, cfg config.Config, upstream string) {
	t.Helper()

	base := t.TempDir()

	upstream = filepath.Join(base, "remote-core")
	fakeCore(t, upstream, "502")

	extra := filepath.Join(base, "extra")
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatal(err)
	}

	recipePath = filepath.Join(base, "recipe.yaml")
	recipe := `name: mirror test
base:
  mdlbranch: "502"
  source:
    git:
      remotes:
        origin: ` + upstream + `
      ref: origin/patch/mutms/MOODLE_502_STABLE
  localbranch: MOODLE_502_STABLE
plugins: []
`
	if err := os.WriteFile(recipePath, []byte(recipe), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg = config.Defaults()
	cfg.MoodleMirror = filepath.Join(extra, "moodle.git")
	cfg.MoodleUpstream = upstream

	return recipePath, cfg, upstream
}

// cloneWith assembles the recipe into a new workspace and returns its root and output.
func cloneWith(t *testing.T, recipePath string, cfg config.Config) (root string, output string) {
	t.Helper()

	root = filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder

	if err := Clone(context.Background(), Options{Config: cfg, Recipe: recipePath, Root: root, Out: &out}); err != nil {
		t.Fatalf("Clone: %v\n%s", err, out.String())
	}

	if _, err := os.Stat(filepath.Join(root, "public", "version.php")); err != nil {
		t.Fatalf("core was not checked out: %v", err)
	}

	runGit(t, root, "fsck", "--no-progress")

	return root, out.String()
}

// alternates returns the object stores a checkout borrows from, empty when none.
func alternates(t *testing.T, root string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, ".git", "objects", "info", "alternates"))
	if os.IsNotExist(err) {
		return ""
	}

	if err != nil {
		t.Fatal(err)
	}

	return strings.TrimSpace(string(data))
}

// skipIfRoot skips permission tests, root can write anywhere.
func skipIfRoot(t *testing.T) {
	t.Helper()

	if os.Geteuid() == 0 {
		t.Skip("permissions do not apply to root")
	}
}

func TestCloneCreatesAndUsesMirror(t *testing.T) {
	recipePath, cfg, _ := mirrorFixture(t)

	root, out := cloneWith(t, recipePath, cfg)

	if !isBareRepo(cfg.MoodleMirror) {
		t.Fatalf("mirror %s was not created", cfg.MoodleMirror)
	}

	if !strings.Contains(out, "mirror: creating "+cfg.MoodleMirror) || !strings.Contains(out, "using mirror "+cfg.MoodleMirror) {
		t.Errorf("mirror use not reported:\n%s", out)
	}

	if got := alternates(t, root); got != filepath.Join(cfg.MoodleMirror, "objects") {
		t.Errorf("alternates = %q", got)
	}

	if got := strings.TrimSpace(gitOut(t, cfg.MoodleMirror, "config", "gc.pruneExpire")); got != "never" {
		t.Errorf("gc.pruneExpire = %q, want never", got)
	}

	if got := strings.TrimSpace(gitOut(t, cfg.MoodleMirror, "config", "remote.origin.fetch")); got != mirrorFetchRefspec {
		t.Errorf("remote.origin.fetch = %q", got)
	}

	// No leftovers of the temporary clone next to the mirror.
	entries, err := os.ReadDir(filepath.Dir(cfg.MoodleMirror))
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Errorf("unexpected files next to the mirror: %v", entries)
	}

	// The next workspace refreshes and reuses the mirror.
	root2, out2 := cloneWith(t, recipePath, cfg)

	if !strings.Contains(out2, "mirror: refreshing "+cfg.MoodleMirror) || strings.Contains(out2, "mirror: creating") {
		t.Errorf("second clone did not refresh the existing mirror:\n%s", out2)
	}

	if alternates(t, root2) == "" {
		t.Error("second workspace is not linked to the mirror")
	}
}

func TestMirrorRefreshNeverPrunes(t *testing.T) {
	recipePath, cfg, upstream := mirrorFixture(t)

	runGit(t, upstream, "branch", "short-lived")
	cloneWith(t, recipePath, cfg)

	runGit(t, upstream, "branch", "-D", "short-lived")
	runGit(t, upstream, "branch", "new-branch")
	cloneWith(t, recipePath, cfg)

	refs := gitOut(t, cfg.MoodleMirror, "for-each-ref", "--format=%(refname)", "refs/heads")
	for _, want := range []string{"refs/heads/short-lived", "refs/heads/new-branch"} {
		if !strings.Contains(refs, want) {
			t.Errorf("mirror is missing %s:\n%s", want, refs)
		}
	}
}

func TestMirrorCreatedByHandIsCompleted(t *testing.T) {
	recipePath, cfg, upstream := mirrorFixture(t)

	// The way setup scripts create it from a LAN mirror.
	runGit(t, filepath.Dir(cfg.MoodleMirror), "clone", "--quiet", "--bare", upstream, cfg.MoodleMirror)

	root, out := cloneWith(t, recipePath, cfg)

	if !strings.Contains(out, "mirror: refreshing") || alternates(t, root) == "" {
		t.Errorf("hand-made mirror was not used:\n%s", out)
	}

	if got := strings.TrimSpace(gitOut(t, cfg.MoodleMirror, "config", "gc.pruneExpire")); got != "never" {
		t.Errorf("gc.pruneExpire = %q, want never", got)
	}
}

func TestCloneUsesReadOnlyMirror(t *testing.T) {
	skipIfRoot(t)

	recipePath, cfg, upstream := mirrorFixture(t)
	extra := filepath.Dir(cfg.MoodleMirror)

	runGit(t, extra, "clone", "--quiet", "--bare", upstream, cfg.MoodleMirror)

	if err := os.Chmod(cfg.MoodleMirror, 0o555); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(cfg.MoodleMirror, 0o755) })

	root, out := cloneWith(t, recipePath, cfg)

	if strings.Contains(out, "mirror: refreshing") {
		t.Errorf("read-only mirror must not be refreshed:\n%s", out)
	}

	if got := alternates(t, root); got != filepath.Join(cfg.MoodleMirror, "objects") {
		t.Errorf("alternates = %q", got)
	}
}

func TestCloneWithoutWritableMirrorDirectory(t *testing.T) {
	skipIfRoot(t)

	recipePath, cfg, _ := mirrorFixture(t)
	extra := filepath.Dir(cfg.MoodleMirror)

	if err := os.Chmod(extra, 0o555); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(extra, 0o755) })

	root, out := cloneWith(t, recipePath, cfg)

	if _, err := os.Stat(cfg.MoodleMirror); !os.IsNotExist(err) {
		t.Errorf("mirror was created in a read-only directory")
	}

	if alternates(t, root) != "" || strings.Contains(out, "mirror:") || strings.Contains(out, "using mirror") {
		t.Errorf("plain clone expected:\n%s", out)
	}
}

func TestCloneWithoutMirrorDirectory(t *testing.T) {
	recipePath, cfg, _ := mirrorFixture(t)
	cfg.MoodleMirror = filepath.Join(t.TempDir(), "missing", "moodle.git")

	root, out := cloneWith(t, recipePath, cfg)

	if alternates(t, root) != "" || strings.Contains(out, "mirror:") || strings.Contains(out, "using mirror") {
		t.Errorf("plain clone expected:\n%s", out)
	}
}

func TestCloneWithMirrorDisabled(t *testing.T) {
	recipePath, cfg, _ := mirrorFixture(t)
	mirror := cfg.MoodleMirror

	t.Setenv(config.EnvMoodleMirror, "0")

	if err := config.ApplyEnv(&cfg); err != nil {
		t.Fatal(err)
	}

	root, out := cloneWith(t, recipePath, cfg)

	if _, err := os.Stat(mirror); !os.IsNotExist(err) {
		t.Errorf("disabled mirror was created")
	}

	if alternates(t, root) != "" || strings.Contains(out, "mirror:") || strings.Contains(out, "using mirror") {
		t.Errorf("plain clone expected:\n%s", out)
	}
}

func TestCloneSurvivesFailedMirrorCreation(t *testing.T) {
	recipePath, cfg, _ := mirrorFixture(t)
	cfg.MoodleUpstream = filepath.Join(t.TempDir(), "nowhere")

	root, out := cloneWith(t, recipePath, cfg)

	if !strings.Contains(out, "could not create mirror") {
		t.Errorf("failed mirror creation not reported:\n%s", out)
	}

	entries, err := os.ReadDir(filepath.Dir(cfg.MoodleMirror))
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 0 {
		t.Errorf("half a mirror left behind: %v", entries)
	}

	if alternates(t, root) != "" {
		t.Error("workspace linked to a mirror that does not exist")
	}
}
