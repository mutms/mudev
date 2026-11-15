package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/mutms/mudev/go/internal/config"
	"github.com/mutms/mudev/go/internal/git"
)

// GitSetOptions configure GitSet.
type GitSetOptions struct {
	// Config is the resolved configuration.
	Config config.Config

	// Root is the directory to search for repositories.
	Root string

	// Key and Value are passed to `git config` unchanged.
	Key   string
	Value string

	// Out receives one line per repository.
	Out io.Writer
}

// GitSet writes one git config value into the local config of every
// repository found in Root and below.
//
// It walks the filesystem and does not read .mudev.json. A failing repository
// does not stop the run; the errors are returned together at the end.
func GitSet(ctx context.Context, opts GitSetOptions) error {
	repos, err := discover(opts.Root)
	if err != nil {
		return err
	}

	if len(repos) == 0 {
		return fmt.Errorf("no git repositories found in %s", opts.Root)
	}

	client := git.New(opts.Config)

	out := newOutput(opts.Out)

	var failed []error

	for _, path := range repos {
		dir := filepath.Join(opts.Root, filepath.FromSlash(path))

		if err := client.SetConfig(ctx, dir, opts.Key, opts.Value); err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", path, err))

			continue
		}

		out.printf("%s: %s set", path, opts.Key)
	}

	return errors.Join(failed...)
}
