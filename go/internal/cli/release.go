package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/go/internal/workspace"
)

// newReleaseCmd builds the `mudev release` group: preparing a release of the
// workspace's release-managed plugins.
func newReleaseCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Prepare a release of the workspace's release-managed plugins",
		Long: "Prepare a release of the workspace's release-managed plugins.\n\n" +
			"A plugin is release-managed when its live recipe entry carries the recipe's own\n" +
			"extra.mudev.release flavour. Release commands work on Moodle 5.3 and later only;\n" +
			"older MuTMS releases are made with the old release tool.",
	}

	set := &cobra.Command{
		Use:   "set",
		Short: "Set a release value in every release-managed plugin",
	}

	set.AddCommand(newReleaseSetVersionCmd(s))
	cmd.AddCommand(set)

	return cmd
}

// newReleaseSetVersionCmd builds `mudev release set version <value>`.
func newReleaseSetVersionCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version <YYYYMMDDxx|today>",
		Short: "Set $plugin->version in every release-managed plugin",
		Long: "Set $plugin->version in every release-managed plugin of the current workspace,\n" +
			"and point their dependencies on each other at the same version.\n\n" +
			"  mudev release set version 2026110553\n" +
			"  mudev release set version today\n\n" +
			"The version is a real date followed by the branch suffix, the first and last digit\n" +
			"of mdlbranch: 2026110553 is 5 November 2026 on 503. A value with another branch's\n" +
			"suffix is rejected. `today` is today's date with this workspace's suffix.\n\n" +
			"A plugin is skipped, with the reason shown, when it is not checked out, when the\n" +
			"recipe pins it to a tag or commit, when its recorded branch is not one of this\n" +
			"Moodle branch (MOODLE_503_* on 503), or when it is checked out on another branch.\n\n" +
			"Every change is worked out first. If the new version is lower than any plugin's\n" +
			"current one, nothing is written: Moodle refuses downgrades. Nothing is committed.",
		Args: cobra.ExactArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.SetVersion(cmd.Context(), workspace.SetVersionOptions{
				Config: s.cfg,
				Root:   root,
				Value:  args[0],
				Out:    cmd.OutOrStdout(),
			})
		},
	}

	return cmd
}
