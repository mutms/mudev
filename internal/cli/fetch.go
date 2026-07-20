package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/internal/workspace"
)

// newFetchCmd builds `mudev fetch [--all]`.
func newFetchCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Fetch every remote of every checkout in the workspace",
		Long: "Run git fetch in every checkout of the workspace — Moodle core, every plugin\n" +
			"the live recipe records, and any other git repository found in the tree —\n" +
			"from every remote that checkout has, with its tags.\n\n" +
			"A development recipe carries mirrors and fork upstreams that should stay\n" +
			"current, and a catalogue recipe has nothing but origin, so fetching them all\n" +
			"costs it nothing.\n\n" +
			"The recipe's extra.mudev.fetch_order decides who is asked first, for example\n" +
			"{fetch_order: [backup, origin]}. Git objects are content-addressed, so a\n" +
			"mirror on the local network fills the repository first and leaves the remote\n" +
			"over the internet with only the difference to send. A remote other than\n" +
			"origin that cannot be reached is reported and stepped over — a mirror being\n" +
			"down must not stop the work.\n\n" +
			"Fetching only updates remote-tracking branches; nothing in a working tree\n" +
			"changes, and nothing is merged. Use mudev pull for that.",
		Args: cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.Fetch(cmd.Context(), workspace.FanOptions{
				Config: s.cfg,
				Root:   root,
				Out:    cmd.OutOrStdout(),
			})
		},
	}

	return cmd
}
