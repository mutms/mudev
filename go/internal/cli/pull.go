package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/go/internal/workspace"
)

// newPullCmd builds `mudev pull`.
func newPullCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Fast-forward every checkout in the workspace",
		Long: "Run git pull --ff-only in every checkout of the workspace — Moodle core, every\n" +
			"plugin the live recipe records, and any other git repository found in the tree.\n\n" +
			"Only fast-forwards are accepted. A checkout that has diverged from its\n" +
			"upstream — one that would need a rebase or a merge — stops the run there and\n" +
			"then, leaving everything after it untouched, so you can deal with that one\n" +
			"repository and run mudev pull again.\n\n" +
			"Checkouts with nothing to pull into are skipped, not treated as failures: a\n" +
			"pinned edition is checked out detached, and a new local branch may not have an\n" +
			"upstream yet.",
		Args: cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.Pull(cmd.Context(), workspace.FanOptions{
				Config: s.cfg,
				Root:   root,
				Out:    cmd.OutOrStdout(),
			})
		},
	}

	return cmd
}
