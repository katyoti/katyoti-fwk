// SPDX-License-Identifier: Apache-2.0

package cwmapp

import (
	"context"
	"encoding/json"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// bootProbe is the ActionService a bootstrap hook runs through: like the
// in-process dispatch, the action resolves CWM from the context it is handed.
type bootProbe struct{ resolved chan error }

func (b *bootProbe) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (b *bootProbe) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	_, err := application.UseService[cwmapi.CWMService](ctx)
	b.resolved <- err
	return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{}`)}, err
}

// A bootstrap hook calls CWM back — the inbox and the planification create
// their system modules that way. It is run once CWM is built, on a context that
// does not carry CWM's own construction: resolving CWM there is not a cycle.
func TestABootstrapHookReachesTheFinishedService(t *testing.T) {
	root := t.TempDir()
	probe := &bootProbe{resolved: make(chan error, 4)}
	exts := []application.Extension{
		Pack(fstest.MapFS{
			"capabilities/container.yaml":  {Data: []byte("name: container\n")},
			"capabilities/boot-probe.yaml": {Data: []byte("name: boot-probe\nattach:\n  types: [project]\nhooks:\n  bootstrap:\n    - uri: x:probe/ensure\n")},
			"object-types/project.yaml":    {Data: []byte("name: project\ncapabilities: [project, container]\n")},
		}),
		func(init *application.ApplicationInit) {
			init.AddContribute("cwmapp", ".probe", func(app *application.Application) error {
				application.PutService[actionsapi.ActionService](app, "default", probe)
				return nil
			})
			// The serving hooks, as a serving process has them: the hook
			// another sidecar serves (x:probe/ensure) runs once it serves.
			init.AddRegisterPoints("cwmapp", ".serving", func(app *application.Application) error {
				return application.RegisterPoint(app, em.NewListPoint[ktsapi.ServingHook]())
			})
		},
	}
	ctx, cwm := NewAt(t, root, exts...)
	p := cwmapi.Object{ID: "demo", Type: cwmapi.CapProject, Name: "demo"}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "demo", Object: p}); err != nil {
		t.Fatal(err)
	}
	for len(probe.resolved) > 0 {
		<-probe.resolved
	}
	appcore.CloseApplicationScope(ctx) // releases the store

	// The restart: the project is there; once the process serves, its
	// bootstrap runs.
	ctx, _ = NewAt(t, root, exts...)
	for _, h := range application.UseExtensions[ktsapi.ServingHook](ctx) {
		if h.Name == "cwm-bootstrap" {
			go h.Start(ctx)
		}
	}
	select {
	case err := <-probe.resolved:
		if err != nil {
			t.Fatalf("the bootstrap hook could not reach CWM: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the bootstrap hook never ran")
	}
}
