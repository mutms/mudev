package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/go/internal/workspace"
)

// newProductionCmd builds the `mudev production` group: turning an assembled
// workspace into a deployable artifact, as opposed to the recipe and checkout
// management the other commands do.
func newProductionCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "production",
		Short: "Build deployable artifacts from the current workspace",
		Long: "Build deployable artifacts from the current workspace.\n\n" +
			"Where the other commands manage the recipe and its checkouts, these take an\n" +
			"assembled tree and turn it into something to ship.",
	}

	cmd.AddCommand(newProductionExportCmd(s))

	return cmd
}

// newProductionExportCmd builds `mudev production export <target.tgz>`.
func newProductionExportCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <target.tgz>",
		Short: "Pack the workspace into a deployable .tgz",
		Long: "Pack the current workspace into a single gzipped tar for deployment.\n\n" +
			"It exports the committed state of every checkout the live recipe records, laid\n" +
			"out at its real path in the tree. Repositories cloned alongside the recipe but\n" +
			"not recorded in .mudev.json are left out — the artifact is the recipe and nothing\n" +
			"else.\n\n" +
			"The tree must be clean: a recorded checkout with uncommitted changes (tracked,\n" +
			"staged or untracked) stops the run before anything is built, so what ships is\n" +
			"exactly what `mudev list` shows. When the exported tree carries Moodle's 5.1+\n" +
			"public/ layout, `composer install --no-dev --classmap-authoritative` runs at the\n" +
			"tree root before the tarball is written.\n\n" +
			"The target must be named .tgz or .tar.gz.",
		Args: cobra.ExactArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.ProductionExport(cmd.Context(), workspace.ProductionExportOptions{
				Config: s.cfg,
				Root:   root,
				Target: args[0],
				Out:    cmd.OutOrStdout(),
			})
		},
	}

	return cmd
}
