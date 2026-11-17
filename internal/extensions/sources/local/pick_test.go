// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	eventscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/events/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// pickVersion is what the fake connector's items say: changing it changes
// them at their source.
var pickVersion = "v1"

// pickActions answers a connector's search, recording what it was asked.
type pickActions struct {
	mu      *sync.Mutex
	asked   *[]api.ConnectorSearchInput
	version *string
}

func (a pickActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a pickActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	if in.URI == "ktw-sp:sp/scopes" {
		// A scoped connector's configured spaces (scopedSpaces).
		return scopeActions{}.Run(ctx, in)
	}
	if in.URI == "ktw-agents:sources/source-check" {
		// Sources' save gate: the service in the process.
		svc, err := application.UseService[api.SourcesService](ctx)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		var cin api.SourceCheckInput
		if err := json.Unmarshal(in.Input, &cin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		got, err := svc.SourceCheck(ctx, cin)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		raw, _ := json.Marshal(got)
		return actionsapi.RunOutput{Value: raw}, nil
	}
	if in.URI == "ktw-apps:apps/source-test" {
		out := api.ConnectorTestOutput{OK: true, Message: "the map answers"}
		if *a.version == "unconfigured" {
			out = api.ConnectorTestOutput{Message: "APPS_TOKEN is not set", NotConfigured: true}
		}
		raw, _ := json.Marshal(out)
		return actionsapi.RunOutput{Value: raw}, nil
	}
	if in.URI == "ktw-apps:apps/source-sync" {
		// Every item picked is written and keyed, but BAD cannot be read
		// (nor FLAKY while the map is "flaky")
		// and a LOST one is named without its file, so it fails to anchor.
		var req api.ConnectorSyncInput
		if err := json.Unmarshal(in.Input, &req); err != nil {
			return actionsapi.RunOutput{}, err
		}
		out := api.ConnectorSyncOutput{Items: []api.ConnectorItem{}}
		for _, it := range api.SelectedItems(req.Selection) {
			if it.Key == "BAD" || (it.Key == "FLAKY" && *a.version == "flaky") {
				out.Errors = append(out.Errors, it.Key+": not found in the map")
				continue
			}
			name := it.Key + ".md"
			if strings.HasPrefix(it.Key, "LOST") {
				out.Items = append(out.Items, api.ConnectorItem{Path: name, URI: "apps://" + it.Key, Title: it.Title, Key: it.Key})
				continue
			}
			if err := os.WriteFile(filepath.Join(req.Dest, name), []byte("# "+it.Title+"\n\n"+*a.version+"\n"), 0o644); err != nil {
				return actionsapi.RunOutput{}, err
			}
			out.Items = append(out.Items, api.ConnectorItem{Path: name, URI: "apps://" + it.Key, Title: it.Title, Key: it.Key})
		}
		raw, _ := json.Marshal(out)
		return actionsapi.RunOutput{Value: raw}, nil
	}
	if in.URI == "ktw-apps:offers/propose" {
		// A proposal carrying every item role: slots
		// detail, why and declared badges.
		var req api.ConnectorProposeInput
		if err := json.Unmarshal(in.Input, &req); err != nil {
			return actionsapi.RunOutput{}, err
		}
		raw, _ := json.Marshal(api.ConnectorProposeOutput{Items: []api.ProposedItem{
			{PickItem: api.PickItem{Key: "O1", Title: "Facturation clients", Kind: "application", Detail: "Émet les factures", Attrs: map[string]string{"code": "FAC"}}, Why: "« " + strings.Join(req.Terms, " · ") + " » dans le nom"},
			{PickItem: api.PickItem{Key: "O2", Title: "Vieux stock", Kind: "application", Badges: []string{"end-of-life"}, Attrs: map[string]string{"code": "STK"}}, Why: "« stock » dans la description"},
		}})
		return actionsapi.RunOutput{Value: raw}, nil
	}
	if in.URI == "ktw-apps:apps/propose" {
		var req api.ConnectorProposeInput
		if err := json.Unmarshal(in.Input, &req); err != nil {
			return actionsapi.RunOutput{}, err
		}
		if req.Text == "boom" {
			return actionsapi.RunOutput{}, fmt.Errorf("the map is down")
		}
		prop := api.ConnectorProposeOutput{Items: []api.ProposedItem{
			{PickItem: api.PickItem{Key: "A1", Title: "Acme ERP"}, Why: "named in " + strings.Join(req.Terms, ", "), Score: 0.9},
		}}
		if req.Text == "partial" {
			prop.Errors = []string{"« Stock » : the map is slow"}
		}
		raw, _ := json.Marshal(prop)
		return actionsapi.RunOutput{Value: raw}, nil
	}
	var req api.ConnectorSearchInput
	if err := json.Unmarshal(in.Input, &req); err != nil {
		return actionsapi.RunOutput{}, err
	}
	a.mu.Lock()
	*a.asked = append(*a.asked, req)
	a.mu.Unlock()
	raw, _ := json.Marshal(api.ConnectorSearchOutput{Items: []api.PickItem{
		{Key: "A1", Title: "Acme ERP · " + req.Query, Badges: []string{"ai"},
			Path: []string{"Map"}, Detail: "RDY · Actif", Attrs: map[string]string{"code": "RDY"}, Modified: "2026-10-12T00:00:00Z"},
		{Key: "A2", Title: "Acme Stock", Parent: "A1"},
	}})
	return actionsapi.RunOutput{Value: raw}, nil
}

// : a connector declares its search, its picker and its propose
// method; the sources module lists the searchable connectors as generic
// pickers (a connector contributed tomorrow is one, with no other change) and
// runs a connector's search for a carrier, in its own source's location.
func TestConnectorsAreGenericPickers(t *testing.T) {
	ctx, cwm, svc, asked := pickWorld(t)
	pickersAndSearch(t, ctx, cwm, svc, asked)
}

func pickWorld(t *testing.T) (context.Context, cwmapi.CWMService, api.SourcesService, *[]api.ConnectorSearchInput) {
	t.Helper()
	return pickWorldWith(t, nil)
}

// pickWorldWith is pickWorld with more connector entries (file -> YAML).
func pickWorldWith(t *testing.T, more map[string]string) (context.Context, cwmapi.CWMService, api.SourcesService, *[]api.ConnectorSearchInput) {
	t.Helper()
	return pickWorldExts(t, more)
}

// pickWorldExts is pickWorldWith with more extensions (an event bus).
func pickWorldExts(t *testing.T, more map[string]string, exts ...application.Extension) (context.Context, cwmapi.CWMService, api.SourcesService, *[]api.ConnectorSearchInput) {
	t.Helper()
	root := t.TempDir()
	asked := &[]api.ConnectorSearchInput{}
	entries := fstest.MapFS{
		"connectors/apps.yaml": {Data: []byte(`name: apps
label: Applications
description: The applications of the map.
sync: ktw-apps:apps/source-sync
test: ktw-apps:apps/source-test
search: ktw-apps:apps/search
propose: ktw-apps:apps/propose
picker:
  unit: application
  layout: list
  badges: {ai: {label: IA, color: violet, description: Generated}}
  derived: its interfaces
configure: {label: Configure the map, hint: The map's key, keys: [APPS_TOKEN]}
`)},
		"connectors/drive.yaml": {Data: []byte("name: drive\ndescription: A drive folder.\nsync: ktw-drive:drive/source-sync\n")},
	}
	for f, body := range more {
		entries["connectors/"+f] = &fstest.MapFile{Data: []byte(body)}
	}
	foreign := loader.NewEmbedFSLoader(entries, ".")
	ctx, cwm := cwmapp.NewAt(t, root, append([]application.Extension{
		sourcescontrib.Pack("ktw-agents"),
		eventscontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-apps", foreign, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return pickActions{mu: &sync.Mutex{}, asked: asked, version: &pickVersion}, nil
		}),
	}, exts...)...)
	cwmapp.Module(t, ctx, cwm, "m")
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.KnowledgeCapability}); err != nil {
		t.Fatal(err)
	}
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc, asked
}

