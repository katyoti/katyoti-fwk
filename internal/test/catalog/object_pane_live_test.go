// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// object_pane_live_test.go — the object pane, served for real.
//
// A real hub and real ktw-agents + ktw-methods binaries: a feature is created,
// the pane read (object-pane/Detail, TransitionOptions), moved along its
// ladder, refused at the ready-to-build gate, the gate's person requirement
// confirmed (gates/Confirm) and the move made — and its transition event
// arrives on NATS, where another process would hear it. What the in-process
// tests cannot show: the services answer over the bus, under their real paths,
// in the binaries that ship.
//
// Skips cleanly when the binaries are not built; fails under KTW_E2E.

const paneUniverse = "object-pane-live"

func TestObjectPaneLive(t *testing.T) {
	var sidecars []hubharness.SidecarBin
	for _, name := range []string{"ktw-agents", "ktw-methods"} {
		bin := binaryFor(t, name)
		if bin == "" {
			hubharness.MissingBinary(t, "object pane live: sidecar binary %s not built", name)
			return
		}
		sidecars = append(sidecars, hubharness.SidecarBin{Name: name, Binary: bin})
	}
	ws := t.TempDir()
	cfgDir := filepath.Join(ws, ".katyoti", "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf("version: 1\nname: %s\ndisplay_name: Object Pane Live\nicon: T\ncolor: gray\nsidecars:", paneUniverse)
	// nats: each member subscribes its slots on the bus, as in a real universe.
	for _, sc := range sidecars {
		manifest += fmt.Sprintf("\n  - name: %s\n    transport: nats\n    binary: %s", sc.Name, filepath.ToSlash(sc.Binary))
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "Univers.yaml"), []byte(manifest+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "Settings.yaml"), []byte("# object pane live\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KTW_CACHE", t.TempDir())
	t.Setenv("KTW_WORKSPACE", ws)
	dataDir := t.TempDir()
	hubharness.Start(t, dataDir)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: dataDir, Mode: hubclient.ModeReuse})
	if err != nil {
		t.Fatalf("connect to hub: %v", err)
	}
	t.Cleanup(func() { hc.Close() })
	// The GUI's sender: a sidecar not yet running is spawned on first use.
	s := messagebus.NewRetrySender(messagebus.NewSender(hc.MessageBus(), tenantID, paneUniverse), "")
	call := func(path string, in, out any) error {
		raw, err := s.Call(ctx, path, in)
		if err != nil {
			return err
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(raw, out)
	}
	ana := cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}

	// The feature's transition is heard on NATS, as another process hears it.
	// CWM publishes under the workspace id it was given — none here, so its
	// default.
	const feature = "pane.f1"
	heard := make(chan string, 8)
	subject := cwmapi.ObjectTransitionedSubject(tenantID, paneUniverse, "unbootstrapped", cwmapi.SubjectToken(feature))
	sub, err := hc.MessageBus().Subscribe(ctx, subject, func(_ context.Context, m mbapi.Message) { heard <- string(m.Data()) })
	if err != nil {
		t.Fatalf("subscribe %s: %v", subject, err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	// The module - documentation, the module type ktw-methods ships - and its
	// feature. The first call may meet the sidecar still starting: retried for
	// a while.
	module := cwmapi.Object{ID: "pane", Type: "documentation", Name: "pane"}
	deadline := time.Now().Add(90 * time.Second)
	for {
		err = call("ktw-agents:cwm/ObjectCreate", cwmapi.ObjectCreateInput{Module: "pane", Object: module}, nil)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		t.Fatalf("create the module: %v", err)
	}
	f := cwmapi.Object{ID: feature, Type: "feature", Name: "Live pane", Parent: "pane"}
	if err := call("ktw-agents:cwm/ObjectCreate", cwmapi.ObjectCreateInput{Module: "pane", Object: f}, nil); err != nil {
		t.Fatalf("create the feature: %v", err)
	}

	// The pane, as the GUI asks for it.
	var detail paneapi.DetailOutput
	if err := call("ktw-agents:object-pane/Detail", paneapi.DetailInput{ID: feature, Body: true}, &detail); err != nil {
		t.Fatalf("object-pane/Detail: %v", err)
	}
	if detail.Object.ID != feature || detail.Object.Type != "feature" {
		t.Fatalf("Detail answered %s (%s), want %s (feature)", detail.Object.ID, detail.Object.Type, feature)
	}
	state := func() (string, []paneapi.LifecycleOptions) {
		t.Helper()
		var o paneapi.TransitionOptionsOutput
		if err := call("ktw-agents:object-pane/TransitionOptions", paneapi.TransitionOptionsInput{ID: feature, By: ana}, &o); err != nil {
			t.Fatalf("object-pane/TransitionOptions: %v", err)
		}
		if len(o.Lifecycles) == 0 {
			t.Fatal("TransitionOptions: no lifecycle for a feature")
		}
		return o.Lifecycles[0].State, o.Lifecycles
	}
	if st, _ := state(); st != "idea" {
		t.Fatalf("a new feature is in %q, want idea", st)
	}

	move := func(to string, force bool) error {
		in := cwmapi.ObjectTransitionInput{ID: feature, To: to, By: ana, Force: force}
		if force {
			in.Note = "the live test forces this step"
		}
		return call("ktw-agents:cwm/ObjectTransition", in, nil)
	}
	for _, to := range []string{"proposed", "in-review", "accepted"} {
		if err := move(to, false); err != nil {
			t.Fatalf("move to %s: %v", to, err)
		}
	}
	if err := move("specified", true); err != nil {
		t.Fatalf("forced move to specified: %v", err)
	}
	// ready-to-build wants a person's challenge: refused until confirmed.
	if err := move("ready", false); err == nil {
		t.Fatal("ready passed before its person requirement was confirmed")
	}
	if err := call("ktw-agents:gates/Confirm", gatesapi.ConfirmInput{ID: feature, Gate: "ready-to-build", Requirement: "feature-challenged", By: ana, Note: "challenged live"}, nil); err != nil {
		t.Fatalf("gates/Confirm: %v", err)
	}
	if err := move("ready", false); err != nil {
		t.Fatalf("move to ready after the confirmation: %v", err)
	}
	if st, _ := state(); st != "ready" {
		t.Fatalf("the pane shows %q after the move, want ready", st)
	}

	// Every transition was heard on NATS; the last one says ready.
	var last string
	timeout := time.After(10 * time.Second)
	for got := 0; got < 5; got++ {
		select {
		case last = <-heard:
		case <-timeout:
			t.Fatalf("NATS carried %d of the 5 transitions (subject %s)", got, subject)
		}
	}
	var ev map[string]any
	_ = json.Unmarshal([]byte(last), &ev)
	if fmt.Sprint(ev["to"]) != "ready" && fmt.Sprint(ev["state"]) != "ready" {
		t.Errorf("the last transition event = %s, want it to say ready", last)
	}
}
