// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// oneManifestCatalog answers Get with one manifest; any other Catalog method
// panics (the embedded interface is nil), so the test names what it touched.
type oneManifestCatalog struct {
	mbapi.Catalog
	man *mbapi.Manifest
}

func (c oneManifestCatalog) Get(_ context.Context, name string) (*mbapi.Manifest, error) {
	if name != c.man.Provider {
		return nil, mbapi.ErrSidecarNotInCatalog
	}
	return c.man, nil
}

// auditSpy counts the invocations journaled; any other journal call panics.
type auditSpy struct {
	journalapi.JournalService
	created int
}

func (s *auditSpy) Invoked(context.Context, journalapi.InvokedInput) (journalapi.InvokedOutput, error) {
	s.created++
	return journalapi.InvokedOutput{}, nil
}

// readOnlyApp publishes echo/say-back in-process and a manifest describing it,
// read-only or not.
func readOnlyApp(t *testing.T, readOnly bool) (context.Context, *auditSpy) {
	t.Helper()
	app := application.New(application.Config())
	app.SetProvider("ktw-test")
	endpoint := ktsp.NewEndpoint("ktw-test", "", "0")
	endpoint.AddSlot(ktslocal.BuildKTSPSlot("echo", ktsp.DefaultInstance, "", echoService{}))
	application.PutService[*ktsp.Endpoint](app, "default", endpoint)
	m := mbapi.SlotMethod{Name: "SayBack"}
	if readOnly {
		m.Tags = []string{ktsp.TagMethodReadOnly}
	}
	application.PutService[mbapi.Catalog](app, "default", oneManifestCatalog{man: &mbapi.Manifest{
		Provider:  "ktw-test",
		SlotTypes: []mbapi.SlotType{{Type: "echo", Methods: []mbapi.SlotMethod{m}}},
	}})
	spy := &auditSpy{}
	application.PutService[journalapi.JournalService](app, "default", spy)
	return application.WithApplication(context.Background(), app), spy
}

// A read-only method is reported as such, and running it — even aimed at a
// node — writes no invocation record. The same method without the tag records
// once: the control that proves the spy sees the audit path.
func TestActionService_ReadOnlyMethodIsDescribedAndLeavesNoRecord(t *testing.T) {
	input := json.RawMessage(`{"text":"hi","node_id":"some.node"}`)
	for _, tc := range []struct {
		readOnly    bool
		wantRecords int
	}{{true, 0}, {false, 1}} {
		ctx, spy := readOnlyApp(t, tc.readOnly)
		svc, err := New(ctx, &Config{})
		if err != nil {
			t.Fatal(err)
		}
		d, err := svc.Describe(ctx, actionsapi.DescribeInput{URI: "ktw-test:echo/say-back"})
		if err != nil {
			t.Fatal(err)
		}
		if d.ReadOnly != tc.readOnly {
			t.Errorf("readOnly=%v: Describe.ReadOnly = %v", tc.readOnly, d.ReadOnly)
		}
		if _, err := svc.Run(ctx, actionsapi.RunInput{URI: "ktw-test:echo/say-back", Input: input}); err != nil {
			t.Fatalf("readOnly=%v: Run: %v", tc.readOnly, err)
		}
		if spy.created != tc.wantRecords {
			t.Errorf("readOnly=%v: %d invocation records, want %d", tc.readOnly, spy.created, tc.wantRecords)
		}
	}
}
