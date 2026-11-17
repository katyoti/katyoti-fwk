// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/progress"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// Three sources searched at once: maps proposes and is slow
// down cannot be reached, runs reads from what maps proposed by its code;
// off is not configured.
const (
	msMaps = `name: maps
label: Cartographie
description: The applications of the map.
sync: ktw-ms:maps/source-sync
search: ktw-ms:maps/search
propose: ktw-ms:maps/propose
`
	msRuns = `name: runs
label: Exploitation
description: The monitored entities of the applications.
sync: ktw-ms:runs/source-sync
browse: ktw-ms:runs/browse
from: {connector: maps, attr: code}
`
	msDown = `name: down
label: Wiki
description: Pages of a wiki out of reach.
sync: ktw-ms:down/source-sync
test: ktw-ms:down/test
search: ktw-ms:down/search
`
	msOff = `name: off
label: Tickets
description: Tickets of a tracker with no key set.
sync: ktw-ms:off/source-sync
test: ktw-ms:off/test
search: ktw-ms:off/search
configure: {label: Configurer Tickets, keys: [TICKETS_TOKEN]}
`
)

// msActions serves the fake connectors: maps' propose waits for release,
// so the others answer first; every call is recorded in order.
type msActions struct {
	mu      *sync.Mutex
	calls   *[]string
	release <-chan struct{}
	from    *[]*api.FromItems
}

func (msActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a msActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	a.mu.Lock()
	*a.calls = append(*a.calls, in.URI)
	a.mu.Unlock()
	var value any
	switch in.URI {
	case "ktw-ms:maps/propose":
		select {
		case <-a.release:
		case <-ctx.Done():
			return actionsapi.RunOutput{}, ctx.Err()
		}
		value = api.ConnectorProposeOutput{Items: []api.ProposedItem{
			{PickItem: api.PickItem{Key: "erp", Title: "ERP", Attrs: map[string]string{"code": "ERP"}}, Why: "Trouvée par « achats »", Score: 1},
			{PickItem: api.PickItem{Key: "crm", Title: "CRM", Attrs: map[string]string{"code": "CRM"}}, Why: "Trouvée par « achats »", Score: 0.5},
		}}
		a.mu.Lock()
		*a.calls = append(*a.calls, "maps answered")
		a.mu.Unlock()
	case "ktw-ms:runs/browse":
		var bin api.ConnectorBrowseInput
		_ = json.Unmarshal(in.Input, &bin)
		a.mu.Lock()
		*a.from = append(*a.from, bin.From)
		a.mu.Unlock()
		items := []api.BrowseItem{}
		if bin.From != nil {
			for _, v := range bin.From.Values {
				items = append(items, api.BrowseItem{Key: "host-" + strings.ToLower(v), Title: v + " host"})
			}
		}
		value = api.ConnectorBrowseOutput{Items: items}
	case "ktw-ms:down/test":
		value = api.ConnectorTestOutput{OK: false, Message: "hors du réseau"}
	case "ktw-ms:off/test":
		value = api.ConnectorTestOutput{NotConfigured: true, Message: "TICKETS_TOKEN manque"}
	case "ktw-ms:down/search", "ktw-ms:off/search", "ktw-ms:maps/search":
		value = api.ConnectorSearchOutput{Items: []api.PickItem{}}
	case "ktw-agents:sources/source-check":
		value = cwmapi.HookAccept{Accept: true}
	default:
		return actionsapi.RunOutput{}, fmt.Errorf("no method %s", in.URI)
	}
	raw, _ := json.Marshal(value)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// msRecorder keeps every snapshot a search emits, and releases maps once
// down and off have answered - so maps is the slow one, whatever the
// machine.
type msRecorder struct {
	mu      sync.Mutex
	snaps   []api.SearchSourcesOutput
	release chan struct{}
	once    sync.Once
}

func (r *msRecorder) Emit(snapshot any) {
	snap, ok := snapshot.(api.SearchSourcesOutput)
	if !ok {
		return
	}
	r.mu.Lock()
	r.snaps = append(r.snaps, snap)
	r.mu.Unlock()
	answered := 0
	for _, s := range snap.Sources {
		if (s.Connector == "down" || s.Connector == "off") && s.Done() {
			answered++
		}
	}
	if answered == 2 {
		r.once.Do(func() { close(r.release) })
	}
}

// states is each source's states in the order the snapshots moved them.
func (r *msRecorder) states() map[string][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string][]string{}
	for _, snap := range r.snaps {
		for _, s := range snap.Sources {
			st := out[s.Connector]
			if len(st) == 0 || st[len(st)-1] != s.State {
				out[s.Connector] = append(st, s.State)
			}
		}
	}
	return out
}

