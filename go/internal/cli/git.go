package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/go/internal/workspace"
)

// newGitCmd builds the `mudev git` group: git settings applied to every
// repository under the current directory.
func newGitCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Apply a git setting to every repository below the current directory",
	}

	cmd.AddCommand(newGitSetCmd(s))

	return cmd
}

// newGitSetCmd builds `mudev git set <key> <value>`.
func newGitSetCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a git config value in every repository below the current directory",
		Long: "Set one git config value in every repository in the current directory and below.\n\n" +
			"One key and one value per call, as with `git config set`:\n\n" +
			"  mudev git set user.name \"Your Name\"\n" +
			"  mudev git set user.email you@example.org\n\n" +
			"The value is written to each repository's own .git/config, so it overrides\n" +
			"~/.gitconfig for these repositories only. Repositories are found by searching the\n" +
			"directory tree; .mudev.json is not used. node_modules and vendor are not searched.\n\n" +
			"Nothing is remembered: run the command again after you add a plugin.",
		Args: cobra.ExactArgs(2),

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.GitSet(cmd.Context(), workspace.GitSetOptions{
				Config: s.cfg,
				Root:   root,
				Key:    args[0],
				Value:  args[1],
				Out:    cmd.OutOrStdout(),
			})
		},
	}

	return cmd
}
