// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// echoService is a published service of the binary under test.
type echoService struct{}

type sayInput struct {
	Text string `json:"text"`
}

type sayOutput struct {
	Said string `json:"said"`
}

func (echoService) SayBack(_ context.Context, in sayInput) (sayOutput, error) {
	return sayOutput{Said: "you said " + in.Text}, nil
}

// A method this binary publishes runs in-process — no hub needed; a method of
// another binary still goes to the hub (here: none runs, so it fails there).
func TestActionService_Run_OwnMethodRunsInProcess(t *testing.T) {
	app := application.New(application.Config())
	app.SetProvider("ktw-test")
	endpoint := ktsp.NewEndpoint("ktw-test", "", "0")
	endpoint.AddSlot(ktslocal.BuildKTSPSlot("echo", ktsp.DefaultInstance, "", echoService{}))
	application.PutService[*ktsp.Endpoint](app, "default", endpoint)
	ctx := application.WithApplication(context.Background(), app)

	svc, err := New(ctx, &Config{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.Run(ctx, actionsapi.RunInput{URI: "ktw-test:echo/say-back", Input: json.RawMessage(`{"text":"hello"}`)})
	if err != nil {
		t.Fatalf("own method: %v, want it run in-process", err)
	}
	var got sayOutput
	if err := json.Unmarshal(out.Value, &got); err != nil || got.Said != "you said hello" {
		t.Fatalf("own method answered %s, %v", out.Value, err)
	}
	if _, err := svc.Run(ctx, actionsapi.RunInput{URI: "ktw-other:echo/say-back"}); err == nil || strings.Contains(err.Error(), "in-process") {
		t.Errorf("another binary's method: %v, want the hub path (and no hub here)", err)
	}
}

// A lone process has no catalog: whether its own method only reads comes from
// its own endpoint — a guard's expression may call only read-only methods.
func TestActionService_Describe_OwnMethodIsReadOnly(t *testing.T) {
	app := application.New(application.Config())
	app.SetProvider("ktw-test")
	endpoint := ktsp.NewEndpoint("ktw-test", "", "0")
	endpoint.AddSlot(ktslocal.BuildKTSPSlotWithTags("echo", ktsp.DefaultInstance, "", echoService{}, nil,
		map[string]meta.MethodDef{"SayBack": {Short: "says it back", ReadOnly: true}}))
	endpoint.AddSlot(ktslocal.BuildKTSPSlot("loud", ktsp.DefaultInstance, "", echoService{}))
	application.PutService[*ktsp.Endpoint](app, "default", endpoint)
	ctx := application.WithApplication(context.Background(), app)

	svc, err := New(ctx, &Config{})
	if err != nil {
		t.Fatal(err)
	}
	d, err := svc.Describe(ctx, actionsapi.DescribeInput{URI: "ktw-test:echo/say-back"})
	if err != nil || !d.ReadOnly {
		t.Fatalf("own read-only method: %+v, %v; want ReadOnly", d, err)
	}
	// Its form: the input schema and description it declares, with no catalog.
	if !strings.Contains(string(d.InputSchema), `"text"`) || d.Description != "says it back" {
		t.Errorf("own method described as %q with schema %s; want its declaration (the text field)", d.Description, d.InputSchema)
	}
	if d, err := svc.Describe(ctx, actionsapi.DescribeInput{URI: "ktw-test:loud/say-back"}); err != nil || d.ReadOnly {
		t.Fatalf("own method with no ReadOnly: %+v, %v; want not read-only", d, err)
	}
}

// A binary never sends its own slots over the bus: a slot of its own that its
// endpoint does not hold (not served, or not published yet — the endpoint is
// built at the end of AppInit) is an error, not a call the hub would answer by
// spawning another member of this binary.
func TestActionService_Run_OwnSlotNeverGoesToTheBus(t *testing.T) {
	app := application.New(application.Config())
	app.SetProvider("ktw-test")
	ctx := application.WithApplication(context.Background(), app)

	dialed := false
	svc := &Service{app: app, dial: func(context.Context) (ktsp.Sender, error) {
		dialed = true
		return nil, errors.New("the bus was used")
	}}
	// No endpoint at all (the binary is still booting), then one without the slot.
	_, err := svc.Run(ctx, actionsapi.RunInput{URI: "ktw-test:echo/say-back"})
	if err == nil || dialed || !strings.Contains(err.Error(), "never go to the bus") {
		t.Fatalf("own slot, no endpoint yet: err=%v dialed=%v; want an error and no bus", err, dialed)
	}
	endpoint := ktsp.NewEndpoint("ktw-test", "", "0")
	endpoint.AddSlot(ktslocal.BuildKTSPSlot("loud", ktsp.DefaultInstance, "", echoService{}))
	application.PutService[*ktsp.Endpoint](app, "default", endpoint)
	if _, err := svc.Run(ctx, actionsapi.RunInput{URI: "ktw-test:echo/say-back"}); err == nil || dialed {
		t.Fatalf("own slot not on the endpoint: err=%v dialed=%v; want an error and no bus", err, dialed)
	}
	// Another binary's slot still goes to the bus.
	if _, err := svc.Run(ctx, actionsapi.RunInput{URI: "ktw-other:echo/say-back"}); err == nil || !dialed {
		t.Fatalf("another binary's slot: err=%v dialed=%v; want the bus", err, dialed)
	}
}
