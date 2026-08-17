package moodle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginPath(t *testing.T) {
	const relpath = "public/admin/tool/mulib"

	got, err := PluginPath(relpath, false)
	if err != nil {
		t.Fatalf("PluginPath: %v", err)
	}

	if got != relpath {
		t.Errorf("PluginPath = %q, want the stored layout", got)
	}

	// Older branches are served by stripping, never by prepending.
	got, err = PluginPath(relpath, true)
	if err != nil {
		t.Fatalf("PluginPath: %v", err)
	}

	if want := "admin/tool/mulib"; got != want {
		t.Errorf("PluginPath = %q, want %q", got, want)
	}
}

func TestPluginPathRejectsEscapes(t *testing.T) {
	if _, err := PluginPath("../outside", false); err == nil {
		t.Error("expected an error for a path escaping the code root")
	}

	if _, err := PluginPath("", false); err == nil {
		t.Error("expected an error for an empty relpath")
	}
}

const coreVersionPHP = "<?php\n$version = 2025111700.00;\n$release = '5.2.1';\n$branch = '502';\n"

func TestBranch(t *testing.T) {
	dir := t.TempDir()

	// 5.1 and later keep the code root, and so version.php, under public/.
	public := filepath.Join(dir, "public")

	if err := os.MkdirAll(public, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(public, "version.php"), []byte(coreVersionPHP), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Branch(dir, false)
	if err != nil {
		t.Fatalf("Branch: %v", err)
	}

	if got != "502" {
		t.Errorf("Branch = %q, want 502", got)
	}

	// The recipe's layout decides which file is read — mudev does not go
	// looking elsewhere when the stated one is absent.
	if _, err := Branch(dir, true); err == nil {
		t.Error("a stripped-layout recipe must read the root version.php only")
	}

	if _, err := Branch(t.TempDir(), false); err == nil {
		t.Error("expected an error when there is no version.php")
	}
}

func TestBranchStrippedLayout(t *testing.T) {
	// Pre-5.1 trees keep version.php at the root.
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte(coreVersionPHP), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Branch(dir, true)
	if err != nil {
		t.Fatalf("Branch: %v", err)
	}

	if got != "502" {
		t.Errorf("Branch = %q", got)
	}
}

func TestBranchRejectsANonMoodleTree(t *testing.T) {
	dir := t.TempDir()

	// A file that exists but declares no $branch — a plugin's version.php, say.
	if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte("<?php\n$plugin->version = 2026060550;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Branch(dir, true); err == nil {
		t.Error("expected an error when version.php declares no $branch")
	}
}

func TestCoreVersionFile(t *testing.T) {
	if got := CoreVersionFile(false); got != "public/version.php" {
		t.Errorf("CoreVersionFile(false) = %q", got)
	}

	if got := CoreVersionFile(true); got != "version.php" {
		t.Errorf("CoreVersionFile(true) = %q", got)
	}
}
