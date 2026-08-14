package moodle

import (
	"os"
	"path/filepath"
	"testing"
)

// writeLang puts a language file where PluginName looks for it.
func writeLang(t *testing.T, dir, component, body string) {
	t.Helper()

	langDir := filepath.Join(dir, "lang", "en")

	if err := os.MkdirAll(langDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(langDir, component+".php")

	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPluginName(t *testing.T) {
	dir := t.TempDir()

	writeLang(t, dir, "tool_muprog", `<?php
$string['pluginname'] = 'Programs';
$string['pluginname_desc'] = 'Programs are designed to allow creation of course sets.';
`)

	got, err := PluginName(dir, "tool_muprog")
	if err != nil {
		t.Fatal(err)
	}

	if got != "Programs" {
		t.Errorf("PluginName = %q, want %q", got, "Programs")
	}
}

// The description string sits right next to pluginname and starts with the
// same prefix, so a lazy pattern would match it first.
func TestPluginNameIsNotTheDescription(t *testing.T) {
	dir := t.TempDir()

	writeLang(t, dir, "mod_mubook", `<?php
$string['pluginname_desc'] = 'The long one.';
$string['pluginname'] = 'Interactive book';
`)

	got, err := PluginName(dir, "mod_mubook")
	if err != nil {
		t.Fatal(err)
	}

	if got != "Interactive book" {
		t.Errorf("PluginName = %q, want %q", got, "Interactive book")
	}
}

func TestPluginNameDoubleQuotedWithApostrophe(t *testing.T) {
	dir := t.TempDir()

	writeLang(t, dir, "tool_x", `<?php
$string['pluginname'] = "Teacher's helper";
`)

	got, err := PluginName(dir, "tool_x")
	if err != nil {
		t.Fatal(err)
	}

	if got != "Teacher's helper" {
		t.Errorf("PluginName = %q, want %q", got, "Teacher's helper")
	}
}

// Activity modules keep their strings in lang/en/<modname>.php, without the
// mod_ prefix — the one plugin type that predates frankenstyle naming.
func TestPluginNameActivityModule(t *testing.T) {
	dir := t.TempDir()

	writeLang(t, dir, "mubook", `<?php
$string['pluginname'] = 'Interactive book';
`)

	got, err := PluginName(dir, "mod_mubook")
	if err != nil {
		t.Fatal(err)
	}

	if got != "Interactive book" {
		t.Errorf("PluginName = %q, want %q", got, "Interactive book")
	}
}

// A module that does spell the file out in full still wins over the short name.
func TestPluginNameActivityModulePrefersFrankenstyle(t *testing.T) {
	dir := t.TempDir()

	writeLang(t, dir, "mod_mubook", `<?php
$string['pluginname'] = 'Frankenstyle';
`)
	writeLang(t, dir, "mubook", `<?php
$string['pluginname'] = 'Short';
`)

	got, err := PluginName(dir, "mod_mubook")
	if err != nil {
		t.Fatal(err)
	}

	if got != "Frankenstyle" {
		t.Errorf("PluginName = %q, want %q", got, "Frankenstyle")
	}
}

func TestPluginNameMissingFile(t *testing.T) {
	got, err := PluginName(t.TempDir(), "tool_absent")
	if err != nil {
		t.Fatal(err)
	}

	if got != "" {
		t.Errorf("PluginName = %q, want empty", got)
	}
}

func TestPluginNameNoComponent(t *testing.T) {
	got, err := PluginName(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}

	if got != "" {
		t.Errorf("PluginName = %q, want empty", got)
	}
}

func TestPluginNameNoSuchString(t *testing.T) {
	dir := t.TempDir()

	writeLang(t, dir, "tool_y", `<?php
$string['somethingelse'] = 'Nope';
`)

	got, err := PluginName(dir, "tool_y")
	if err != nil {
		t.Fatal(err)
	}

	if got != "" {
		t.Errorf("PluginName = %q, want empty", got)
	}
}
