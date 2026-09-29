package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mutms/mudev/go/internal/config"
)

// pushFixture clones the standard workspace fixture and returns the workspace
// root plus the core and plugin "remotes" it was cloned from.
func pushFixture(t *testing.T) (root string, core string, plugin string) {
	t.Helper()

	recipePath, root := workspaceFixture(t, "502")

	if err := cloneInto(t, recipePath, root); err != nil {
		t.Fatalf("clone: %v", err)
	}

	base := filepath.Dir(recipePath)

	return root, filepath.Join(base, "remote-core"), filepath.Join(base, "remote-plugin")
}

func pushInto(t *testing.T, root string) (string, error) {
	t.Helper()

	var out strings.Builder

	err := Push(context.Background(), FanOptions{Config: config.Defaults(), Root: root, Out: &out})

	return out.String(), err
}

func TestPushSendsPluginsButNeverCore(t *testing.T) {
	root, core, plugin := pushFixture(t)

	mulib := filepath.Join(root, "public", "admin", "tool", "mulib")

	coreBefore := gitOut(t, core, "rev-parse", "patch/mutms/MOODLE_502_STABLE")

	runGit(t, root, "-c", "user.email=t@example.org", "-c", "user.name=t", "commit", "--quiet", "--allow-empty", "--message", "core work")
	runGit(t, mulib, "-c", "user.email=t@example.org", "-c", "user.name=t", "commit", "--quiet", "--allow-empty", "--message", "plugin work")

	out, err := pushInto(t, root)
	if err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}

	if got, want := gitOut(t, plugin, "rev-parse", "MOODLE_500_STABLE"), gitOut(t, mulib, "rev-parse", "HEAD"); got != want {
		t.Errorf("plugin remote is at %s, want the pushed %s", got, want)
	}

	if got := gitOut(t, core, "rev-parse", "patch/mutms/MOODLE_502_STABLE"); got != coreBefore {
		t.Error("core must never be pushed")
	}

	if !strings.Contains(out, "---- public/admin/tool/mulib ") {
		t.Errorf("the pushed plugin should be announced:\n%s", out)
	}

	// Everything is on origin now, so a second run has nothing to say.
	out, err = pushInto(t, root)
	if err != nil {
		t.Fatalf("second push: %v", err)
	}

	if strings.Contains(out, "----") || !strings.Contains(out, "1 plugin(s) with nothing to push") {
		t.Errorf("an up-to-date plugin should only be counted:\n%s", out)
	}
}

func TestPushLeavesAPluginOnAnotherBranchAlone(t *testing.T) {
	root, _, plugin := pushFixture(t)

	mulib := filepath.Join(root, "public", "admin", "tool", "mulib")

	remoteBefore := gitOut(t, plugin, "rev-parse", "MOODLE_500_STABLE")

	runGit(t, mulib, "switch", "--quiet", "--create", "MDL-1234-fix")
	runGit(t, mulib, "-c", "user.email=t@example.org", "-c", "user.name=t", "commit", "--quiet", "--allow-empty", "--message", "feature work")

	out, err := pushInto(t, root)
	if err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}

	if !strings.Contains(out, "on branch MDL-1234-fix, the recipe expects MOODLE_500_STABLE — not pushed") {
		t.Errorf("expected a notice about the branch:\n%s", out)
	}

	if !gitFails(t, plugin, "rev-parse", "--verify", "-q", "refs/heads/MDL-1234-fix") {
		t.Error("the feature branch must not reach the remote")
	}

	if got := gitOut(t, plugin, "rev-parse", "MOODLE_500_STABLE"); got != remoteBefore {
		t.Error("the recorded branch on the remote must not change")
	}
}
