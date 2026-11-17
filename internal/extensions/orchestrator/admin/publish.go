// SPDX-License-Identifier: Apache-2.0

package admin

import (
	"context"
	"encoding/json"
	"log"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// PublishEvent announces a control-plane change on the admin universe:
// subjectFor is the subject builder (subjects.UniversesChangedSubject,
// subjects.ProfileChangedSubject, ...), payload is JSON-encoded.
//
// The change is published on admin because control-plane state belongs to no
// one universe, and every GUI or CLI is always connected to admin.
//
// Two kinds of process make these changes, and only one has a bus:
//
//   - Inside a hub (the admin core, ktw-univers): the MessageBus is a
//     registered service, so the event goes straight onto it.
//   - Inside the katyoti CLI running an engine in-process: no bus at all, so
//     the admin hub's is borrowed, connecting in ModeReuse. ModeReuse never
//     spawns: with no hub running there is no GUI to notify, and the call
//     costs one lockfile stat.
//
// BEST-EFFORT BY DESIGN. The change is already persisted and the caller has
// already been told it succeeded; a bus that is absent or refuses the publish
// must never turn a completed change into a reported failure. A dropped event
// costs a GUI that refreshes on its next read.
func PublishEvent(ctx context.Context, subjectFor func(tenantID, universeID string) string, payload any) {
	tenant, _ := cenv.ResolveTenantUniverse("")
	if tenant == "" {
		tenant = "local"
	}
	subject := subjectFor(tenant, UniverseName)
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	if bus, err := application.UseService[mbapi.MessageBus](ctx); err == nil && bus != nil {
		publish(ctx, bus, subject, data)
		return
	}
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{
		DataDir: application.EnvOf(Home()).DataDir(),
		Mode:    hubclient.ModeReuse,
	})
	if err != nil {
		return // no hub running == nobody to notify
	}
	defer func() { _ = hc.Close() }()
	publish(ctx, hc.MessageBus(), subject, data)
}

func publish(ctx context.Context, bus mbapi.MessageBus, subject string, data []byte) {
	if err := bus.Publish(ctx, subject, data); err != nil {
		log.Printf("[admin] publish %s: %v -- GUIs will refresh on their next read", subject, err)
	}
}
