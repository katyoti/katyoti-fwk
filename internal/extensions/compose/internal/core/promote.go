// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/gitcli"
	"github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
)

// PromoteMarketplace commits a marketplace tree into its git repo, and pushes
// ONLY when explicitly asked.
//
// A team marketplace IS a git repo (team-marketplace.md §git hosting): who may
// publish and what gets reviewed come free from git permissions and PRs. Until
// now the publish chain stopped one step short — it produced a signed, verified
// tree and left "now commit and push it" as a manual step, which is exactly the
// step a human does inconsistently.
//
// PUSH IS OPT-IN. Committing is local and reversible; pushing publishes to
// everyone who has subscribed, and a bad push is visible to the whole team
// before it can be withdrawn. So the default does the safe half and says what
// the other half would be.
//
// It deliberately stages ONLY the marketplace folder. A publisher's repo often
// holds more than the catalog, and `git add -A` from a publish verb would sweep
// unrelated work into a release commit.
func (s *Service) PromoteMarketplace(_ context.Context, in api.PromoteMarketplaceInput) (api.PromoteMarketplaceOutput, error) {
	var out api.PromoteMarketplaceOutput
	dir := wsPath(in.Marketplace)
	if strings.TrimSpace(dir) == "" {
		return out, fmt.Errorf("promote-marketplace: the marketplace folder is required")
	}

	top, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return out, fmt.Errorf("%s is not inside a git repository — a team marketplace is a git repo (see modules/katyoti/team-marketplace.md); host it another way, or `git init` here", dir)
	}
	out.Repo = strings.TrimSpace(top)

	if _, err := git(dir, "add", "--", "."); err != nil {
		return out, fmt.Errorf("stage %s: %w", dir, err)
	}

	// Nothing staged means nothing was published since the last promote. That
	// is a normal no-op, not a failure: re-running a publish chain that changed
	// nothing should be quiet.
	if _, err := git(dir, "diff", "--cached", "--quiet", "--", "."); err == nil {
		out.Committed = false
		return out, nil
	}

	msg := strings.TrimSpace(in.Message)
	if msg == "" {
		msg = "publish marketplace"
	}
	if _, err := git(dir, "commit", "-m", msg, "--", "."); err != nil {
		return out, fmt.Errorf("commit: %w", err)
	}
	out.Committed = true
	if sha, err := git(dir, "rev-parse", "--short", "HEAD"); err == nil {
		out.Commit = strings.TrimSpace(sha)
	}

	if !in.Push {
		return out, nil
	}
	if _, err := git(dir, "push"); err != nil {
		// The commit stands; only the publish half failed. Say so, because the
		// recovery is `git push`, not re-running the whole publish chain.
		return out, fmt.Errorf("committed %s but push failed: %w", out.Commit, err)
	}
	out.Pushed = true
	return out, nil
}

func git(dir string, args ...string) (string, error) { return gitcli.Run(dir, args...) }