func pickersAndSearch(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, svc api.SourcesService, asked *[]api.ConnectorSearchInput) {
	t.Helper()
	pk, err := svc.Pickers(ctx, api.PickersInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	// Every connector that picks: apps by its search
	// Documents and the local folder by browsing - with no search
	// -; drive only syncs.
	if len(pk.Connectors) != 3 || pk.Connectors[0].Name != "apps" || pk.Connectors[1].Name != "documents" || pk.Connectors[2].Name != "local-folder" ||
		pk.Connectors[2].Search != "" || pk.Connectors[2].Browse != api.SourceBrowseURI {
		t.Fatalf("pickers: %+v; want the connectors that search or browse, a search only where declared", pk.Connectors)
	}
	p := pk.Connectors[0]
	if p.Label != "Applications" || p.Search != api.SearchURI || p.Args["connector"] != "apps" || p.Unit != "application" ||
		p.Layout != "list" || p.Derived != "its interfaces" || p.Badges["ai"].Label != "IA" || !p.Propose {
		t.Errorf("the apps picker: %+v", p)
	}

	// No source of its own: the connector's whole reach.
	got, err := svc.Search(ctx, api.SearchInput{Connector: "apps", Object: "m", Query: "acme"})
	if err != nil || len(got.Items) != 2 || got.Items[1].Parent != "A1" || got.Items[0].Badges[0] != "ai" {
		t.Fatalf("search: %v %+v", err, got)
	}
	if len(*asked) != 1 || (*asked)[0].Query != "acme" || len((*asked)[0].Location) != 0 {
		t.Fatalf("the connector was asked %+v", *asked)
	}
	// Its own source for the connector: searched in its location.
	if _, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "apps"}); err != nil {
		t.Fatal(err)
	}
	src, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.apps"})
	if err != nil {
		t.Fatal(err)
	}
	var f api.InformationSourceFields
	_ = json.Unmarshal(src.Object.FieldsJSON(), &f)
	f.Location = map[string]any{"workspace": "acme"}
	raw, _ := json.Marshal(f)
	src.Object.SetFieldsJSON(raw)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: src.Object}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Search(ctx, api.SearchInput{Connector: "apps", Object: "m", Query: "stock"}); err != nil {
		t.Fatal(err)
	}
	if last := (*asked)[len(*asked)-1]; last.Location["workspace"] != "acme" || last.Root == "" {
		t.Errorf("the search did not run in the carrier's source location: %+v", last)
	}
	if _, err := svc.Search(ctx, api.SearchInput{Connector: "drive", Query: "x"}); err == nil {
		t.Error("a connector with no search was searched")
	}
}

