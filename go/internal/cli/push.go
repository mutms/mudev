package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/go/internal/workspace"
)

// newPushCmd builds `mudev push`.
func newPushCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push every plugin's unpushed commits to origin",
		Long: "Push the plugins of the workspace to their origin remote, each to the branch\n" +
			"the live recipe recorded for it. Moodle core is never pushed — do that by hand.\n\n" +
			"A plugin is pushed only while it is on the branch the recipe expects. One on\n" +
			"another branch, or with a detached HEAD, is reported and left alone: where that\n" +
			"work belongs is your decision. Repositories the recipe does not record are\n" +
			"reported the same way.\n\n" +
			"Plugins with nothing to push print nothing and are counted at the end. Nothing\n" +
			"is forced: the first push the remote rejects stops the run there.",
		Args: cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.Push(cmd.Context(), workspace.FanOptions{
				Config: s.cfg,
				Root:   root,
				Out:    cmd.OutOrStdout(),
			})
		},
	}

	return cmd
}
