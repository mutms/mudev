// Package config resolves mudev's runtime configuration.
//
// Every value follows one precedence chain, highest first:
//
//	flag > env var > built-in default
//
// Directory values may be given relative; Resolve makes them absolute. Paths
// found *inside* a YAML file follow a different rule — they are anchored to
// the directory of that file (see Anchor).
//
// There is deliberately no git authentication configuration. A recipe names the
// URL it means — `git@github.com:…` for a checkout you push from, https for one
// you only read — and mudev hands it to git untouched. Rewriting URLs or
// injecting tokens would be mudev second-guessing a decision git already lets
// you make properly, in ~/.gitconfig (url.<base>.insteadOf), an SSH agent, or a
// credential helper.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Environment variable names mudev reads.
const (
	EnvPluginsDir = "MUDEV_PLUGINS_DIRECTORY"
	EnvRecipesDir = "MUDEV_RECIPES_DIRECTORY"
)

// Built-in defaults — the shared /opt layout used on a developer machine.
const (
	DefaultPluginsDir = "/opt/mdl-plugins"
	DefaultRecipesDir = "/opt/mdl-recipes"
)

// LiveRecipeFile is the name of the single mudev-managed state file written at
// the root of a workspace (the "live recipe").
const LiveRecipeFile = ".mudev.json"

// Config holds the resolved settings for one mudev run.
type Config struct {
	// PluginsDir is where bare plugin references resolve
	// (<PluginsDir>/<vendor>/<package>.yaml).
	PluginsDir string

	// RecipesDir is where recipe identifiers resolve
	// (<RecipesDir>/<vendor>/<stream>/<version>.yaml).
	RecipesDir string
}

// Defaults returns the built-in configuration, before env vars and flags.
func Defaults() Config {
	return Config{
		PluginsDir: DefaultPluginsDir,
		RecipesDir: DefaultRecipesDir,
	}
}

// ApplyEnv overlays the MUDEV_* environment variables on top of c. Only
// variables that are actually present are applied, so unset ones leave the
// defaults (or an earlier value) alone.
func ApplyEnv(c *Config) error {
	if v, ok := os.LookupEnv(EnvPluginsDir); ok && v != "" {
		c.PluginsDir = v
	}

	if v, ok := os.LookupEnv(EnvRecipesDir); ok && v != "" {
		c.RecipesDir = v
	}

	return nil
}

// Resolve turns the directory settings into absolute paths. Call it once,
// after env vars and flags have been applied.
func (c *Config) Resolve() error {
	var err error

	if c.PluginsDir, err = filepath.Abs(c.PluginsDir); err != nil {
		return fmt.Errorf("plugins directory: %w", err)
	}

	if c.RecipesDir, err = filepath.Abs(c.RecipesDir); err != nil {
		return fmt.Errorf("recipes directory: %w", err)
	}

	return nil
}

// Anchor resolves a path that was read from inside a YAML file. Relative
// values are anchored to the directory holding that file (not to the process
// working directory), then made absolute. Absolute values pass through.
func Anchor(file string, path string) (string, error) {
	if path == "" {
		return "", nil
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(file), path)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("anchor %q to %q: %w", path, file, err)
	}

	return abs, nil
}
