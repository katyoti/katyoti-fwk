// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/redact"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// A RUN'S TRANSITIONS LEAVE THE PROCESS — each is published, redacted, on its
// universe's RunLifecycleSubject, so a GUI (another process) refreshes its
// Runs pane on them. Best effort: no bus (a CLI run, a test), nothing leaves.

// lifecycleBridge publishes a run's transitions on the universe's bus.
type lifecycleBridge struct {
	once     sync.Once
	bus      mbapi.MessageBus
	subject  string
	redactor *redact.Redactor
}

// resolve finds the bus and the subject once (the bus may be registered after
// the task service is built).
func (b *lifecycleBridge) resolve(ctx context.Context) {
	b.once.Do(func() {
		if ctx == nil {
			return
		}
		bus, err := application.UseService[mbapi.MessageBus](ctx)
		if err != nil || bus == nil {
			return
		}
		env := application.Env()
		if env == nil {
			return
		}
		tenant, universe := cenv.ResolveTenantUniverse(env.HomeDir())
		if tenant == "" || universe == "" {
			return
		}
		settings, _ := application.UseConfig[redact.RedactionSettings](ctx)
		b.bus, b.subject, b.redactor = bus, subjects.RunLifecycleSubject(tenant, universe), redact.New(settings)
	})
}

// publish sends one transition (no-op without a bus).
func (b *lifecycleBridge) publish(ctx context.Context, evt *engine.RunLifecycleEvent) {
	b.resolve(ctx)
	if b.bus == nil {
		return
	}
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	data = b.redactor.RedactBytes(data) // reasons can carry secrets
	pctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = b.bus.Publish(pctx, b.subject, data) // best effort: the journal and the in-process event have it
}
