// Package cli defines the mudev command-line interface (cobra).
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/internal/config"
)

// settings holds the configuration shared by every subcommand. It starts from
// the built-in defaults, takes the environment next, and finally the
// persistent flags — mudev's precedence chain, applied in that order.
type settings struct {
	cfg config.Config
}

// newRootCmd builds the root command. Subcommands are registered here as they land.
func newRootCmd() *cobra.Command {
	s := &settings{cfg: config.Defaults()}

	cmd := &cobra.Command{
		Use:   "mudev",
		Short: "Manage git checkouts for MuTMS / Moodle plugin development",
		Long: "mudev manages git checkouts for developing MuTMS plugins for Moodle and\n" +
			"assembles Moodle test-site code trees (core + patches + plugins) for CI.\n" +
			"Linux only.",
		SilenceUsage:  true,
		SilenceErrors: true,

		// Resolution happens here so it runs once, after cobra has parsed the
		// flags but before any subcommand does work.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return s.resolve(cmd)
		},
	}

	flags := cmd.PersistentFlags()
	flags.String("plugins-dir", config.DefaultPluginsDir, "plugin catalogue directory ("+config.EnvPluginsDir+")")
	flags.String("recipes-dir", config.DefaultRecipesDir, "recipe catalogue directory ("+config.EnvRecipesDir+")")

	cmd.AddCommand(newCloneCmd(s))
	cmd.AddCommand(newListCmd(s))
	cmd.AddCommand(newFetchCmd(s))
	cmd.AddCommand(newPullCmd(s))
	cmd.AddCommand(newRecipeCmd(s))
	cmd.AddCommand(newStatusCmd(s))

	return cmd
}

// resolve applies the environment and then any flag the user actually set, so
// an untouched flag never overrides an environment variable.
func (s *settings) resolve(cmd *cobra.Command) error {
	if err := config.ApplyEnv(&s.cfg); err != nil {
		return err
	}

	flags := cmd.Flags()

	if flags.Changed("plugins-dir") {
		value, err := flags.GetString("plugins-dir")
		if err != nil {
			return err
		}

		s.cfg.PluginsDir = value
	}

	if flags.Changed("recipes-dir") {
		value, err := flags.GetString("recipes-dir")
		if err != nil {
			return err
		}

		s.cfg.RecipesDir = value
	}

	return s.cfg.Resolve()
}

// Execute runs the root command and exits non-zero on error.
func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		// Writing the error to stderr cannot meaningfully fail here, and we
		// are about to exit non-zero regardless — explicitly discard.
		_, _ = fmt.Fprintln(os.Stderr, "mudev:", err)
		os.Exit(1)
	}
}
