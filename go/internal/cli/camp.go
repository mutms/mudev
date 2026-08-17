package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/mutms/mudev/go/internal/workspace"
)

// newCampCmd builds the `mudev camp` group: registering the workspace's
// plugins with the camp registry (https://camp-registry.org), which keeps a
// plugin's descriptive content in the plugin's own repository.
func newCampCmd(s *settings) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "camp",
		Short: "Register the workspace's plugins with the camp registry",
		Long: "Register the workspace's plugins with the camp registry.\n\n" +
			"camp (https://camp-registry.org) keeps each plugin's listing content in the\n" +
			"plugin's own repository, at .camp/listing.yml, and ingests it at every tagged\n" +
			"release. These commands generate that content from what the workspace already\n" +
			"knows about each plugin.",
	}

	cmd.AddCommand(newCampInitCmd(s))

	return cmd
}

// newCampInitCmd builds `mudev camp init [relpath]`.
func newCampInitCmd(s *settings) *cobra.Command {
	var (
		labels   []string
		noBadges bool
		force    bool
	)

	cmd := &cobra.Command{
		Use:   "init [relpath]",
		Short: "Write .camp/listing.yml into the plugin checkouts",
		Long: "Write a camp listing manifest into each plugin checkout, ready to commit.\n\n" +
			"The manifest is generated from the plugin itself: the display name is its\n" +
			"pluginname language string, the summary is its README's opening line, and the\n" +
			"documentation and issue links come from the origin remote the live recipe\n" +
			"recorded. camp's export-ignore rules are appended to .gitattributes alongside,\n" +
			"keeping development files out of the distribution ZIPs.\n\n" +
			"With no argument every recorded plugin is done. <relpath> is the checkout's\n" +
			"path as `mudev list` prints it, for one plugin at a time; Moodle core has no\n" +
			"listing, so \".\" is not accepted.\n\n" +
			"A plugin forked from somebody else's repository is skipped: camp keys a listing\n" +
			"to one canonical source, and a fork is not it.\n\n" +
			"An existing manifest is never overwritten — the file is meant to be edited by\n" +
			"hand after generation, which is where the long description goes. Use --force to\n" +
			"regenerate it anyway.",
		Args: cobra.MaximumNArgs(1),

		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}

			relpath := ""
			if len(args) > 0 {
				relpath = args[0]
			}

			return workspace.CampInit(workspace.CampInitOptions{
				Root:     root,
				Relpath:  relpath,
				Labels:   labels,
				NoBadges: noBadges,
				Force:    force,
				Out:      cmd.OutOrStdout(),
			})
		},
	}

	cmd.Flags().StringSliceVar(&labels, "labels", nil,
		"disclosure labels to declare (default fully-free): fully-free, freemium, "+
			"paid-service, external-account, donation-supported, commercial-support-available")
	cmd.Flags().BoolVar(&noBadges, "no-badges", false,
		"leave out the MDL Shield badge")
	cmd.Flags().BoolVar(&force, "force", false,
		"rewrite a listing manifest that is already there")

	return cmd
}
