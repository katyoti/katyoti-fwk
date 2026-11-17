// SPDX-License-Identifier: Apache-2.0

package census

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// assertProbedManifestCarriesNoLocalEntry — blind audit M3: a `local:`
// contribution is binary-private and never crosses a process boundary. The
// census records the binary's SELF view, local entries included (that is the
// point of the scope, and the goldens carry `local:/service/sidecar`); this
// guard asks the other question — what does the AGGREGATOR get when it probes
// this binary? — through the binary's own ResourceService, with the one
// request the hub's probe renders as CLI flags (appcore.MemberManifestRequest).
// Nothing `local:` may come back.
//
// A binary with no ResourceService cannot be probed by a hub at all, so it is
// no member and the question does not arise (ktw-test).
func assertProbedManifestCarriesNoLocalEntry(t *testing.T, name string, app *application.Application) {
	t.Helper()
	ctx := application.WithApplication(context.Background(), app)
	svc, err := application.UseService[mbapi.ResourceService](ctx)
	if err != nil {
		t.Logf("census[%s]: no ResourceService (%v): the hub cannot probe this binary, nothing to guard", name, err)
		return
	}
	out, err := svc.GetManifest(ctx, appcore.MemberManifestRequest(false))
	if err != nil {
		t.Errorf("census[%s]: the manifest as the hub probes it: %v", name, err)
		return
	}
	if out.Manifest == nil {
		t.Errorf("census[%s]: the manifest as the hub probes it is nil", name)
		return
	}
	if bad := LocalEntries(out.Manifest); len(bad) > 0 {
		t.Errorf("census[%s]: the manifest the hub probes from this binary carries %d local: entr%s — a local contribution never crosses a process boundary (blind audit M3):\n  %s",
			name, len(bad), map[bool]string{true: "y", false: "ies"}[len(bad) == 1], strings.Join(bad, "\n  "))
	}
}

// LocalEntries is the decision: the URI of every entry that is not bus-visible
// (contribapi.IsBusVisible), sorted. Exported for the compiled-binaries and
// hub checks, which ask the same question of a probed record.
func LocalEntries(m *mbapi.Manifest) []string {
	var out []string
	for _, c := range m.Contributions {
		if !contribapi.IsBusVisible(c.Provider) {
			out = append(out, c.ResolvedURI())
		}
	}
	sort.Strings(out)
	return out
}