// : a connector's propose method answers items with why and a
// score; the proposal wizard shows one step per chosen source, its picker
// pre-ticked with that proposal; a failed proposal leaves its picker empty,
// named in the errors.
func TestAWizardProposesFromEachSource(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	got, err := svc.Propose(ctx, api.ProposeInput{Connector: "apps", Object: "m", Terms: []string{"Acme"}})
	if err != nil || len(got.Items) != 1 || got.Items[0].Why != "named in Acme" || got.Items[0].Score != 0.9 {
		t.Fatalf("propose: %v %+v", err, got)
	}
	if _, err := svc.Propose(ctx, api.ProposeInput{Connector: "drive", Terms: []string{"x"}}); err == nil {
		t.Error("a connector with no propose proposed")
	}
	if _, err := svc.Propose(ctx, api.ProposeInput{Connector: "apps"}); err == nil {
		t.Error("a proposal with no context ran")
	}

	wz, err := svc.ProposalWizard(ctx, api.ProposalWizardInput{Object: "m", Terms: []string{"Acme"}})
	if err != nil || len(wz.Errors) != 0 {
		t.Fatalf("wizard: %v %v", err, wz.Errors)
	}
	if wz.Page["kind"] != "wizard" {
		t.Fatalf("the page is no wizard: %v", wz.Page)
	}
	ext := wz.Page["schema"].(map[string]any)["types"].(map[string]any)["Proposal"].(map[string]any)["extensions"].([]any)[0].(map[string]any)
	steps := ext["steps"].([]any)
	if ext["type"] != "ui-wizard" || len(steps) != 3 {
		t.Fatalf("one step per source that picks (apps, Documents and the local folder by browsing): %+v", ext)
	}
	step := steps[0].(map[string]any)
	picker := step["node"].(map[string]any)
	sel := picker["selected"].([]any)
	if step["title"] != "Applications" || step["description"] != "its interfaces" || picker["kind"] != "picker" ||
		picker["search"] != api.SearchURI || picker["object"] != "m" || len(sel) != 1 || sel[0].(map[string]any)["key"] != "A1" {
		t.Fatalf("the step's picker pre-ticked with the proposal: %+v", step)
	}
	failed, err := svc.ProposalWizard(ctx, api.ProposalWizardInput{Object: "m", Connectors: []string{"apps"}, Text: "boom"})
	if err != nil || len(failed.Errors) != 1 || !strings.Contains(failed.Errors[0], "Applications") {
		t.Fatalf("a failed proposal: %v %v", err, failed.Errors)
	}
	// : a search that failed is said beside what the others
	// found, which stays pre-ticked.
	partial, err := svc.ProposalWizard(ctx, api.ProposalWizardInput{Object: "m", Connectors: []string{"apps"}, Text: "partial"})
	if err != nil || len(partial.Errors) != 1 || partial.Errors[0] != "Applications: « Stock » : the map is slow" {
		t.Fatalf("a partly failed proposal: %v %v", err, partial.Errors)
	}
	pext := partial.Page["schema"].(map[string]any)["types"].(map[string]any)["Proposal"].(map[string]any)["extensions"].([]any)[0].(map[string]any)
	if psel := pext["steps"].([]any)[0].(map[string]any)["node"].(map[string]any)["selected"].([]any); len(psel) != 1 {
		t.Fatalf("what the others found stays pre-ticked: %+v", psel)
	}
	if _, err := svc.ProposalWizard(ctx, api.ProposalWizardInput{Connectors: []string{"nope"}, Text: "x"}); err == nil {
		t.Error("an unknown connector made a step")
	}
}

