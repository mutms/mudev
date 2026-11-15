package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitSetWritesEveryRepository(t *testing.T) {
	root := t.TempDir()

	repos := []string{".", "public/admin/tool/mulib", "public/admin/tool/mulib/subplugin", "handmade"}
	for _, path := range repos {
		initRepo(t, filepath.Join(root, filepath.FromSlash(path)), "main")
	}

	skipped := filepath.Join(root, "vendor", "lib")
	initRepo(t, skipped, "main")

	var out strings.Builder

	err := GitSet(context.Background(), GitSetOptions{
		Root:  root,
		Key:   "user.email",
		Value: "project@example.org",
		Out:   &out,
	})
	if err != nil {
		t.Fatalf("GitSet: %v", err)
	}

	for _, path := range repos {
		dir := filepath.Join(root, filepath.FromSlash(path))

		got := strings.TrimSpace(gitOut(t, dir, "config", "--local", "--get", "user.email"))
		if got != "project@example.org" {
			t.Errorf("%s: user.email = %q", path, got)
		}

		if !strings.Contains(out.String(), path+": user.email set\n") {
			t.Errorf("no line for %s:\n%s", path, out.String())
		}
	}

	got := strings.TrimSpace(gitOut(t, skipped, "config", "--local", "--get", "user.email"))
	if got != "test@example.org" {
		t.Errorf("vendor repository was changed: user.email = %q", got)
	}
}

func TestGitSetFromSubdirectoryLeavesParentAlone(t *testing.T) {
	root := t.TempDir()

	plugin := filepath.Join(root, "public", "local", "thing")

	initRepo(t, root, "main")
	initRepo(t, plugin, "main")

	err := GitSet(context.Background(), GitSetOptions{Root: plugin, Key: "user.name", Value: "Someone Else"})
	if err != nil {
		t.Fatalf("GitSet: %v", err)
	}

	if got := strings.TrimSpace(gitOut(t, plugin, "config", "--local", "--get", "user.name")); got != "Someone Else" {
		t.Errorf("plugin user.name = %q", got)
	}

	if got := strings.TrimSpace(gitOut(t, root, "config", "--local", "--get", "user.name")); got != "mudev test" {
		t.Errorf("parent user.name = %q", got)
	}
}

func TestGitSetErrors(t *testing.T) {
	empty := t.TempDir()

	err := GitSet(context.Background(), GitSetOptions{Root: empty, Key: "user.name", Value: "x"})
	if err == nil || !strings.Contains(err.Error(), "no git repositories") {
		t.Errorf("empty directory: err = %v", err)
	}

	root := t.TempDir()

	initRepo(t, root, "main")
	initRepo(t, filepath.Join(root, "plugin"), "main")

	err = GitSet(context.Background(), GitSetOptions{Root: root, Key: "nodot", Value: "x"})
	if err == nil {
		t.Fatal("an invalid key should fail")
	}

	// Every repository is tried, so each one is named in the error.
	for _, path := range []string{".: ", "plugin: "} {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("error does not name %q: %v", path, err)
		}
	}
}
