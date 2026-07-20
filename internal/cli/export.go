package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/internal/workspace"
)

// newExportCmd builds `mudev export`.
func newExportCmd() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write the current workspace's recipe out as YAML",
		Long: "Write the current workspace's recipe out as YAML.\n\n" +
			"It prints to standard output by default, so it pipes and diffs; --file writes it\n" +
			"to a recipe file instead, which must be named .yaml or .yml.\n\n" +
			"What comes out is .mudev.json rendered as a recipe: every plugin flattened, with\n" +
			"no dependency on a plugin catalogue, so it describes the same tree wherever it is\n" +
			"taken. It is what the workspace was assembled to be — for what its checkouts are\n" +
			"doing right now, use `mudev list`.",
		Args: cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.Export(workspace.ExportOptions{
				Root: root,
				File: file,
				Out:  cmd.OutOrStdout(),
			})
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "write to this recipe file (.yaml or .yml) instead of standard output")

	return cmd
}
