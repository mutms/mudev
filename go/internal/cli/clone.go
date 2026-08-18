package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/go/internal/workspace"
)

// newCloneCmd builds `mudev clone <recipe>`.
func newCloneCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clone <recipe>",
		Short: "Assemble a Moodle code tree from a recipe into the current directory",
		Long: "Assemble a Moodle code tree from a recipe into the current directory.\n\n" +
			"<recipe> is either a recipe file (a path ending in .yaml/.yml, handy for an\n" +
			"ad-hoc private recipe) or a catalogue identifier vendor/stream/version, e.g.\n" +
			"mutms/release/5.2.2.01, resolved in the recipes directory.\n\n" +
			"Moodle core is checked out into the current directory — which mudev never\n" +
			"creates, so it keeps the owner and permissions you gave it — and each plugin\n" +
			"becomes its own git checkout at its path inside that tree.\n\n" +
			"The run is idempotent: every step records itself in .mudev.json as it goes and\n" +
			"skips what is already in place, so an interrupted clone is resumed, and a\n" +
			"recipe that has gained a plugin is caught up, by running the command again.",
		Args: cobra.ExactArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			shallow, err := cmd.Flags().GetBool("shallow")
			if err != nil {
				return err
			}

			return workspace.Clone(cmd.Context(), workspace.Options{
				Config:  s.cfg,
				Recipe:  args[0],
				Root:    root,
				Out:     cmd.OutOrStdout(),
				Shallow: shallow,
			})
		},
	}

	cmd.Flags().Bool("shallow", false,
		"fetch only the tip commit, not the full history: a tag or commit lands\n"+
			"detached, a branch as a local branch with no upstream (a later\n"+
			"`mudev fetch` unshallows). Best effort — a remote that will not serve\n"+
			"the ref at depth 1 is assembled in full instead")

	return cmd
}
