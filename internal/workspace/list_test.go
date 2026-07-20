package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// mkGitDir fakes a checkout: the walk only looks for a .git entry.
func mkGitDir(t *testing.T, root string, path string) {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(path), ".git")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()

	mkGitDir(t, root, ".")
	mkGitDir(t, root, "public/admin/tool/certificate")
	// A subplugin lives inside another checkout, so the walk must keep
	// descending after it finds one.
	mkGitDir(t, root, "public/admin/tool/certificate/element/muprog")
	// Third-party trees are not part of the workspace composition.
	mkGitDir(t, root, "public/node_modules/thing")
	mkGitDir(t, root, "vendor/library")

	found, err := discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}

	want := []string{
		CoreDir,
		"public/admin/tool/certificate",
		"public/admin/tool/certificate/element/muprog",
	}

	sorted := append([]string(nil), found...)

	if !reflect.DeepEqual(sorted, want) {
		t.Errorf("discover = %v, want %v", sorted, want)
	}
}

func TestDiscoverHandlesGitFile(t *testing.T) {
	root := t.TempDir()

	// A submodule or linked worktree has a .git *file*, not a directory.
	dir := filepath.Join(root, "public", "mod", "thing")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../../.git/modules/thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	found, err := discover(root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}

	if !reflect.DeepEqual(found, []string{"public/mod/thing"}) {
		t.Errorf("discover = %v", found)
	}
}

const liveRecipe = `{
  "name": "test",
  "based_on_recipe": "mutms/dev/5.2",
  "moodle": {
    "mdlbranch": "502",
    "localbranch": "MOODLE_502_STABLE",
    "source": {
      "git": {
        "remotes": {"origin": "https://github.com/mutms/patches.git"},
        "ref": "origin/patch/mutms/MOODLE_502_STABLE"
      }
    }
  },
  "plugins": [
    {
      "name": "mutms/tool_mulib",
      "title": "MuTMS shared library",
      "relpath": "public/admin/tool/mulib",
      "requirements": {"MOODLE_500_STABLE": {"mdlbranches": ["502"]}},
      "source": {
        "git": {
          "remotes": {"origin": "https://github.com/mutms/moodle-tool_mulib.git"},
          "ref": "origin/MOODLE_500_STABLE"
        }
      }
    },
    {
      "name": "mutms/tool_mutenancy",
      "title": "Tenancy",
      "relpath": "public/admin/tool/mutenancy",
      "requirements": {"MOODLE_502_STABLE": {"mdlbranches": ["502"]}},
      "source": {
        "git": {
          "remotes": {"origin": "https://github.com/mutms/moodle-tool_mutenancy.git"},
          "ref": "v5.2.1.01"
        }
      }
    }
  ]
}
`

func TestRecordedRepos(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(LivePath(root), []byte(liveRecipe), 0o644); err != nil {
		t.Fatal(err)
	}

	recorded, _, err := recordedRepos(root)
	if err != nil {
		t.Fatalf("recordedRepos: %v", err)
	}

	core, ok := recorded[CoreDir]
	if !ok {
		t.Fatalf("core not recorded: %v", recorded)
	}

	// localbranch overrides the remote branch name — the patched core is
	// cloned from patch/mutms/… but lives locally as MOODLE_502_STABLE.
	if core.RecordedBranch != "MOODLE_502_STABLE" || !core.Managed {
		t.Errorf("core = %+v", core)
	}

	// Rows are keyed by the path in the tree, public/ prefix included.
	mulib, ok := recorded["public/admin/tool/mulib"]
	if !ok {
		t.Fatalf("plugin path not recorded: %v", recorded)
	}

	if mulib.Name != "mutms/tool_mulib" || mulib.RecordedBranch != "MOODLE_500_STABLE" {
		t.Errorf("mulib = %+v", mulib)
	}

	// A tag pins the checkout, so there is no branch it could stray from.
	tenancy := recorded["public/admin/tool/mutenancy"]

	if tenancy.RecordedRef != "v5.2.1.01" || tenancy.RecordedBranch != "" {
		t.Errorf("mutenancy = %+v", tenancy)
	}
}

func TestRecordedReposStripsPublicForOlderMoodle(t *testing.T) {
	root := t.TempDir()

	// Pre-5.1 trees have no public/ directory.
	stripped := `{"moodle":{"mdlbranch":"405","strippublic":true,"source":{"git":{"remotes":{"origin":"x"},"ref":"v4.5.12"}}},` +
		`"plugins":[{"name":"mutms/tool_mulib","title":"x","relpath":"public/admin/tool/mulib",` +
		`"requirements":{"MOODLE_405_STABLE":{"mdlbranches":["405"]}},` +
		`"source":{"git":{"remotes":{"origin":"y"},"ref":"origin/MOODLE_405_STABLE"}}}]}`

	if err := os.WriteFile(LivePath(root), []byte(stripped), 0o644); err != nil {
		t.Fatal(err)
	}

	recorded, _, err := recordedRepos(root)
	if err != nil {
		t.Fatalf("recordedRepos: %v", err)
	}

	if _, ok := recorded["admin/tool/mulib"]; !ok {
		t.Errorf("expected a stripped path, got %v", keysOf(recorded))
	}
}

func TestRecordedReposWithoutLiveRecipe(t *testing.T) {
	// A directory that is not a mudev workspace still lists its checkouts;
	// they are simply all unmanaged.
	recorded, _, err := recordedRepos(t.TempDir())
	if err != nil {
		t.Fatalf("recordedRepos: %v", err)
	}

	if len(recorded) != 0 {
		t.Errorf("expected nothing recorded, got %v", keysOf(recorded))
	}
}

func keysOf(m map[string]*Repo) []string {
	keys := make([]string, 0, len(m))

	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