// : every source searches at once, off any page's load, each
// answering as it comes with its state; a source reading from another waits
// for its answer and follows what it proposed; with a carrier what it holds
// is not proposed again; with none nothing is held or created; one source is
// searched again alone.
func TestEverySourceSearchesAtOnceEachAnsweringAsItComes(t *testing.T) {
	root := t.TempDir()
	calls, from := &[]string{}, &[]*api.FromItems{}
	release := make(chan struct{})
	ms := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/maps.yaml": {Data: []byte(msMaps)},
		"connectors/runs.yaml": {Data: []byte(msRuns)},
		"connectors/down.yaml": {Data: []byte(msDown)},
		"connectors/off.yaml":  {Data: []byte(msOff)},
	}, ".")
	mu := &sync.Mutex{}
	var released <-chan struct{} = release
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		contributeConnectors("ktw-ms", ms, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			mu.Lock()
			defer mu.Unlock()
			return msActions{mu: mu, calls: calls, release: released, from: from}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.KnowledgeCapability}); err != nil {
		t.Fatal(err)
	}
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	callsSoFar := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, *calls...)
	}

	// A page's load lists the pickers: no source is searched.
	if _, err := svc.Pickers(ctx, api.PickersInput{ID: "m"}); err != nil {
		t.Fatal(err)
	}
	if got := callsSoFar(); len(got) != 0 {
		t.Fatalf("a page's load searches nothing: %v", got)
	}

	// No carrier (a creation form): every source at once.
	rec := &msRecorder{release: release}
	out, err := svc.SearchSources(progress.WithEmitter(ctx, rec), api.SearchSourcesInput{Terms: []string{"achats"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.snaps) == 0 {
		t.Fatal("the search emits each source as it moves")
	}
	first := map[string]api.SourceSearch{}
	for _, s := range rec.snaps[0].Sources {
		first[s.Connector] = s
	}
	if first["runs"].State != api.SearchWaiting || first["runs"].Waiting != "maps" || first["runs"].WaitingLabel != "Cartographie" {
		t.Errorf("runs first waits on maps: %+v", first["runs"])
	}
	if first["maps"].State != api.SearchSearching || first["down"].State != api.SearchSearching {
		t.Errorf("maps and down first search: %+v", rec.snaps[0])
	}
	states := rec.states()
	want := map[string]string{
		"maps": "searching,found",
		"runs": "waiting,searching,found",
		"down": "searching,unreachable",
		"off":  "searching,not-configured",
	}
	for name, w := range want {
		if got := strings.Join(states[name], ","); got != w {
			t.Errorf("%s moves %s, want %s", name, got, w)
		}
	}
	// The slow one answers last but runs: down and off were answered while
	// maps still searched.
	got := callsSoFar()
	answered, browsed := -1, -1
	for i, c := range got {
		switch c {
		case "maps answered":
			answered = i
		case "ktw-ms:runs/browse":
			browsed = i
		}
	}
	if answered < 0 || browsed < answered {
		t.Fatalf("runs is browsed only once maps answered: %v", got)
	}
	byName := map[string]api.SourceSearch{}
	for _, s := range out.Sources {
		byName[s.Connector] = s
	}
	if s := byName["maps"]; s.Found != 2 || len(s.Items) != 2 || s.Items[0].Why == "" {
		t.Errorf("maps proposes its two items with why: %+v", s)
	}
	if s := byName["runs"]; s.Found != 2 || strings.Join(sorted(proposedKeys(s.Items)), ",") != "host-crm,host-erp" || !strings.Contains(s.Items[0].Why, "Cartographie") {
		t.Errorf("runs follows what maps proposed: %+v", s)
	}
	if s := byName["down"]; s.Message != "hors du réseau" || s.Found != 0 {
		t.Errorf("down is unreachable, saying why: %+v", s)
	}
	if s := byName["off"]; s.Configure == nil || s.Configure.Label != "Configurer Tickets" {
		t.Errorf("off is not configured, saying where: %+v", s)
	}
	for _, c := range got {
		if c == "ktw-ms:down/search" || c == "ktw-ms:off/search" {
			t.Errorf("a source unreachable or not configured is not searched: %v", got)
		}
	}
	// No carrier: nothing held, nothing created.
	if objs, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "m", Type: api.InformationSourceType}); err != nil || len(objs.Objects) != 0 {
		t.Fatalf("a search with no carrier creates nothing: %+v %v", objs, err)
	}

	// A carrier holding ERP: « Chercher encore » proposes only CRM; runs
	// still follows ERP.
	if _, err := svc.Hold(ctx, api.HoldInput{ID: "m", Connector: "maps", Items: []api.SelectedItem{{Key: "erp", Title: "ERP"}}}); err != nil {
		t.Fatal(err)
	}
	out, err = svc.SearchSources(ctx, api.SearchSourcesInput{Object: "m", Terms: []string{"achats"}, Connectors: []string{"maps", "runs"}})
	if err != nil {
		t.Fatal(err)
	}
	byName = map[string]api.SourceSearch{}
	for _, s := range out.Sources {
		byName[s.Connector] = s
	}
	if s := byName["maps"]; s.Found != 1 || s.Held != 1 || s.Items[0].Key != "crm" {
		t.Errorf("what the carrier holds is not proposed again: %+v", s)
	}
	if s := byName["runs"]; strings.Join(sorted(proposedKeys(s.Items)), ",") != "host-crm,host-erp" {
		t.Errorf("runs follows what maps found, held included: %+v", s)
	}

	// « Réessayer » on runs: it alone answers, maps searched again quietly.
	out, err = svc.SearchSources(ctx, api.SearchSourcesInput{Terms: []string{"achats"}, Only: "runs"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Sources) != 1 || out.Sources[0].Connector != "runs" || out.Sources[0].Found != 2 {
		t.Errorf("only runs answers, following maps: %+v", out.Sources)
	}
}

func proposedKeys(items []api.ProposedItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Key
	}
	return out
}
