package moodle

import (
	"os"
	"path/filepath"
	"testing"
)

func writeVersion(t *testing.T, dir string, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "version.php"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadVersionPlugin(t *testing.T) {
	dir := t.TempDir()

	writeVersion(t, dir, `<?php
defined('MOODLE_INTERNAL') || die();

$plugin->component = 'tool_mulib';
$plugin->version   = 2026060550;
$plugin->requires  = 2025100600;
$plugin->release   = 'v5.0.8.01';
$plugin->maturity  = MATURITY_STABLE;
`)

	v, err := ReadVersion(dir)
	if err != nil {
		t.Fatalf("ReadVersion: %v", err)
	}

	if v.Version != "2026060550" || v.Release != "v5.0.8.01" {
		t.Errorf("ReadVersion = %+v", v)
	}
}

func TestReadVersionCore(t *testing.T) {
	dir := t.TempDir()

	public := filepath.Join(dir, "public")

	if err := os.MkdirAll(public, 0o755); err != nil {
		t.Fatal(err)
	}

	// Core's version.php uses plain variables, and its release carries a build
	// stamp that would widen a listing column for no benefit.
	writeVersion(t, public, `<?php
$version  = 2026071600.00;
$release  = '5.2.1+ (Build: 20260716)';
$branch   = '502';
$maturity = MATURITY_STABLE;
`)

	v, err := ReadVersion(dir)
	if err != nil {
		t.Fatalf("ReadVersion: %v", err)
	}

	if v.Version != "2026071600.00" {
		t.Errorf("Version = %q", v.Version)
	}

	if v.Release != "5.2.1+" {
		t.Errorf("Release = %q, want the build stamp trimmed", v.Release)
	}
}

func TestReadVersionWithoutFile(t *testing.T) {
	// Plenty of checkouts have no version.php — a library, a theme fork, or a
	// repository someone cloned by hand. That is not an error.
	v, err := ReadVersion(t.TempDir())
	if err != nil {
		t.Fatalf("ReadVersion: %v", err)
	}

	if !v.Empty() {
		t.Errorf("ReadVersion = %+v, want empty", v)
	}
}