// : a choice can be held - kept on the carrier's source for the
// connector without being read - until the carrier applies it, which makes
// it the source's selection.
func TestAChoiceIsHeldUntilApplied(t *testing.T) {
	ctx, cwm, svc, _ := pickWorld(t)
	held, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}, {Key: "A2", Title: "Acme Stock"}}})
	if err != nil || !held.OK || held.Held != 2 || held.Source == "" {
		t.Fatalf("hold: %v %+v", err, held)
	}
	fields := func() api.InformationSourceFields {
		t.Helper()
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: held.Source})
		if err != nil {
			t.Fatal(err)
		}
		var f api.InformationSourceFields
		_ = json.Unmarshal(got.Object.FieldsJSON(), &f)
		return f
	}
	if f := fields(); len(f.Held) != 2 || len(api.SelectedItems(f.Selection)) != 0 {
		t.Fatalf("a held choice is kept apart from the selection a sync reads: %+v", f)
	}
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil || len(of.Sources) != 1 || of.Sources[0].Held != 2 || of.Sources[0].Picked != 0 {
		t.Fatalf("the Sources tab counts what is held: %v %+v", err, of.Sources)
	}
	// Holding again replaces the choice; the same source holds it.
	again, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{{Key: "A2", Title: "Acme Stock"}}})
	if err != nil || again.Source != held.Source || again.Held != 1 {
		t.Fatalf("hold again: %v %+v", err, again)
	}
	ap, err := svc.Apply(ctx, api.ApplyInput{ID: "m"})
	if err != nil || len(ap.Sources) != 1 || ap.Sources[0] != held.Source {
		t.Fatalf("apply: %v %+v", err, ap)
	}
	f := fields()
	if picked := api.SelectedItems(f.Selection); len(f.Held) != 0 || len(picked) != 1 || picked[0].Key != "A2" {
		t.Fatalf("applied, the held choice is the selection: %+v", f)
	}
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{{Key: " "}}}); err == nil {
		t.Error("an item with no key was held")
	}
}

// : applying reads what the choice adds and removes what it
// drops; each item records whether it is read, removed or failed and why.
func TestApplyingReadsAndRecordsEachItem(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	states := func() map[string]api.ItemState {
		t.Helper()
		of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
		if err != nil || len(of.Sources) != 1 {
			t.Fatalf("sources: %v %+v", err, of.Sources)
		}
		return of.Sources[0].States
	}
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}, {Key: "A2", Title: "Acme Stock"}, {Key: "BAD", Title: "Gone"}}}); err != nil {
		t.Fatal(err)
	}
	ap, err := svc.Apply(ctx, api.ApplyInput{ID: "m"})
	if err != nil || len(ap.Results) != 1 {
		t.Fatalf("apply: %v %+v", err, ap)
	}
	r := ap.Results[0]
	if strings.Join(r.Read, ",") != "A1,A2" || strings.Join(r.Failed, ",") != "BAD" {
		t.Fatalf("read and failed: %+v", r)
	}
	st := states()
	if st["A1"].State != api.ItemRead || st["BAD"].State != api.ItemFailed || !strings.Contains(st["BAD"].Why, "not found") {
		t.Fatalf("each item's state, with why: %+v", st)
	}
	// Applying a smaller choice removes what it drops.
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}}}); err != nil {
		t.Fatal(err)
	}
	if st := states(); st["A1"].State != api.ItemRead {
		t.Fatalf("held, nothing changes before applying: %+v", st)
	}
	ap, err = svc.Apply(ctx, api.ApplyInput{ID: "m"})
	if err != nil || len(ap.Results) != 1 {
		t.Fatalf("apply again: %v %+v", err, ap)
	}
	if r := ap.Results[0]; strings.Join(r.Read, ",") != "A1" || strings.Join(r.Removed, ",") != "A2,BAD" {
		t.Fatalf("the dropped items are removed: %+v", r)
	}
	// Removed, they keep no state.
	if st := states(); len(st) != 1 || st["A1"].State != api.ItemRead {
		t.Fatalf("states after removal: %+v", st)
	}
}

