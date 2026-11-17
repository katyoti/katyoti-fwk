// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestACatchUpKnowsItsRepositoryWhateverFormTheRootTakes pins:
// the project root is given in another form than the one git answers for its
// top level - an 8.3 short name on Windows (as a CI runner's TEMP is), a
// symlinked folder elsewhere - and a commit made outside CWM is still caught up
// as the commits between the two HEADs, never as the working tree.
func TestACatchUpKnowsItsRepositoryWhateverFormTheRootTakes(t *testing.T) {
	long := filepath.Join(t.TempDir(), "a-project-folder-with-a-long-name")
	if err := os.MkdirAll(long, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := otherPathForm(t, long)
	r := &catchUpRepo{t: t, root: alias, ws: freshWorkspace(t)}
	r.git("init", "-q")
	paths := r.seeded("cwm.a")
	r.open() // the cursor at the seed's commit
	from := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	r.handEdit(paths["cwm.a"], "name: cwm.a", "name: edited by hand")
	r.commit("edited outside")
	sha := strings.TrimSpace(r.git("rev-parse", "HEAD"))

	_, c := r.openPublishing()
	p := caughtUp(t, c)
	if p.Cause != (api.CaughtUpCause{Kind: api.CaughtUpCommits, From: from, To: sha}) {
		t.Fatalf("root %s (git's top level is the long form of %s): cause %+v, want the commits %s..%s", alias, long, p.Cause, from, sha)
	}
}
