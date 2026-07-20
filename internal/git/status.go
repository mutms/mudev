package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mutms/mudev/internal/exec"
)

// Status is the live state of one checkout, as git sees it.
type Status struct {
	// Branch is the checked-out branch, empty when HEAD is detached.
	Branch string

	// Detached reports a HEAD that is not on a branch — the normal state for a
	// plugin pinned to a tag or commit by a release recipe.
	Detached bool

	// Head is the abbreviated commit HEAD points at.
	Head string

	// Tracking is the upstream branch (e.g. "origin/MOODLE_502_STABLE"), empty
	// when the branch has none.
	Tracking string

	// Ahead and Behind count the commits by which the branch and its upstream
	// have diverged: unpushed work and incoming work respectively.
	Ahead  int
	Behind int

	// Dirty reports uncommitted changes in the working tree or index.
	Dirty bool

	// Unborn reports a repository whose HEAD has no commit yet — a directory
	// someone ran `git init` in. There is nothing to compare or tag, so the
	// rest of the fields (except Branch and Dirty) stay zero.
	Unborn bool

	// Tags are the tags pointing at HEAD — what identifies a released
	// checkout at a glance.
	Tags []string
}

// Status collects the state of the checkout in dir.
//
// It runs several plumbing commands rather than parsing one porcelain line,
// because each answer is independently useful and the parsing stays obvious.
func (c *Client) Status(ctx context.Context, dir string) (Status, error) {
	var s Status

	// An unborn HEAD makes rev-parse fail; the branch it *would* commit to is
	// still readable, and the working tree is still worth reporting.
	head, err := c.optional(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		s.Unborn = true
		s.Branch, _ = c.optional(ctx, dir, "symbolic-ref", "--short", "HEAD")

		if dirty, err := c.capture(ctx, dir, "status", "--porcelain"); err == nil {
			s.Dirty = dirty != ""
		}

		return s, nil
	}

	if head == "HEAD" {
		s.Detached = true
	} else {
		s.Branch = head
	}

	if sha, err := c.capture(ctx, dir, "rev-parse", "--short", "HEAD"); err == nil {
		s.Head = sha
	}

	// A branch without an upstream is normal (a fresh local branch), so a
	// failure here is an answer, not an error.
	if tracking, err := c.optional(ctx, dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}"); err == nil && tracking != "" {
		s.Tracking = tracking

		ahead, behind, err := c.divergence(ctx, dir, head, tracking)
		if err != nil {
			return s, err
		}

		s.Ahead, s.Behind = ahead, behind
	}

	dirty, err := c.capture(ctx, dir, "status", "--porcelain")
	if err != nil {
		return s, err
	}

	s.Dirty = dirty != ""

	tags, err := c.capture(ctx, dir, "tag", "--points-at", "HEAD")
	if err != nil {
		return s, err
	}

	if tags != "" {
		s.Tags = strings.Split(tags, "\n")
	}

	return s, nil
}

// divergence counts the commits on each side of branch...tracking.
func (c *Client) divergence(ctx context.Context, dir string, branch string, tracking string) (ahead int, behind int, err error) {
	out, err := c.capture(ctx, dir, "rev-list", "--left-right", "--count", branch+"..."+tracking)
	if err != nil {
		return 0, 0, err
	}

	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("unexpected rev-list output %q", out)
	}

	// Left is the branch (ours, unpushed), right is the upstream (incoming).
	if ahead, err = strconv.Atoi(fields[0]); err != nil {
		return 0, 0, err
	}

	if behind, err = strconv.Atoi(fields[1]); err != nil {
		return 0, 0, err
	}

	return ahead, behind, nil
}

// optional runs a command whose failure is an expected answer ("there is no
// upstream") rather than a problem to report.
func (c *Client) optional(ctx context.Context, dir string, args ...string) (string, error) {
	res, err := exec.Capture(ctx, exec.Cmd{Name: "git", Args: args, Dir: dir})
	if err != nil {
		return "", err
	}

	if res.Failed() {
		return "", fmt.Errorf("git %s: exit status %d", strings.Join(args, " "), res.Code)
	}

	return res.Stdout, nil
}
