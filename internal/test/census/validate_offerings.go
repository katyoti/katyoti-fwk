// SPDX-License-Identifier: Apache-2.0

package census

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// assertEveryOfferingIsPublished: what a binary offers on the bus is
// discoverable through the one publication mechanism (goal G21). Every
// PublishedService that carries a KTSP container — a callable slot — has a
// matching `<provider>:/service/<name>` contribution in the manifest; a
// binary-private (WithLocal) slot has it under the `local` provider.
//
// A slot published without a ServiceDef is exactly the parallel registry G21
// forbids: callable over the bus, advertised nowhere. The services provider
// skips such entries silently; this guard names them.
func assertEveryOfferingIsPublished(t *testing.T, name string, app *application.Application, m *mbapi.Manifest) {
	t.Helper()
	published := map[string]bool{}
	for _, c := range m.Contributions {
		if c.Type == "service" {
			published[c.Provider+"/"+c.ID] = true
		}
	}
	var missing []string
	for _, ps := range application.UseAppExtensions[publish.PublishedService](app) {
		if !ps.HasCapability("ktsp-container") {
			continue // not a slot: infra, HTTP-only or agent-plugin-only publications offer nothing on the bus
		}
		id := ps.Name
		if ps.Definition != nil {
			if r := ps.Definition.ResolvedBusName(); r != "" {
				id = r
			}
		}
		provider := m.Provider
		if ps.Local {
			provider = contribapi.LocalProvider
		}
		if published[provider+"/"+id] {
			continue
		}
		why := "no service contribution carries it"
		if ps.Definition == nil {
			why = "published without a ServiceDef, so the services provider never emits it"
		}
		missing = append(missing, fmt.Sprintf("  slot %s (service %s): %s", id, ps.Name, why))
	}
	if len(missing) == 0 {
		return
	}
	sort.Strings(missing)
	t.Errorf("census[%s]: %d KTSP slot(s) are callable but not published as a service contribution (G21):\n%s\n\n"+
		"Give the service a ServiceDef (publish.WithServiceDef) so it is advertised where everything else is, "+
		"or drop its KTSP container if it offers nothing.",
		name, len(missing), strings.Join(missing, "\n"))
}