// : items read again with another content raise source.changed
// on the carrier; the daily and on-open modes re-read what is due; a daily
// source keeps its carrier's timer.
func TestSourcesAreReReadAndTheirChangesRaised(t *testing.T) {
	ctx, cwm, svc, _ := pickWorld(t)
	pickVersion = "v1"
	defer func() { pickVersion = "v1" }()
	held, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Apply(ctx, api.ApplyInput{ID: "m"}); err != nil {
		t.Fatal(err)
	}
	same, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: held.Source})
	if err != nil || len(same.Changed) != 0 {
		t.Fatalf("read again unchanged, nothing changed: %v %+v", err, same.Changed)
	}
	pickVersion = "v2"
	changed, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: held.Source})
	if err != nil || len(changed.Changed) != 1 || changed.Changed[0] != "Acme ERP" {
		t.Fatalf("an item changed at its source is named: %v %+v", err, changed)
	}
	// The carrier can receive it: the event type is declared and raisable.
	if _, err := cwm.EventRaise(ctx, cwmapi.EventRaiseInput{Type: api.EventSourceChanged, ObjectID: "m", Payload: []byte(`{"source":"m.apps","items":["x"]}`)}); err != nil {
		t.Fatalf("source.changed is not raisable on the carrier: %v", err)
	}

	setMode := func(mode string, last string) {
		t.Helper()
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: held.Source})
		if err != nil {
			t.Fatal(err)
		}
		var f api.InformationSourceFields
		_ = json.Unmarshal(got.Object.FieldsJSON(), &f)
		f.Sync, f.LastSynced = mode, last
		raw, _ := json.Marshal(f)
		got.Object.SetFieldsJSON(raw)
		if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: got.Object}); err != nil {
			t.Fatal(err)
		}
	}
	setMode(api.SyncDaily, "2020-01-01T00:00:00Z")
	if due, err := svc.SyncDue(ctx, api.SyncDueInput{ID: "m", Mode: api.SyncOnOpen}); err != nil || len(due.Synced) != 0 {
		t.Fatalf("a daily source is not on-open: %v %+v", err, due)
	}
	due, err := svc.SyncDue(ctx, api.SyncDueInput{ID: "m", Mode: api.SyncDaily})
	if err != nil || len(due.Synced) != 1 {
		t.Fatalf("a daily source not read for years is due: %v %+v", err, due)
	}
	if again, err := svc.SyncDue(ctx, api.SyncDueInput{ID: "m", Mode: api.SyncDaily}); err != nil || len(again.Synced) != 0 {
		t.Fatalf("just read, it is fresh: %v %+v", err, again)
	}
	if _, err := svc.SyncDue(ctx, api.SyncDueInput{ID: "m", Mode: "weekly"}); err == nil {
		t.Error("an unknown mode ran")
	}
	// Opening the tab keeps the daily timer on the carrier.
	if _, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"}); err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	timers, _ := got.Object.Capabilities["timed"]["timers"].([]any)
	if len(timers) != 1 || timers[0].(map[string]any)["name"] != api.DailyTimer {
		t.Fatalf("the carrier keeps the daily timer: %+v", got.Object.Capabilities["timed"])
	}
}

// : each connector says, with no location, whether it is
// configured and reachable, and where it is configured; its picker carries
// that link.
func TestAConnectorSaysWhereItIsConfigured(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	pickVersion = "v1"
	defer func() { pickVersion = "v1" }()
	st, err := svc.Status(ctx, api.StatusInput{Connector: "apps"})
	if err != nil || !st.Configured || !st.Reachable || st.Configure == nil || st.Configure.Label != "Configure the map" {
		t.Fatalf("a configured connector: %v %+v", err, st)
	}
	pickVersion = "unconfigured"
	st, err = svc.Status(ctx, api.StatusInput{Connector: "apps"})
	if err != nil || st.Configured || st.Reachable || !strings.Contains(st.Message, "APPS_TOKEN") || st.Configure.Keys[0] != "APPS_TOKEN" {
		t.Fatalf("a connector missing its key says so, and where to set it: %v %+v", err, st)
	}
	pk, err := svc.Pickers(ctx, api.PickersInput{})
	if err != nil || len(pk.Connectors) == 0 || pk.Connectors[0].Name != "apps" || pk.Connectors[0].Status != api.StatusURI || pk.Connectors[0].Configure == nil {
		t.Fatalf("the picker carries its status method and its configure link: %v %+v", err, pk.Connectors)
	}
	if st, err := svc.Status(ctx, api.StatusInput{Connector: "drive"}); err != nil || !st.Configured {
		t.Fatalf("a connector with no test reads as configured: %v %+v", err, st)
	}
	// A source's own picker asks its connector's status.
	used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: "apps"})
	if err != nil {
		t.Fatal(err)
	}
	if st, err := svc.Status(ctx, api.StatusInput{Source: used.Source}); err != nil || st.Connector != "apps" || st.Configured {
		t.Fatalf("a source's status is its connector's: %v %+v", err, st)
	}
}

