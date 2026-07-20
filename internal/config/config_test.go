package config

import (
	"path/filepath"
	"testing"
)

func TestApplyEnvOverridesDefaults(t *testing.T) {
	t.Setenv(EnvPluginsDir, "/somewhere/plugins")

	cfg := Defaults()

	if err := ApplyEnv(&cfg); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}

	if cfg.PluginsDir != "/somewhere/plugins" {
		t.Errorf("PluginsDir = %q", cfg.PluginsDir)
	}

	// An unset variable must leave the built-in default alone.
	if cfg.RecipesDir != DefaultRecipesDir {
		t.Errorf("RecipesDir = %q, want the default", cfg.RecipesDir)
	}
}

// TestApplyEnvIgnoresAnEmptyValue: an exported-but-empty variable is how a
// shell says "unset", and must not blank out the default.
func TestApplyEnvIgnoresAnEmptyValue(t *testing.T) {
	t.Setenv(EnvRecipesDir, "")

	cfg := Defaults()

	if err := ApplyEnv(&cfg); err != nil {
		t.Fatalf("ApplyEnv: %v", err)
	}

	if cfg.RecipesDir != DefaultRecipesDir {
		t.Errorf("RecipesDir = %q, want the default", cfg.RecipesDir)
	}
}

func TestResolveMakesDirectoriesAbsolute(t *testing.T) {
	cfg := Config{PluginsDir: "plugins", RecipesDir: "recipes"}

	if err := cfg.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if !filepath.IsAbs(cfg.PluginsDir) || !filepath.IsAbs(cfg.RecipesDir) {
		t.Errorf("not absolute: %q, %q", cfg.PluginsDir, cfg.RecipesDir)
	}
}

func TestAnchorUsesTheFilesDirectory(t *testing.T) {
	// The rule for paths *inside* a YAML file: relative to that file.
	got, err := Anchor("/opt/mdl-recipes/mutms/dev/5.2.yaml", "../../../mdl-plugins")
	if err != nil {
		t.Fatalf("Anchor: %v", err)
	}

	if want := "/opt/mdl-plugins"; got != want {
		t.Errorf("Anchor = %q, want %q", got, want)
	}

	got, err = Anchor("/opt/mdl-recipes/x.yaml", "/absolute/stays")
	if err != nil {
		t.Fatalf("Anchor: %v", err)
	}

	if want := "/absolute/stays"; got != want {
		t.Errorf("Anchor = %q, want %q", got, want)
	}
}
