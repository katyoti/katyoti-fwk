// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// Substrate-level test for the (KTSPNamedInstancesCap → Endpoint.AddSlot(slot, instance))
// path. Publishes a fake EchoService with three named impls, runs
// AutoKTSPContainer, then dispatches via the produced Endpoint to confirm
// each (slot, instance) pair routes to its own impl.

// echoIn / echoOut + EchoService are the minimal reflectable contract
// AutoKTSPContainer / BuildKTSPSlotWithTags need: one input struct, one
// output struct, one method with (ctx, in) → (out, error) signature.
type echoIn struct {
	Msg string `json:"msg" desc:"Anything"`
}

type echoOut struct {
	Reply string `json:"reply"`
	Which string `json:"which"`
}

type echoSvc interface {
	Which(ctx context.Context, in echoIn) (echoOut, error)
}

type echoImpl struct{ name string }

func (e *echoImpl) Which(_ context.Context, in echoIn) (echoOut, error) {
	return echoOut{Reply: in.Msg, Which: e.name}, nil
}

// echoServiceDef is the meta.ServiceDef the publish layer needs to
// populate the cap's SlotName / Methods. The slot wire-name is
// "echo-svc" — that becomes the slottype token in the bus subject.
var echoServiceDef = meta.ServiceDef{
	Name:    "echo-svc",
	Short:   "Test echo service for named-instance dispatch",
	Version: "0.0.1",
	Methods: map[string]meta.MethodDef{
		"Which": {Short: "Identify which impl answered"},
	},
}

// echoPublishExtension hand-rolls a publish.PublishedService entry — the
// generic PublishService[T] helper insists on a service factory it can
// register in SM, but for the named-instances-only path we don't need a
// default SM service. Contributing the PublishedService directly is
// simpler than wrapping a no-op factory.
func echoPublishExtension(provider func(app *application.Application) (map[string]any, error)) application.Extension {
	ps := publish.PublishedService{
		Name:        "EchoService",
		Description: echoServiceDef.Description(),
		Version:     echoServiceDef.Version,
		ServiceType: reflect.TypeOf((*echoSvc)(nil)).Elem(),
		Definition:  &echoServiceDef,
	}
	// Reuse WithKTSPNamedInstances so the same code path the production
	// CRUD wiring will use gets exercised end-to-end.
	publish.WithKTSPNamedInstances(provider)(&ps)

	return func(init *application.ApplicationInit) {
		init.AddRegisterPoints("echo-test", ".points", func(app *application.Application) error {
			if app.PointInfo(em.PointID[publish.PublishedService]()) == nil {
				return application.RegisterPoint(app, publish.NewPublishedServicesPoint())
			}
			return nil
		})
		init.AddContribute("echo-test", ".published", func(app *application.Application) error {
			return application.Contribute(app, ps)
		})
	}
}

func TestAutoKTSP_NamedInstancesDispatch(t *testing.T) {
	alice := &echoImpl{name: "alice-impl"}
	bob := &echoImpl{name: "bob-impl"}
	carol := &echoImpl{name: "carol-impl"}

	provider := func(_ *application.Application) (map[string]any, error) {
		return map[string]any{
			"alice": alice,
			"bob":   bob,
			"carol": carol,
		}, nil
	}

	cfgSvc := config.NewConfigService(loader.NewMapLoader(nil))
	app := application.New(
		cfgSvc,
		echoPublishExtension(provider),
		AutoKTSPContainer("test-app", "named-instances substrate test", "0.0.1"),
	)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}

	ep, err := application.UseService[*ktsp.Endpoint](app.Context())
	if err != nil {
		t.Fatalf("UseService[*ktsp.Endpoint]: %v", err)
	}

	// Sanity: Instances() should enumerate exactly what we registered.
	got := ep.Instances("echo-svc")
	want := []string{"alice", "bob", "carol"}
	if len(got) != len(want) {
		t.Fatalf("Instances(echo-svc) = %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("Instances[%d] = %q, want %q", i, got[i], name)
		}
	}

	// Each (slot, instance) routes to its own impl. The reflection-built
	// method name is the kebab-case of "Which" → "which".
	cases := []struct {
		instance string
		want     string
	}{
		{"alice", "alice-impl"},
		{"bob", "bob-impl"},
		{"carol", "carol-impl"},
	}
	for _, tc := range cases {
		params, _ := json.Marshal(echoIn{Msg: "hi"})
		resp, err := ep.Handle(context.Background(), "echo-svc", tc.instance, "which", params)
		if err != nil {
			t.Fatalf("instance %q: Handle: %v", tc.instance, err)
		}
		var out echoOut
		if err := json.Unmarshal(resp, &out); err != nil {
			t.Fatalf("instance %q: decode: %v (raw=%s)", tc.instance, err, string(resp))
		}
		if out.Which != tc.want {
			t.Fatalf("instance %q: Which = %q, want %q", tc.instance, out.Which, tc.want)
		}
		if out.Reply != "hi" {
			t.Fatalf("instance %q: Reply = %q, want %q", tc.instance, out.Reply, "hi")
		}
	}

	// Missing instance returns ErrSlotNotFound with the slot(instance) form.
	_, err = ep.Handle(context.Background(), "echo-svc", "ghost", "which",
		mustJSON(t, echoIn{Msg: "?"}))
	if err == nil {
		t.Fatal("expected error for missing instance, got nil")
	}
	if rpc, ok := err.(*ktsp.RPCError); ok {
		if rpc.Code != ktsp.CodeSlotNotFound {
			t.Fatalf("missing instance: code = %d, want %d", rpc.Code, ktsp.CodeSlotNotFound)
		}
	} else {
		t.Fatalf("missing instance: error = %v (%T), want *ktsp.RPCError", err, err)
	}
}

// TestAutoKTSP_NamedInstancesComposedWithDefault confirms that the
// default-instance (KTSPContainerCap) binding from the SM-registered
// service AND the named-instance bindings from the provider coexist
// on the same Endpoint.
//
// Skipped for now — exercising both caps in the same PublishedService
// entry requires a factory the publish framework registers in SM, and
// our minimal echoSvc has no real factory. The realnats e2e
// (TestRealNATS_NamedInstanceDispatch) already proves the default
// + named composition over the wire using a properly registered
// service, so the substrate path is covered there.
func TestAutoKTSP_NamedInstancesComposedWithDefault(t *testing.T) {
	t.Skip("default + named composition is covered by TestRealNATS_NamedInstanceDispatch")
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustJSON: %v", err)
	}
	return b
}