// : a source's « Choisir » tab is the generic picker bound to the
// source - its search runs in the source's own location, its selection shows
// each item's state, then those held.
func TestASourcesPickerSearchesItAndShowsItsChoice(t *testing.T) {
	ctx, cwm, svc, asked := pickWorld(t)
	pickVersion = "v1"
	defer func() { pickVersion = "v1" }()
	held, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}, {Key: "BAD", Title: "Gone"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Apply(ctx, api.ApplyInput{ID: "m"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}, {Key: "A9", Title: "New"}}}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.SourcePicked(ctx, api.SourcePickedInput{ID: held.Source})
	if err != nil || got.Connector != "apps" || len(got.Selected) != 3 {
		t.Fatalf("source-picked: %v %+v", err, got)
	}
	byKey := map[string]api.PickState{}
	for _, p := range got.Selected {
		byKey[p.Key] = p
	}
	if byKey["A1"].State != api.ItemRead || byKey["BAD"].State != api.ItemFailed || !byKey["A9"].Held {
		t.Fatalf("each item's state, then the held ones: %+v", got.Selected)
	}
	// Its search runs in the source's location, with no carrier named.
	gotSrc, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: held.Source})
	if err != nil {
		t.Fatal(err)
	}
	var f api.InformationSourceFields
	_ = json.Unmarshal(gotSrc.Object.FieldsJSON(), &f)
	f.Location = map[string]any{"workspace": "sales"}
	raw, _ := json.Marshal(f)
	gotSrc.Object.SetFieldsJSON(raw)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: gotSrc.Object}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Search(ctx, api.SearchInput{Source: held.Source, Query: "acme"}); err != nil {
		t.Fatal(err)
	}
	if last := (*asked)[len(*asked)-1]; last.Location["workspace"] != "sales" || last.Root == "" {
		t.Fatalf("the search did not run in the source's location: %+v", last)
	}
}

// : a source-set value becomes the carrier's sources, one per
// connector with its items, read; the Sources tab's knowledge-base box counts
// the documents brought and what is still in progress.
func TestASourceSetValueBecomesTheCarriersSources(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	pickVersion = "v1"
	defer func() { pickVersion = "v1" }()
	if _, err := svc.UseSet(ctx, api.UseSetInput{ID: "m", Items: []api.UseSetItem{{Connector: "apps", Key: " "}}}); err == nil {
		t.Error("an item with no key was used")
	}
	got, err := svc.UseSet(ctx, api.UseSetInput{ID: "m", Wait: true, Items: []api.UseSetItem{
		{Connector: "apps", Key: "A1", Title: "Acme ERP"}, {Connector: "apps", Key: "A2", Title: "Acme Stock"},
	}})
	if err != nil || !got.OK || len(got.Sources) != 1 {
		t.Fatalf("use-set: %v %+v", err, got)
	}
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if of.Documents != 2 || of.Pending != 0 || of.DocumentsLabel != "2 document(s)" || of.PendingLabel != "à jour" {
		t.Fatalf("the box after reading: %+v", of)
	}
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "apps", Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}}}); err != nil {
		t.Fatal(err)
	}
	if of, _ = svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"}); of.Pending != 1 || of.PendingLabel != "1 en cours" {
		t.Fatalf("a held choice is in progress: %+v", of)
	}
}

