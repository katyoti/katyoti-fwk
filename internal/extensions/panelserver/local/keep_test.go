// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
)

// writesCWM records the file writes and edges the keeper makes. Every other
// method is the embedded nil interface: the keeper calling one is a contract
// change worth a failing test.
type writesCWM struct {
	cwmapi.CWMService

	mu     sync.Mutex
	writes []cwmapi.ObjectFileWriteInput
	edges  []cwmapi.EdgeCreateInput
}

func (c *writesCWM) ObjectFileWrite(_ context.Context, in cwmapi.ObjectFileWriteInput) (cwmapi.ObjectFileWriteOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.writes = append(c.writes, in)
	module := strings.SplitN(in.ID, ".", 2)[0]
	return cwmapi.ObjectFileWriteOutput{Path: in.Path, ProjectPath: "modules/" + module + "/" + in.Path, SizeBytes: int64(len(in.Content))}, nil
}

func (c *writesCWM) EdgeCreate(_ context.Context, in cwmapi.EdgeCreateInput) (cwmapi.EdgeCreateOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.edges = append(c.edges, in)
	return cwmapi.EdgeCreateOutput{Edge: cwmapi.Edge{From: in.From, To: in.To, Kind: in.Kind}}, nil
}

// The hub's keeper writes a kept panel's outcome through CWM as a document
// in its object's module and attaches it to the object.
func TestTheHubKeeperWritesTheOutcomeOnTheObjectThroughCWM(t *testing.T) {
	cwm := &writesCWM{}
	app := application.New(
		config.NewConfigService(loader.NewMapLoader(nil)),
		func(init *application.ApplicationInit) {
			init.AddContribute("test", ".cwm", func(app *application.Application) error {
				application.PutService[cwmapi.CWMService](app, "default", cwm)
				return nil
			})
		},
	)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)

	id := panel.NewID()
	o := panelapi.Outcome{Keep: panel.KeepOutcome, Snapshot: panelapi.Snapshot{
		Panel: id, Seq: 2, State: panel.StateComplete, Title: "Publication", Object: "wiki.F-012",
		Ended:  time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
		Events: []panel.Event{{Panel: id, Seq: 2, Kind: panel.KindComplete, Complete: &panel.Complete{Status: "ok"}}},
	}}
	path, err := keepOnObject(ctx, "wiki.F-012", o)
	if err != nil {
		t.Fatal(err)
	}
	if path != "panels/"+id+".md" {
		t.Fatalf("kept at %q, want panels/%s.md", path, id)
	}
	if len(cwm.writes) != 1 || cwm.writes[0].ID != "wiki.F-012" || cwm.writes[0].Path != path ||
		!strings.Contains(string(cwm.writes[0].Content), "# Publication") {
		t.Fatalf("writes %+v, want the one document in the object's module", cwm.writes)
	}
	want := cwmapi.EdgeCreateInput{From: "wiki.F-012", To: "modules/wiki/" + path, Kind: cwmapi.EdgeKindAttaches}
	if len(cwm.edges) != 1 || cwm.edges[0] != want {
		t.Fatalf("edges %+v, want %+v", cwm.edges, want)
	}
}
