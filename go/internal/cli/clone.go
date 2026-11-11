package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/go/internal/workspace"
)

// newCloneCmd builds `mudev clone <recipe>`.
func newCloneCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clone <recipe> [<directory>]",
		Short: "Assemble a Moodle code tree from a recipe into a directory",
		Long: "Assemble a Moodle code tree from a recipe into <directory>, or into the current\n" +
			"directory when none is given.\n\n" +
			"<recipe> is either a recipe file (a path ending in .yaml/.yml, handy for an\n" +
			"ad-hoc private recipe) or a catalogue identifier vendor/stream/version, e.g.\n" +
			"mutms/release/5.2.2.01, resolved in the recipes directory.\n\n" +
			"Moodle core is checked out into that directory and each plugin becomes its own\n" +
			"git checkout at its path inside the tree. A <directory> that does not exist is\n" +
			"created, like `git clone` does; an existing one keeps its owner and permissions.\n\n" +
			"The run is idempotent: every step records itself in .mudev.json as it goes and\n" +
			"skips what is already in place, so an interrupted clone is resumed, and a\n" +
			"recipe that has gained a plugin is caught up, by running the command again.",
		Args: cobra.RangeArgs(1, 2),

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			create := len(args) == 2
			if create {
				root = args[1]
			}

			return workspace.Clone(cmd.Context(), workspace.Options{
				Config:     s.cfg,
				Recipe:     args[0],
				Root:       root,
				CreateRoot: create,
				Out:        cmd.OutOrStdout(),
			})
		},
	}

	return cmd
}