// (mock-up v2): the
// « Sources » board's opened card draws its connector's own panel -
// « Utiliser » while it is unused, then the generic picker bound to the
// carrier and the connector in two columns, searching on Enter or its icon
// (never live, its hint saying so), its selection with states, « Appliquer »
// holding the choice; each card's row says what is picked.
func TestTheSourcesBoardDrawsTheSelectedConnector(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	unused, err := svc.ConnectorPanel(ctx, api.ConnectorPanelInput{ID: "m", Connector: "apps"})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(unused.DSLTree); !strings.Contains(string(raw), `Utiliser Applications`) || strings.Contains(string(raw), `"picker"`) {
		t.Fatalf("an unused connector offers Utiliser, no picker: %s", raw)
	}
	if _, err := svc.Hold(ctx, api.HoldInput{Object: "m", Connector: "apps", Items: []api.SelectedItem{{Key: "A1", Title: "Acme ERP"}}}); err != nil {
		t.Fatalf("a picker's apply holds through its object: %v", err)
	}
	used, err := svc.ConnectorPanel(ctx, api.ConnectorPanelInput{ID: "m", Connector: "apps"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(used.DSLTree)
	for _, want := range []string{`"kind":"picker"`, `"apply":"ktw-agents:sources/hold"`, `"status":"ktw-agents:sources/status"`, `"derived":"its interfaces"`, `"object":"m"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the used connector's panel lacks %s: %s", want, raw)
		}
	}
	var picker map[string]any
	for _, c := range used.DSLTree["children"].([]any) {
		if n, ok := c.(map[string]any); ok && n["kind"] == "picker" {
			picker = n
		}
	}
	if picker == nil || picker["columns"] != true {
		t.Fatalf("the opened card's picker draws two columns: %+v", picker)
	}
	if _, live := picker["live"]; live {
		t.Errorf("mock-up v2's picker never searches while typing: %+v", picker)
	}
	if hint, _ := picker["hint"].(string); !strings.Contains(hint, "Entrée") || strings.Contains(hint, "deux lettres") {
		t.Errorf("its hint says Enter, never two letters: %q", hint)
	}
	if sel, _ := used.Data["selected"].([]any); len(sel) != 1 || used.Data["count"] != "1 application" {
		t.Fatalf("its data: %+v", used.Data)
	}
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range of.Connectors {
		if c.Name == "apps" && (!c.On || c.Key != "apps" || c.Count != "1 application" || len(c.Chips) != 1) {
			t.Fatalf("the apps row: %+v", c)
		}
	}
}

// The proposal wizard as a page region - proposing from the
// terms a page edits, a failed proposal said above it, its finish applying
// what the carrier holds (its knowledge base built from what is kept).
func TestTheProposalPageFinishesWithApply(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	got, err := svc.ProposalPage(ctx, api.ProposalPageInput{Object: "m", Terms: []string{"Acme"}, CompleteLabel: "Construire la base de connaissance"})
	if err != nil {
		t.Fatal(err)
	}
	kids := got.DSLTree["children"].([]any)
	wz := kids[len(kids)-1].(map[string]any)
	if wz["kind"] != "wizard" || len(kids) != 1 {
		t.Fatalf("the region holds the wizard alone: %+v", got.DSLTree)
	}
	done := wz["complete"].(map[string]any)
	if done["uri"] != api.ApplyURI || done["label"] != "Construire la base de connaissance" || done["args"].(map[string]any)["id"] != "m" {
		t.Errorf("the finish applies what the carrier holds: %+v", done)
	}
	steps := wz["schema"].(map[string]any)["types"].(map[string]any)["Proposal"].(map[string]any)["extensions"].([]any)[0].(map[string]any)["steps"].([]any)
	if sel := steps[0].(map[string]any)["node"].(map[string]any)["selected"].([]any); len(sel) != 1 {
		t.Errorf("the step proposes from the terms: %+v", steps[0])
	}
	failed, err := svc.ProposalPage(ctx, api.ProposalPageInput{Object: "m", Connectors: []string{"apps"}, Text: "boom"})
	if err != nil {
		t.Fatal(err)
	}
	fk := failed.DSLTree["children"].([]any)
	if len(fk) != 2 || !strings.Contains(fk[0].(map[string]any)["content"].(string), "Applications") {
		t.Errorf("a failed proposal is said above the wizard: %+v", fk)
	}
	if def, _ := svc.ProposalPage(ctx, api.ProposalPageInput{Object: "m", Terms: []string{"Acme"}}); def.DSLTree["children"].([]any)[0].(map[string]any)["complete"].(map[string]any)["label"] != "Appliquer" {
		t.Error("the finish's default words")
	}
	if _, err := svc.ProposalPage(ctx, api.ProposalPageInput{Terms: []string{"x"}}); err == nil {
		t.Error("a proposal page with no carrier")
	}
}

// : each proposal step holds what is ticked - its picker
// tickable, applying Hold with what a picker sends (its args' connector, the
// carrier, the items) - and the proposal page takes a cancel action.
func TestAProposalStepHoldsWhatIsTicked(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	wz, err := svc.ProposalWizard(ctx, api.ProposalWizardInput{Object: "m", Terms: []string{"Acme"}})
	if err != nil {
		t.Fatal(err)
	}
	steps := wz.Page["schema"].(map[string]any)["types"].(map[string]any)["Proposal"].(map[string]any)["extensions"].([]any)[0].(map[string]any)["steps"].([]any)
	picker := steps[0].(map[string]any)["node"].(map[string]any)
	if picker["tickable"] != true || picker["apply"] != api.HoldURI || picker["applyOn"] != "change" {
		t.Fatalf("the step's picker holds what is ticked: %+v", picker)
	}
	// What the picker sends its apply: its args, the carrier, the ticked items.
	in := map[string]any{"object": picker["object"], "items": []any{map[string]any{"key": "A1", "title": "Acme ERP"}}}
	for k, v := range picker["args"].(map[string]any) {
		in[k] = v
	}
	raw, _ := json.Marshal(in)
	var hold api.HoldInput
	if err := json.Unmarshal(raw, &hold); err != nil {
		t.Fatal(err)
	}
	if held, err := svc.Hold(ctx, hold); err != nil || !held.OK || held.Held != 1 {
		t.Fatalf("Hold takes what the picker sends: %v %+v", err, held)
	}
	cancel := map[string]any{"label": "Abandonner", "uri": "ktw-x:studies/abandon", "args": map[string]any{"id": "m"}, "confirm": "Abandonner ?"}
	page, err := svc.ProposalPage(ctx, api.ProposalPageInput{Object: "m", Terms: []string{"Acme"}, Cancel: cancel})
	if err != nil {
		t.Fatal(err)
	}
	kids := page.DSLTree["children"].([]any)
	if got := kids[len(kids)-1].(map[string]any)["cancel"]; got == nil || got.(map[string]any)["uri"] != "ktw-x:studies/abandon" {
		t.Errorf("the page's cancel is the one given: %v", got)
	}
	bare, _ := svc.ProposalPage(ctx, api.ProposalPageInput{Object: "m", Terms: []string{"Acme"}})
	if _, has := bare.DSLTree["children"].([]any)[0].(map[string]any)["cancel"]; has {
		t.Error("a cancel with none given")
	}
}

// : a proposal page's finish is the action given - a
// consumer's own run -, Apply without one.
func TestAProposalPageFinishesWithTheActionGiven(t *testing.T) {
	ctx, _, svc, _ := pickWorld(t)
	complete := map[string]any{"label": "Faire l'étude", "uri": "ktw-x:studies/run", "args": map[string]any{"id": "m"}}
	page, err := svc.ProposalPage(ctx, api.ProposalPageInput{Object: "m", Terms: []string{"Acme"}, CompleteLabel: "ignored", Complete: complete})
	if err != nil {
		t.Fatal(err)
	}
	kids := page.DSLTree["children"].([]any)
	if got := kids[len(kids)-1].(map[string]any)["complete"].(map[string]any); got["uri"] != "ktw-x:studies/run" || got["label"] != "Faire l'étude" {
		t.Errorf("the page's finish is the one given: %v", got)
	}
	bare, _ := svc.ProposalPage(ctx, api.ProposalPageInput{Object: "m", Terms: []string{"Acme"}})
	if got := bare.DSLTree["children"].([]any)[0].(map[string]any)["complete"].(map[string]any); got["uri"] != api.ApplyURI {
		t.Errorf("Apply with no finish given: %v", got)
	}
}

// : with no connector named, the proposal wizard offers each
// connector a picker can search - pre-ticked where it proposes, its picker
// empty (never null) where it cannot -, and its steps are an empty list, never
// null, when none can be searched.
func TestTheProposalWizardOffersEachSearchableSourceAndNeverNull(t *testing.T) {
	ctx, _, svc, _ := pickWorldWith(t, map[string]string{"notes.yaml": "name: notes\nlabel: Notes\ndescription: A notes folder.\nsync: ktw-notes:notes/source-sync\nsearch: ktw-notes:notes/search\n"})
	stepsOf := func(wz api.ProposalWizardOutput) []any {
		t.Helper()
		raw, err := json.Marshal(wz.Page)
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Schema struct {
				Types struct {
					Proposal struct {
						Extensions []struct {
							Steps *[]map[string]any `json:"steps"`
						} `json:"extensions"`
					} `json:"Proposal"`
				} `json:"types"`
			} `json:"schema"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			t.Fatal(err)
		}
		ext := page.Schema.Types.Proposal.Extensions
		if len(ext) != 1 || ext[0].Steps == nil {
			t.Fatalf("the wizard's steps are null: %s", raw)
		}
		out := make([]any, len(*ext[0].Steps))
		for i, s := range *ext[0].Steps {
			out[i] = s
		}
		return out
	}
	wz, err := svc.ProposalWizard(ctx, api.ProposalWizardInput{Object: "m", Terms: []string{"Acme"}})
	if err != nil {
		t.Fatal(err)
	}
	steps := stepsOf(wz)
	if len(steps) != 4 {
		t.Fatalf("one step per connector that picks (apps, Documents and the local folder by browsing, notes), got %d: %+v", len(steps), steps)
	}
	byID := map[string]map[string]any{}
	for _, s := range steps {
		st := s.(map[string]any)
		byID[st["id"].(string)] = st["node"].(map[string]any)
	}
	if sel, _ := byID["apps"]["selected"].([]any); len(sel) != 1 {
		t.Errorf("the proposing source is pre-ticked: %+v", byID["apps"])
	}
	notes, ok := byID["notes"]
	if !ok {
		t.Fatalf("a searchable source that cannot propose is offered: %+v", byID)
	}
	if sel, isList := notes["selected"].([]any); !isList || len(sel) != 0 {
		t.Errorf("its picker is empty, never null: %#v", notes["selected"])
	}
	// : a step whose connector declares a test says where its
	// source stands; one with no test has no status to ask.
	if byID["apps"]["status"] != api.StatusURI {
		t.Errorf("the apps step carries its status method: %+v", byID["apps"])
	}
	if _, has := notes["status"]; has {
		t.Errorf("a connector with no test has no status: %+v", notes)
	}
	// No searchable connector named: an empty list of steps.
	none, err := svc.ProposalWizard(ctx, api.ProposalWizardInput{Object: "m", Connectors: []string{"drive"}, Text: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if got := stepsOf(none); len(got) != 0 {
		t.Errorf("no searchable source, no step: %+v", got)
	}
}
