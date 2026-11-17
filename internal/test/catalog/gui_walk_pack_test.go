// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os"
	"sort"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// guiWalkUniverse is the kebab universe token of the GUI walk.
const guiWalkUniverse = "gui-walk"

// startGUIWalkUniverse stands up the GUI walk's universe: the six contract
// sidecars and, when bin/ktw-host is built, the e19 files-only pack served by
// it (the same member files_only_kind_test.go declares). Reports whether the
// pack is a member. Without ktw-host the walk still runs over the six — unless
// KTW_E2E says every binary is built, where a missing host is a failure.
func startGUIWalkUniverse(t *testing.T) (*messagebus.Sender, *manifestremote.Catalog, bool) {
	t.Helper()
	root := repoRoot(t)
	var members []packMember
	for _, name := range sidecarsToInclude {
		bin := binaryFor(t, name)
		if bin == "" {
			hubharness.MissingBinary(t, "GUI walk: sidecar binary %s not built", name)
			return nil, nil, false
		}
		members = append(members, packMember{Name: name, Binary: bin})
	}
	packed := false
	if host := binaryFor(t, "ktw-host"); host != "" {
		members = append(members, packMember{Name: fixtureProvider(t), Binary: host, Folder: fixtureCopy(t, root, "files-only-pack")})
		packed = true
	} else if os.Getenv("KTW_E2E") != "" {
		hubharness.MissingBinary(t, "GUI walk: bin/ktw-host not built, so the files-only pack cannot be walked")
		return nil, nil, false
	} else {
		t.Log("GUI walk: bin/ktw-host not built — walking the six sidecars without the files-only pack")
	}
	sender, cat := startPackUniverse(t, guiWalkUniverse, members)
	return sender, cat, packed
}

// fixtureProvider and fixtureKind read the provider token and the kind off the
// fixture's entry URI, so nothing here spells the kind a second time.
func fixtureProvider(t *testing.T) string { return fixtureURI(t).Provider }
func fixtureKind(t *testing.T) string     { return fixtureURI(t).Type }

func fixtureURI(t *testing.T) contribapi.ResourceURI {
	t.Helper()
	u, err := contribapi.ParseResourceURI(fixtureEntryURI)
	if err != nil {
		t.Fatalf("fixture entry URI %q: %v", fixtureEntryURI, err)
	}
	return u
}

// assertPackEntryOpened holds the walk's record of the fixture entry to its
// kind's declaration: every declared layer opened, none refused, none failed.
func assertPackEntryOpened(t *testing.T, kinds contribapi.Kinds, got map[string]string) {
	t.Helper()
	decl, ok := kinds.Get(fixtureKind(t))
	if !ok {
		t.Fatalf("GUI walk: the pack's kind %q is not in the universe registry", fixtureKind(t))
	}
	if len(decl.Layers) == 0 {
		t.Fatalf("GUI walk: the pack's kind declares no layers — nothing to prove")
	}
	var bad []string
	for name := range decl.Layers {
		if state := got[name]; state != "opened" {
			if state == "" {
				state = "never reached (not carried by the index entry)"
			}
			bad = append(bad, name+": "+state)
		}
	}
	sort.Strings(bad)
	if len(bad) > 0 {
		t.Errorf("GUI walk: %s did not open every declared layer:\n  %s", fixtureEntryURI, strings.Join(bad, "\n  "))
		return
	}
	t.Logf("GUI walk: %s opened all %d declared layers with zero refusals", fixtureEntryURI, len(decl.Layers))
}
