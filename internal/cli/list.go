package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/internal/workspace"
)

// newListCmd builds `mudev list`.
func newListCmd(s *settings) *cobra.Command {
	var columns []string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show every checkout in the workspace and its git state",
		Long: "Show every checkout in the workspace: the plugins the live recipe records,\n" +
			"plus any other git repository found in the tree.\n\n" +
			"Rows are identified by the checkout's path in the tree — including the\n" +
			"public/ prefix on Moodle 5.1 and later — with Moodle core itself as \".\".\n\n" +
			"The state column marks:\n" +
			"  N" + workspace.MarkerAhead + "  commits not pushed yet\n" +
			"  N" + workspace.MarkerBehind + "  commits waiting to be pulled\n" +
			"  " + workspace.MarkerDirty + "   uncommitted changes\n" +
			"  " + workspace.MarkerStrayed + "   on a different branch from the one the recipe recorded\n" +
			"  " + workspace.MarkerUnmanaged + "   a checkout the live recipe does not record\n" +
			"  " + workspace.MarkerMissing + "   recorded by the live recipe but not on disk\n\n" +
			"Columns: " + strings.Join(workspace.KnownColumns(), ", ") + "\n" +
			"(default: " + strings.Join(workspace.DefaultColumns(), ", ") + ")",
		Args:    cobra.NoArgs,
		Aliases: []string{"ls"},

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			repos, err := workspace.List(cmd.Context(), workspace.ListOptions{
				Config: s.cfg,
				Root:   root,
			})
			if err != nil {
				return err
			}

			// Silence would look the same as a broken command; say so, but on
			// stderr, so a piped listing stays clean.
			if len(repos) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "no git checkouts found in %s\n", root)

				return nil
			}

			return workspace.RenderList(cmd.OutOrStdout(), repos, columns)
		},
	}

	cmd.Flags().StringSliceVar(&columns, "columns", nil,
		"columns to show, comma separated (default: "+strings.Join(workspace.DefaultColumns(), ",")+")")

	return cmd
}
