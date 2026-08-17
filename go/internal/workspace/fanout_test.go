package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHeader(t *testing.T) {
	got := header("public/admin/tool/mulib")

	if !strings.HasPrefix(got, "---- public/admin/tool/mulib -") {
		t.Errorf("header = %q", got)
	}

	if len(got) != headerWidth {
		t.Errorf("header is %d wide, want %d: %q", len(got), headerWidth, got)
	}

	// A path longer than the separator is not truncated — the name matters
	// more than the alignment.
	long := header(strings.Repeat("x", headerWidth+10))

	if len(long) <= headerWidth {
		t.Errorf("a long path should not be cut: %q", long)
	}
}

func TestFanOutVisitsEveryCheckoutAndStopsOnError(t *testing.T) {
	root := t.TempDir()

	mkGitDir(t, root, ".")
	mkGitDir(t, root, "public/admin/tool/certificate")
	mkGitDir(t, root, "public/admin/tool/mulib")

	var (
		visited []string
		out     strings.Builder
	)

	opts := FanOptions{Root: root, Out: &out}

	err := fanOut(context.Background(), opts, func(dir string, repo Repo) (string, error) {
		visited = append(visited, repo.Path)

		return "", nil
	})
	if err != nil {
		t.Fatalf("fanOut: %v", err)
	}

	// Core first, then by path — the same order a listing shows.
	want := []string{CoreDir, "public/admin/tool/certificate", "public/admin/tool/mulib"}

	if strings.Join(visited, ",") != strings.Join(want, ",") {
		t.Errorf("visited %v, want %v", visited, want)
	}

	// Each checkout is announced, so git's own output underneath is
	// attributable to it.
	for _, path := range want {
		if !strings.Contains(out.String(), "---- "+path+" ") {
			t.Errorf("no header for %s:\n%s", path, out.String())
		}
	}

	// A failure stops the run there: whatever came after is left untouched,
	// which is what makes "fix that one repository and run it again" work.
	visited = nil

	err = fanOut(context.Background(), opts, func(dir string, repo Repo) (string, error) {
		visited = append(visited, repo.Path)

		if repo.Path == "public/admin/tool/certificate" {
			return "", os.ErrPermission
		}

		return "", nil
	})
	if err == nil {
		t.Fatal("expected the error to stop the run")
	}

	if !strings.Contains(err.Error(), "public/admin/tool/certificate") {
		t.Errorf("the error should name the checkout: %v", err)
	}

	if len(visited) != 2 {
		t.Errorf("visited %v, want to stop at the failing checkout", visited)
	}
}

func TestFanOutSkipsMissingCheckouts(t *testing.T) {
	root := t.TempDir()

	// A live recipe recording a plugin that is not on disk (an interrupted
	// clone): there is no repository to run git in.
	if err := os.WriteFile(LivePath(root), []byte(liveRecipe), 0o644); err != nil {
		t.Fatal(err)
	}

	mkGitDir(t, root, ".")
	mkGitDir(t, root, "public/admin/tool/mulib")

	var (
		visited []string
		out     strings.Builder
	)

	err := fanOut(context.Background(), FanOptions{Root: root, Out: &out}, func(dir string, repo Repo) (string, error) {
		visited = append(visited, repo.Path)

		if _, statErr := os.Stat(filepath.Join(dir, ".git")); statErr != nil {
			t.Errorf("%s has no repository", repo.Path)
		}

		return "", nil
	})
	if err != nil {
		t.Fatalf("fanOut: %v", err)
	}

	for _, path := range visited {
		if path == "public/admin/tool/mutenancy" {
			t.Error("a checkout that is not on disk should be skipped")
		}
	}
}

func TestFanOutReportsAnEmptyWorkspace(t *testing.T) {
	var out strings.Builder

	err := fanOut(context.Background(), FanOptions{Root: t.TempDir(), Out: &out}, func(string, Repo) (string, error) {
		t.Error("nothing should have been visited")

		return "", nil
	})
	if err != nil {
		t.Fatalf("fanOut: %v", err)
	}

	// Silence would look the same as a command that quietly did nothing.
	if !strings.Contains(out.String(), "no git checkouts found") {
		t.Errorf("expected a note, got %q", out.String())
	}
}

func TestFanOutFilteredSkipsAndCounts(t *testing.T) {
	root := t.TempDir()

	mkGitDir(t, root, ".")
	mkGitDir(t, root, "public/admin/tool/a")
	mkGitDir(t, root, "public/admin/tool/b")

	var (
		visited []string
		out     strings.Builder
	)

	err := fanOutFiltered(context.Background(), FanOptions{Root: root, Out: &out},
		func(ws *Workspace, dir string, repo Repo) (bool, error) {
			// Only one checkout has anything to say.
			return repo.Path == "public/admin/tool/b", nil
		},
		func(ws *Workspace, dir string, repo Repo) (string, error) {
			visited = append(visited, repo.Path)

			return "", nil
		},
	)
	if err != nil {
		t.Fatalf("fanOutFiltered: %v", err)
	}

	if len(visited) != 1 || visited[0] != "public/admin/tool/b" {
		t.Errorf("visited %v, want only the admitted checkout", visited)
	}

	// A skipped checkout gets no header — that is the whole point, twenty
	// repetitions of "nothing to commit" would bury the one that matters.
	if strings.Contains(out.String(), "---- public/admin/tool/a") {
		t.Errorf("a skipped checkout should print nothing:\n%s", out.String())
	}

	// But silence about a repository is not the same as there being nothing to
	// say about it, so the count has to be reported.
	if !strings.Contains(out.String(), "2 checkout(s) with nothing to report") {
		t.Errorf("skipped checkouts were not counted:\n%s", out.String())
	}
}

func TestFanOutFilteredReportsFilterErrors(t *testing.T) {
	root := t.TempDir()
	mkGitDir(t, root, "public/admin/tool/a")

	var out strings.Builder

	err := fanOutFiltered(context.Background(), FanOptions{Root: root, Out: &out},
		func(ws *Workspace, dir string, repo Repo) (bool, error) {
			return false, os.ErrPermission
		},
		func(ws *Workspace, dir string, repo Repo) (string, error) {
			t.Error("the action must not run when the filter failed")

			return "", nil
		},
	)
	if err == nil {
		t.Fatal("a filter error must stop the run")
	}

	if !strings.Contains(err.Error(), "public/admin/tool/a") {
		t.Errorf("the error should name the checkout: %v", err)
	}
}
