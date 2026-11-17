// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
)

// reportingEndpoint serves hello/report: it reports twice with
// progress.Emit, knowing nothing of panels.
func reportingEndpoint() *ktsp.Endpoint {
	ep := ktsp.NewEndpoint("test", "", "")
	slot := ktsp.NewSlot("hello", ktsp.DefaultInstance, "", nil)
	slot.Add(ktsp.RawMethod("report", ktsp.MethodOpts{},
		func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			progress.Emit(ctx, panel.Progress{Label: "steps", Current: 1, Total: 2})
			progress.Emit(ctx, panel.Progress{Label: "steps", Current: 2, Total: 2})
			return json.RawMessage(`"done"`), nil
		}))
	ep.AddSlot(slot)
	return ep
}

// assertALentPanelCrossesTheCall: the caller lends its panel to a call; the
// method's reports go out numbered after the caller's open, and the caller's
// next event follows the method's last one.
func assertALentPanelCrossesTheCall(t *testing.T, bus mbapi.MessageBus) {
	t.Helper()
	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, reportingEndpoint())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()

	var mu sync.Mutex
	var seqs []int64
	id := panel.NewID()
	sub, err := bus.Subscribe(context.Background(), subjects.PanelOutSubject(testTenant, testUniverse, id), func(_ context.Context, m mbapi.Message) {
		if e, err := panel.DecodeEvent(m.Data()); err == nil {
			mu.Lock()
			seqs = append(seqs, e.Seq)
			mu.Unlock()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	ctx := context.Background()
	h, err := panel.OpenPanelWithID(ctx, panel.Scope{Tenant: testTenant, Universe: testUniverse}, func(ctx context.Context, s string, p []byte) error {
		return bus.Publish(ctx, s, p)
	}, id, panel.Open{Type: "ktw-test:/panel-type/progress"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSender(bus, testTenant, testUniverse).Call(panel.WithHandle(ctx, h), testProvider+":/hello/report", nil); err != nil {
		t.Fatal(err)
	}
	if h.Seq() != 3 {
		t.Fatalf("after the call the caller's panel is at %d, want 3: the method's last sequence did not come back", h.Seq())
	}
	if err := h.Complete(ctx, "ok", nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(seqs)
		mu.Unlock()
		if n >= 4 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })
	if len(seqs) != 4 || seqs[0] != 1 || seqs[1] != 2 || seqs[2] != 3 || seqs[3] != 4 {
		t.Fatalf("the panel's events are numbered %v, want [1 2 3 4]: the open, the method's two reports, the complete", seqs)
	}
}

// TestALentPanelCrossesTheCall_RealNATS: over a real NATS server.
func TestALentPanelCrossesTheCall_RealNATS(t *testing.T) {
	h := startEmbeddedHub(t)
	assertALentPanelCrossesTheCall(t, natsxport.New(h.Client))
}

// TestALentPanelCrossesTheCall_Stub: over the in-memory bus.
func TestALentPanelCrossesTheCall_Stub(t *testing.T) {
	assertALentPanelCrossesTheCall(t, teststub.New())
}
