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
	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// knowledgeActions is the ActionService of the knowledge tests: the
// local-folder connector routed to the service, a fake wiki connector writing
// one page into dest, and a fake knowledge base engine recording each kb/run.
type knowledgeActions struct {
	mu   *sync.Mutex
	runs *[]string
	// kb is what each kb/run answers: a clean run unless a test sets it.
	kb *kbapi.RunOutput
	// calls, when a test sets it, records each call made.
	calls *[]actionsapi.RunInput
}

func (a knowledgeActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a knowledgeActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	if a.calls != nil {
		a.mu.Lock()
		*a.calls = append(*a.calls, in)
		a.mu.Unlock()
	}
	var value any
	switch in.URI {
	case "ktw-agents:sources/folder-sync":
		svc, err := application.UseService[api.SourcesService](ctx)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		var sin api.ConnectorSyncInput
		if err := json.Unmarshal(in.Input, &sin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		if value, err = svc.FolderSync(ctx, sin); err != nil {
			return actionsapi.RunOutput{}, err
		}
	case "ktw-agents:sources/folder-browse":
		svc, err := application.UseService[api.SourcesService](ctx)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		var bin api.ConnectorBrowseInput
		if err := json.Unmarshal(in.Input, &bin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		if value, err = svc.FolderBrowse(ctx, bin); err != nil {
			return actionsapi.RunOutput{}, err
		}
	case "ktw-wiki:wiki/source-sync":
		var sin api.ConnectorSyncInput
		if err := json.Unmarshal(in.Input, &sin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		if err := os.WriteFile(filepath.Join(sin.Dest, "home.md"), []byte("# Home\n\nThe wiki's home.\n"), 0o644); err != nil {
			return actionsapi.RunOutput{}, err
		}
		out := api.ConnectorSyncOutput{Items: []api.ConnectorItem{{Path: "home.md", URI: "wiki://home", Title: "Home"}}}
		// The page "partial": two of its children are refused, for rights.
		if page, _ := sin.Location["page"].(string); page == "partial" {
			out.Errors = []string{"child-1: 403 Forbidden", "child-2: 403 Forbidden"}
		}
		value = out
	case "ktw-agents:sources/source-check":
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
		value = got
	case api.KBRunURI:
		var kin struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(in.Input, &kin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		a.mu.Lock()
		*a.runs = append(*a.runs, kin.Path)
		value = kbapi.RunOutput{Index: kbapi.IndexOutput{ChunksIndexed: 3}}
		if a.kb != nil {
			value = *a.kb
		}
		a.mu.Unlock()
	default:
		return actionsapi.RunOutput{}, fmt.Errorf("no method %s", in.URI)
	}
	raw, _ := json.Marshal(value)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// knowledgeWorld boots a real CWM with the sources pack, the local-folder
// connector, a wiki connector another member contributes and a module `m`
// carrying the knowledge capability; runs records each kb/run's path.
func knowledgeWorld(t *testing.T) (context.Context, cwmapi.CWMService, api.SourcesService, string, *[]string) {
	ctx, cwm, svc, root, runs, _ := knowledgeWorldKB(t)
	return ctx, cwm, svc, root, runs
}

// knowledgeWorldKB is knowledgeWorld with what kb/run answers in hand.
func knowledgeWorldKB(t *testing.T) (context.Context, cwmapi.CWMService, api.SourcesService, string, *[]string, *kbapi.RunOutput) {
	t.Helper()
	return knowledgeWorldCalls(t, nil)
}

// knowledgeWorldCalls is knowledgeWorldKB recording each call into calls.
func knowledgeWorldCalls(t *testing.T, calls *[]actionsapi.RunInput) (context.Context, cwmapi.CWMService, api.SourcesService, string, *[]string, *kbapi.RunOutput) {
	t.Helper()
	root := t.TempDir()
	runs := &[]string{}
	kb := &kbapi.RunOutput{Index: kbapi.IndexOutput{ChunksIndexed: 3}}
	foreign := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/wiki-page.yaml": {Data: []byte("name: wiki-page\nlabel: Wiki page\ndescription: A page of the team wiki.\ncolor: grape\nsync: ktw-wiki:wiki/source-sync\n")},
	}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-wiki", foreign, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return knowledgeActions{mu: &sync.Mutex{}, runs: runs, kb: kb, calls: calls}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.KnowledgeCapability}); err != nil {
		t.Fatalf("the knowledge capability is not declared: %v", err)
	}
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc, root, runs, kb
}

// The sources pack declares the knowledge capability - its kb and input files
// resolved on the carrier - and the « Sources » tab shown on its carriers,
// read from sources-of.
func TestKnowledgeCapabilityDeclared(t *testing.T) {
	ctx, cwm, _, _, _ := knowledgeWorld(t)
	files, err := cwm.ObjectNamedFileList(ctx, cwmapi.ObjectNamedFileListInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range files.Files {
		if f.Capability == api.KnowledgeCapability {
			got[f.Name] = f.Path
		}
	}
	if got["kb"] != ".kb/KnowledgeBase.yaml" || !strings.HasPrefix(got["input"], "input") {
		t.Errorf("the knowledge capability's files: %v, want kb .kb/KnowledgeBase.yaml and input input/", got)
	}

	panels, err := mbapi.QueryTyped[paneapi.PanelDecl](ctx, mbapi.Q{Reach: mbapi.ReachSelf, Kind: paneapi.ObjectPanelKind})
	if err != nil {
		t.Fatal(err)
	}
	var tab *paneapi.PanelDecl
	for i := range panels {
		if strings.HasSuffix(panels[i].URI, "/sources") {
			tab = &panels[i].Body
		}
	}
	if tab == nil {
		t.Fatalf("no Sources panel among %d panels", len(panels))
	}
	if tab.Source != "ktw-agents:sources/sources-of" || len(tab.Filter.Capabilities) != 1 || tab.Filter.Capabilities[0] != api.KnowledgeCapability {
		t.Errorf("the Sources panel is not shown on knowledge carriers from sources-of: %+v", tab)
	}
}

// A carrier's sources are listed - the one under it and the one feeding it
// from another module - with their facts and its knowledge base; sync-all
// syncs each, goes on past a failing one, then reindexes the carrier's .kb;
// with no base set up nothing is reindexed, and that is said.
func TestKnowledgeCapability(t *testing.T) {
	ctx, cwm, svc, root, runs := knowledgeWorld(t)
	ext := filepath.Join(root, "ext")
	_ = os.MkdirAll(ext, 0o755)
	if err := os.WriteFile(filepath.Join(ext, "a.md"), []byte("# A\n\nAlpha.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwmapp.Module(t, ctx, cwm, "other")
	create := func(module, local, parent string, fields map[string]any) string {
		t.Helper()
		obj := cwmapi.NewContainerObject(module, local, api.InformationSourceType, parent, local, "")
		raw, _ := json.Marshal(fields)
		obj.SetFieldsJSON(raw)
		out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: obj})
		if err != nil {
			t.Fatalf("create %s: %v", local, err)
		}
		return out.Object.ID
	}
	folder := create("m", "specs", "m", map[string]any{"connector": api.FolderConnector, "location": map[string]any{"path": "ext"}})
	wiki := create("other", "wiki", "other", map[string]any{"connector": "wiki-page", "location": map[string]any{"page": "home"}})
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: wiki, To: "m", Kind: api.FeedsEdge}); err != nil {
		t.Fatal(err)
	}
	// One source per connector per holder: the failing one names a connector
	// nobody contributes.
	broken := create("m", "broken", "m", map[string]any{"connector": "nobody", "location": map[string]any{"path": "nowhere"}})
	create("other", "elsewhere", "other", map[string]any{"connector": api.FolderConnector, "location": map[string]any{"path": "ext"}})

	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, s := range of.Sources {
		ids = append(ids, s.ID+"/"+s.Via)
	}
	if strings.Join(ids, " ") != broken+"/child "+folder+"/child "+wiki+"/feeds" {
		t.Errorf("sources of m = %v, want its two children then the one feeding it - never a source of another module that does not", ids)
	}
	if of.KBExists || !strings.HasSuffix(filepath.ToSlash(of.KBPath), "/.kb") || !strings.HasSuffix(filepath.ToSlash(of.InputPath), "/input") {
		t.Errorf("knowledge base before it is set up: %+v", of)
	}

	// No base set up: every source is synced, nothing is reindexed, and that
	// is said.
	res, err := svc.SyncAll(ctx, api.SyncAllInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Reindexed || len(*runs) != 0 || !strings.Contains(res.Message, "pas créée") {
		t.Errorf("sync-all with no base: %+v runs %v", res, *runs)
	}

	// The base set up: every source synced, the failing one reported, the
	// others brought, then the carrier's .kb reindexed once.
	if err := os.MkdirAll(of.KBPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(of.KBPath, "KnowledgeBase.yaml"), []byte("name: m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = svc.SyncAll(ctx, api.SyncAllInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || !res.Reindexed || len(res.Sources) != 3 || len(res.Errors) != 1 || !strings.Contains(res.Errors[0], broken) {
		t.Fatalf("sync-all: %+v", res)
	}
	if len(*runs) != 1 || filepath.Clean((*runs)[0]) != filepath.Clean(of.KBPath) {
		t.Errorf("kb/run paths %v, want once on %s", *runs, of.KBPath)
	}
	if _, err := os.Stat(filepath.Join(of.InputPath, "specs", "a.md")); err != nil {
		t.Errorf("the folder source's file is not in the carrier's input/: %v", err)
	}

	of, err = svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	facts := map[string]api.SourceSummary{}
	for _, s := range of.Sources {
		facts[s.ID] = s
	}
	if f := facts[folder]; f.State != api.SourceStateSynced || f.Items != 1 || f.LastSynced == "" {
		t.Errorf("the folder source's facts: %+v", f)
	}
	if f := facts[wiki]; f.State != api.SourceStateSynced || f.Items != 1 || f.Connector != "wiki-page" {
		t.Errorf("the wiki source's facts: %+v", f)
	}
	if f := facts[broken]; f.State != api.SourceStateFailed || f.Error == "" {
		t.Errorf("the failing source's facts: %+v", f)
	}
	if !of.KBExists || of.KBSize == 0 {
		t.Errorf("the knowledge base once set up: %+v", of)
	}

	// Reindex alone runs the engine on the same base.
	if r, err := svc.Reindex(ctx, api.ReindexInput{ID: "m"}); err != nil || !r.OK || len(*runs) != 2 {
		t.Errorf("reindex: %v %+v runs %v", err, r, *runs)
	}

	// A carrier that is not a module is refused, saying why.
	if _, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: folder}); err == nil || !strings.Contains(err.Error(), "not a module") {
		t.Errorf("a source as carrier: %v", err)
	}
}

// What a carrier says is true: a source whose connector
// could not read some of the items it named is not synced - it fails, saying
// how many of how many and why - and a knowledge base whose run reports
// index errors or failed extractions is not « réindexée »: sync-all and
// reindex answer ok false naming the first error and the count.
func TestSyncAndReindexSayWhatFailed(t *testing.T) {
	ctx, cwm, svc, _, _, kb := knowledgeWorldKB(t)
	obj := cwmapi.NewContainerObject("m", "wiki", api.InformationSourceType, "m", "wiki", "")
	raw, _ := json.Marshal(map[string]any{"connector": "wiki-page", "location": map[string]any{"page": "partial"}})
	obj.SetFieldsJSON(raw)
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj})
	if err != nil {
		t.Fatal(err)
	}
	id := made.Object.ID

	res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if res.OK || res.State != api.SourceStateFailed || res.Items != 1 || !strings.Contains(res.Message, "2 élément(s) sur 3") {
		t.Errorf("a sync with two items not read: %+v, want failed, « 2 élément(s) sur 3 »", res)
	}
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(of.Sources) != 1 || of.Sources[0].State != api.SourceStateFailed || !strings.Contains(of.Sources[0].Error, "403 Forbidden") {
		t.Errorf("the partly read source's facts: %+v", of.Sources)
	}

	// The base set up, the source read whole: synced, and reindexed.
	if err := os.MkdirAll(of.KBPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(of.KBPath, "KnowledgeBase.yaml"), []byte("name: m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(map[string]any{"connector": "wiki-page", "location": map[string]any{"page": "home"}})
	cur, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
	if err != nil {
		t.Fatal(err)
	}
	obj = cur.Object
	obj.SetFieldsJSON(raw)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: obj}); err != nil {
		t.Fatal(err)
	}
	all, err := svc.SyncAll(ctx, api.SyncAllInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if !all.OK || !all.Reindexed || len(all.Sources) != 1 || all.Sources[0].State != api.SourceStateSynced {
		t.Fatalf("sync-all read whole on a clean index: %+v", all)
	}

	// The index run reports embedding errors: nothing is « réindexée ».
	*kb = kbapi.RunOutput{Index: kbapi.IndexOutput{Errors: []string{"embed: HTTP 404", "embed: HTTP 404"}}}
	all, err = svc.SyncAll(ctx, api.SyncAllInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if all.OK || all.Reindexed || !strings.Contains(all.Message, "2 erreur(s) d'indexation (embed: HTTP 404)") {
		t.Errorf("sync-all after a failed index: %+v", all)
	}
	r, err := svc.Reindex(ctx, api.ReindexInput{ID: "m"})
	if err != nil || r.OK || !strings.Contains(r.Message, "2 erreur(s) d'indexation") {
		t.Errorf("reindex after a failed index: %v %+v", err, r)
	}

	// Failed extractions fail it too.
	*kb = kbapi.RunOutput{Extract: kbapi.ExtractOutput{Failed: 1, Errors: []string{"a.pdf: unreadable"}}, Index: kbapi.IndexOutput{ChunksIndexed: 2}}
	if r, err := svc.Reindex(ctx, api.ReindexInput{ID: "m"}); err != nil || r.OK || !strings.Contains(r.Message, "1 extraction(s) en échec (a.pdf: unreadable)") {
		t.Errorf("reindex after a failed extraction: %v %+v", err, r)
	}
}
