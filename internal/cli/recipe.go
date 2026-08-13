package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/internal/workspace"
)

// newRecipeCmd builds the `mudev recipe` group: the commands that work on the
// recipe of the workspace you are standing in, rather than on its checkouts.
func newRecipeCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipe",
		Short: "Work on the recipe of the current workspace",
		Long: "Work on the recipe of the current workspace.\n\n" +
			"`mudev clone` records what it assembled in .mudev.json, the live recipe.\n" +
			"These commands change what that workspace contains, and therefore make it\n" +
			"differ from the recipe it was assembled from.",
	}

	cmd.AddCommand(newRecipeInitCmd(s))
	cmd.AddCommand(newRecipeUpdateCmd(s))
	cmd.AddCommand(newRecipeAddCmd(s))
	cmd.AddCommand(newRecipePruneCmd())
	cmd.AddCommand(newRecipeSetCmd())
	cmd.AddCommand(newRecipeExportCmd())

	return cmd
}

// newRecipeExportCmd builds `mudev recipe export`.
func newRecipeExportCmd() *cobra.Command {
	var (
		file string
		sort bool
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write the current workspace's recipe out as YAML",
		Long: "Write the current workspace's recipe out as YAML.\n\n" +
			"It prints to standard output by default, so it pipes and diffs; --file writes it\n" +
			"to a recipe file instead, which must be named .yaml or .yml.\n\n" +
			"What comes out is .mudev.json rendered as a recipe: every plugin flattened, with\n" +
			"no dependency on a plugin catalogue, so it describes the same tree wherever it is\n" +
			"taken. It is what the workspace was assembled to be — for what its checkouts are\n" +
			"doing right now, use `mudev list`.\n\n" +
			"Plugins come out in the order the workspace was assembled in; --sort orders them by\n" +
			"install path instead, which is how you look one up in a long recipe, and makes two\n" +
			"exports of the same plugins diff cleanly.",
		Args: cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.Export(workspace.ExportOptions{
				Root: root,
				File: file,
				Sort: sort,
				Out:  cmd.OutOrStdout(),
			})
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "write to this recipe file (.yaml or .yml) instead of standard output")
	cmd.Flags().BoolVar(&sort, "sort", false, "order the plugins by install path instead of by assembly order")

	return cmd
}

// newRecipeInitCmd builds `mudev recipe init`.
func newRecipeInitCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Reconstruct .mudev.json from the checkouts already in the tree",
		Long: "Reconstruct the live recipe (.mudev.json) from the git checkouts already in the\n" +
			"current directory, so a tree assembled by hand or by the old tool becomes a\n" +
			"workspace mudev can manage.\n\n" +
			"It is the reverse of `mudev clone`, and it changes no working tree: it asks git\n" +
			"what each checkout is — its remotes, its branch and tracking ref — and reads each\n" +
			"plugin's version.php for the frankenstyle component that names it. Core at the\n" +
			"root becomes the base block; every other checkout becomes a flattened plugin\n" +
			"entry, identified as <remote-owner>/<component> (e.g. mutms/tool_mulib, or\n" +
			"acme/mod_thing for a private forge). The result is a self-contained recipe, the\n" +
			"same shape clone writes, so `list`, `status`, `fetch`, `pull` and `recipe export`\n" +
			"work against it from then on.\n\n" +
			"Names are mudev's own state keys — adjust them afterwards with `recipe set` or by\n" +
			"editing the file. A checkout with no origin remote, or none that yields a valid\n" +
			"identifier, is reported and left out rather than recorded under a guess. It\n" +
			"refuses to overwrite an existing .mudev.json.",
		Args: cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.Init(cmd.Context(), workspace.InitOptions{
				Config: s.cfg,
				Root:   root,
				Out:    cmd.OutOrStdout(),
			})
		},
	}

	return cmd
}

