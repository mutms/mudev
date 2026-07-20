package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/internal/workspace"
)

// newStatusCmd builds `mudev status [git status arguments]`.
func newStatusCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [git status arguments]",
		Short: "Show git status for the checkouts that have something to report",
		Long: "Run git status in every checkout of the workspace that has something to\n" +
			"report — uncommitted changes, or commits that have not been pushed.\n\n" +
			"A workspace holds twenty-odd repositories and most of them are untouched on\n" +
			"any given day, so the quiet ones are left out and counted at the end; --all\n" +
			"shows every checkout. A checkout that is merely behind its upstream is not\n" +
			"reported here — that is incoming work rather than something of yours waiting\n" +
			"to be committed, and mudev list shows it as N↓.\n\n" +
			"Every argument except --all goes straight to git, flags included, so\n" +
			"`mudev status -s` and `mudev status -uall` mean exactly what they mean in\n" +
			"git. (Because the arguments belong to git, mudev's global options are not\n" +
			"accepted on this command; the MUDEV_* environment variables still apply.)",
		Args: cobra.ArbitraryArgs,

		// git's flags are git's. Parsing them here would either reject them as
		// unknown or, worse, swallow them silently — so mudev takes only the
		// one argument of its own and hands the rest over untouched.
		DisableFlagParsing: true,

		RunE: func(cmd *cobra.Command, args []string) error {
			all, gitArgs, help := splitPassthroughArgs(args)

			if help {
				return cmd.Help()
			}

			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.Status(cmd.Context(), workspace.FanOptions{
				Config: s.cfg,
				Root:   root,
				Out:    cmd.OutOrStdout(),
			}, all, gitArgs)
		},
	}

	return cmd
}

// splitPassthroughArgs separates mudev's own arguments from git's.
//
// Only --all and the help flags are mudev's; everything else — including
// anything that looks like a flag — belongs to the git command being run.
func splitPassthroughArgs(args []string) (all bool, gitArgs []string, help bool) {
	for _, arg := range args {
		switch arg {
		case "--all":
			all = true

		case "-h", "--help":
			help = true

		default:
			gitArgs = append(gitArgs, arg)
		}
	}

	return all, gitArgs, help
}
