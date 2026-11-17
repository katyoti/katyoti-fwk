// SPDX-License-Identifier: Apache-2.0

package cwmapp

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A one-shot CLI write is followed at once by CloseApplicationScope. The
// in-process observers (the journal, the planification, the feedback timer)
// have not handled the write's event yet; they must still record it — the
// close DRAINS them while the application's services are open — and the close
// must stay prompt.
func TestCloseApplicationScopeLetsObserversRecordTheLastWrite(t *testing.T) {
	recorded := make(chan string, 8)
	observer := cwmapi.ObserveEvents(cwmapi.EventObserver{
		Name: "close-drain-probe",
		Handle: func(ctx context.Context, ev cwmapi.EventEnvelope) {
			if !strings.HasSuffix(ev.Topic, ".created") {
				return
			}
			// The write's process is already closing when this runs.
			time.Sleep(100 * time.Millisecond)
			// Like the journal: resolve a service, call CWM back, record.
			cwm, err := application.UseService[cwmapi.CWMService](ctx)
			if err != nil {
				recorded <- "resolve failed: " + err.Error()
				return
			}
			if _, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{}); err != nil {
				recorded <- "call back failed: " + err.Error()
				return
			}
			recorded <- ev.Topic
		},
	})
	ctx, cwm := New(t,
		// The in-process bus CWM publishes on (a host binary registers it).
		application.DeclareSimpleService[eventbus.EventBus](func(context.Context) (eventbus.EventBus, error) {
			return eventbus.NewMemoryEventBus(), nil
		}),
		observer, Pack(fstest.MapFS{
			"capabilities/container.yaml": {Data: []byte("name: container\n")},
			"object-types/project.yaml":   {Data: []byte("name: project\ncapabilities: [container]\n")},
		}))
	p := cwmapi.Object{ID: "demo", Type: cwmapi.CapProject, Name: "demo"}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "demo", Object: p}); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	appcore.CloseApplicationScope(ctx) // what ExecuteCLI does as the command returns
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("CloseApplicationScope took %v, want well under a second past the handler", d)
	}
	select {
	case got := <-recorded:
		if !strings.HasSuffix(got, ".created") {
			t.Fatalf("the observer did not record the last write: %s", got)
		}
	default:
		t.Fatal("the observer never handled the last write before the close returned")
	}
}