// newRecipeUpdateCmd builds `mudev recipe update <relpath>`.
func newRecipeUpdateCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update <relpath>",
		Short: "Fold one checkout's current state into .mudev.json",
		Long: "Bring one checkout's entry in .mudev.json up to date with what is on disk.\n\n" +
			"<relpath> is the checkout's path as `mudev list` prints it — a path relative to\n" +
			"the workspace root, or \".\" for Moodle core. This is the per-checkout companion to\n" +
			"`mudev recipe init`: after the initial bulk reconstruction, run it for the one\n" +
			"plugin you just cloned into the tree, or the one whose branch or remotes you\n" +
			"changed.\n\n" +
			"A checkout the record does not know yet is adopted — identified <owner>/<component>\n" +
			"from its origin remote and its own version.php, and hidden from its containing\n" +
			"repository. A checkout it already records is refreshed: only the git identity\n" +
			"(remotes, ref, localbranch) is rewritten to match, so a name you fixed by hand\n" +
			"survives, and a checkout you moved onto a new branch is recorded where it now is.\n\n" +
			"It touches no working tree. Removing a plugin stays `recipe prune`'s job — update\n" +
			"never drops an entry.",
		Args: cobra.ExactArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.Update(cmd.Context(), workspace.UpdateOptions{
				Config:  s.cfg,
				Root:    root,
				Relpath: args[0],
				Out:     cmd.OutOrStdout(),
			})
		},
	}

	return cmd
}

// newRecipeSetCmd builds `mudev recipe set <key> <value>`.
func newRecipeSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Name the workspace: set name, description or contributed_by",
		Long: "Set one descriptive field of the current workspace's recipe.\n\n" +
			"One key and one value per call, the way `git config user.name \"…\"` works, and an\n" +
			"empty value clears the field:\n\n" +
			"  mudev recipe set name \"MuTMS dev 5.2\"\n" +
			"  mudev recipe set description \"programs + certifications, patched core\"\n" +
			"  mudev recipe set description \"\"\n\n" +
			"A workspace starts out describing the recipe it was cloned from, which stops being\n" +
			"true as soon as you add or prune plugins — `mudev recipe export` would otherwise carry a\n" +
			"name belonging to something else.\n\n" +
			"Settable: name, description, contributed_by. Everything else in the recipe is a\n" +
			"record of what mudev actually did, and changes by cloning, adding and pruning.",
		Args: cobra.ExactArgs(2),

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.Set(workspace.SetOptions{
				Root:  root,
				Key:   args[0],
				Value: args[1],
				Out:   cmd.OutOrStdout(),
			})
		},
	}

	return cmd
}

// newRecipePruneCmd builds `mudev recipe prune`.
func newRecipePruneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Drop plugins the recipe records but the tree no longer has",
		Long: "Drop plugins the recipe records but the tree no longer has.\n\n" +
			"This is how a plugin leaves a workspace: delete its directory yourself, then run\n" +
			"this to catch .mudev.json up. mudev does not delete the code, because a checkout\n" +
			"can hold uncommitted changes, unpushed commits or a stash that no recipe knows\n" +
			"about and nothing here could give back.\n\n" +
			"No git repository and no working tree is touched — entries leave the live recipe,\n" +
			"and the exclude that hid each one from its containing repository is cleaned up.\n" +
			"`mudev list` marks exactly what this would remove with ✗, so it is the preview.",
		Args: cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.Prune(workspace.PruneOptions{
				Root: root,
				Out:  cmd.OutOrStdout(),
			})
		},
	}

	return cmd
}

// newRecipeAddCmd builds `mudev recipe add <plugin>`.
func newRecipeAddCmd(s *settings) *cobra.Command {
	var ref string

	cmd := &cobra.Command{
		Use:   "add <plugin>",
		Short: "Check a plugin out into the current workspace and record it",
		Long: "Check a plugin out into the current workspace and record it in .mudev.json.\n\n" +
			"<plugin> is either a vendor/package catalogue identifier, e.g. mutms/tool_mulib,\n" +
			"or a path to a plugin file (ending in .yaml/.yml), which is how a plugin that is\n" +
			"not in the catalogue yet gets used.\n\n" +
			"The plugin lands at its relpath inside the tree, on the branch that serves this\n" +
			"workspace's Moodle. Exactly that plugin is added and nothing else: what a plugin\n" +
			"declares it requires is reported, not installed — composing a dev site is your\n" +
			"decision, and Moodle checks dependencies at install time. A plugin already in the\n" +
			"workspace is left alone, so the command is safe to repeat.",
		Args: cobra.ExactArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			return workspace.Add(cmd.Context(), workspace.AddOptions{
				Config: s.cfg,
				Plugin: args[0],
				Ref:    ref,
				Root:   root,
				Out:    cmd.OutOrStdout(),
			})
		},
	}

	cmd.Flags().StringVar(&ref, "ref", "",
		"check this tag, branch or commit out instead of the branch serving this Moodle")

	return cmd
}
