// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// actionsRouting is the ActionService of the test: it routes the sources
// methods a sync calls by URI to the service in the process, as the bus does
// in a binary.
type actionsRouting struct{ ctx context.Context }

func (a actionsRouting) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a actionsRouting) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	if in.URI == "ktw-agents:sources/source-check" {
		var cin api.SourceCheckInput
		if err := json.Unmarshal(in.Input, &cin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		out, err := svc.SourceCheck(ctx, cin)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		raw, _ := json.Marshal(out)
		return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
	}
	if in.URI == "ktw-wiki:wiki/source-check" {
		// The wiki connector's check: a page named forbidden
		// cannot be read, the rest saves.
		var cin api.SourceCheckInput
		_ = json.Unmarshal(in.Input, &cin)
		loc, _ := cin.Object.Fields["location"].(map[string]any)
		out := cwmapi.HookAccept{Accept: true}
		if loc["page"] == "forbidden" {
			out = cwmapi.HookAccept{Reason: "the wiki refuses the page forbidden"}
		}
		raw, _ := json.Marshal(out)
		return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
	}
	if in.URI != "ktw-agents:sources/folder-sync" {
		return actionsapi.RunOutput{}, fmt.Errorf("no method %s", in.URI)
	}
	var sin api.ConnectorSyncInput
	if err := json.Unmarshal(in.Input, &sin); err != nil {
		return actionsapi.RunOutput{}, err
	}
	out, err := svc.FolderSync(ctx, sin)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	raw, _ := json.Marshal(out)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// sourcesWorld boots a real CWM on a temporary project with the sources pack,
// the source-connector kind, the local-folder connector, a connector another
// member contributes, and an ActionService routing a sync's call.
func sourcesWorld(t *testing.T) (context.Context, cwmapi.CWMService, api.SourcesService, string) {
	t.Helper()
	root := t.TempDir()
	foreign := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/wiki-page.yaml": {Data: []byte("name: wiki-page\nlabel: Wiki page\ndescription: A page of the team wiki.\nsync: ktw-wiki:wiki/source-sync\ncheck: ktw-wiki:wiki/source-check\n")},
	}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-wiki", foreign, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
			// Two study holders: the v2 asks for one source per connector
			// and for its sources' three sub-tabs
			// , the v1 does not.
			"object-types/study-v2.yaml": {Data: []byte("name: study-v2\ntree: node\ncapabilities: [one-source-per-connector, source-sub-tabs]\n")},
			"object-types/study-v1.yaml": {Data: []byte("name: study-v1\ntree: node\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(ctx context.Context) (actionsapi.ActionService, error) {
			return actionsRouting{ctx}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, cwm, svc, root
}

// A connector is a contribution: the one the sources extension serves and one
// another member publishes are both listed, with nothing in sources naming
// them.
func TestSourceConnectorKind(t *testing.T) {
	ctx, _, svc, _ := sourcesWorld(t)
	out, err := svc.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]api.ConnectorDecl{}
	for _, c := range out.Connectors {
		got[c.Name] = c
	}
	if c := got[api.FolderConnector]; c.Sync != "ktw-agents:sources/folder-sync" {
		t.Errorf("the local-folder connector is not listed with its sync method: %+v", out.Connectors)
	}
	if c := got["wiki-page"]; c.Sync != "ktw-wiki:wiki/source-sync" || c.Label != "Wiki page" {
		t.Errorf("a connector another member contributes is not listed: %+v", out.Connectors)
	}
	// The kind is declared once, by the sources extension, with its schema
	// and as executable: its entries name methods a sync runs.
	kinds, err := mbapi.KindsAt(ctx, mbapi.Q{Reach: mbapi.ReachSelf})
	if err != nil {
		t.Fatal(err)
	}
	decl, ok := kinds.Get(api.ContributionTypeSourceConnector)
	if !ok || decl.Schema != "ktw-agents:/data-type/source-connector" {
		t.Errorf("the source-connector kind is not declared with its schema: %+v %v", decl, ok)
	}
}

// An information source is read through its connector: the items land in the
// module's input/<source>/, each anchored as a source-document under the
// source, its facts recorded and its state synced; a source its connector
// cannot read is failed, with the error recorded.
func TestInformationSource(t *testing.T) {
	ctx, cwm, svc, root := sourcesWorld(t)
	ext := filepath.Join(root, "ext")
	for name, body := range map[string]string{"a.md": "# A\n\nAlpha.\n", "b.txt": "skip me", "sub/c.md": "# C\n\nGamma.\n"} {
		p := filepath.Join(ext, filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	create := func(local string, fields map[string]any) string {
		t.Helper()
		obj := cwmapi.NewContainerObject("m", local, api.InformationSourceType, "m", local, "")
		raw, _ := json.Marshal(fields)
		obj.SetFieldsJSON(raw)
		out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj})
		if err != nil {
			t.Fatalf("create %s: %v", local, err)
		}
		return out.Object.ID
	}
	id := create("specs", map[string]any{"connector": api.FolderConnector, "location": map[string]any{"path": "ext"},
		"selection": map[string]any{"include": []any{"*.md"}}, "sync": "manual"})

	out, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: id})
	if err != nil || !out.OK || out.State != api.SourceStateSynced || out.Items != 2 || len(out.Anchors) != 2 {
		t.Fatalf("sync: %v %+v", err, out)
	}
	for _, rel := range []string{"a.md", "sub/c.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(out.Folder), filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s not brought into %s: %v", rel, out.Folder, err)
		}
	}
	if !strings.HasSuffix(out.Folder, "/input/specs") {
		t.Errorf("items folder %q, want <module>/input/specs", out.Folder)
	}
	for _, a := range out.Anchors {
		doc, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: a})
		if err != nil || doc.Object.Type != api.SourceDocumentType || doc.Object.Parent != id {
			t.Errorf("anchor %s is not a source-document under the source: %v %+v", a, err, doc.Object)
		}
	}
	got, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
	var f api.InformationSourceFields
	_ = json.Unmarshal(got.Object.FieldsJSON(), &f)
	if f.Items != 2 || f.LastSynced == "" || f.Folder != out.Folder || f.Error != "" {
		t.Errorf("the sync's facts are not recorded: %+v", f)
	}
	if st := got.Object.Capabilities["lifecycle"]["state"]; st != api.SourceStateSynced {
		t.Errorf("state %v, want synced", st)
	}

	// A second sync of the same source is a sync again.
	if again, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: id}); err != nil || !again.OK {
		t.Fatalf("second sync: %v %+v", err, again)
	}

	// A source its connector cannot read fails, and says why - under another
	// module: a holder reads a connector through one source.
	cwmapp.Module(t, ctx, cwm, "m2")
	badObj := cwmapi.NewContainerObject("m2", "gone", api.InformationSourceType, "m2", "gone", "")
	badObj.Fields = map[string]any{"connector": api.FolderConnector, "location": map[string]any{"path": "nowhere"}}
	badOut, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m2", Object: badObj})
	if err != nil {
		t.Fatal(err)
	}
	bad := badOut.Object.ID
	fail, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: bad})
	if err != nil || fail.OK || fail.State != api.SourceStateFailed {
		t.Fatalf("a source that cannot be read: %v %+v", err, fail)
	}
	got, _ = cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m2", ID: bad})
	_ = json.Unmarshal(got.Object.FieldsJSON(), &f)
	if f.Error == "" || got.Object.Capabilities["lifecycle"]["state"] != api.SourceStateFailed {
		t.Errorf("a failed sync is not recorded: %+v %v", f, got.Object.Capabilities)
	}

	// A connector nobody contributes is a failed sync, named.
	none := create("none", map[string]any{"connector": "nobody"})
	if res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: none}); err != nil || res.OK || !strings.Contains(res.Message, "nobody") {
		t.Fatalf("an unknown connector: %v %+v", err, res)
	}
}

// SOURCES HOLDS THE SAVE GATE: an information source about to be
// created or updated is checked by its connector's `check` - refused with the
// connector's reason - so a connector's pack never gates a type it does not
// own; a connector declaring none saves as it is.
func TestAnInformationSourceIsCheckedByItsConnectorBeforeItIsSaved(t *testing.T) {
	ctx, cwm, _, _ := sourcesWorld(t)
	create := func(local string, fields map[string]any) error {
		t.Helper()
		obj := cwmapi.NewContainerObject("m", local, api.InformationSourceType, "m", local, "")
		raw, _ := json.Marshal(fields)
		obj.SetFieldsJSON(raw)
		_, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: obj})
		return err
	}
	err := create("nope", map[string]any{"connector": "wiki-page", "location": map[string]any{"page": "forbidden"}})
	if err == nil || !strings.Contains(err.Error(), "the wiki refuses the page forbidden") {
		t.Fatalf("a source its connector's check refuses: %v; want refused with the connector's reason", err)
	}
	if err := create("home", map[string]any{"connector": "wiki-page", "location": map[string]any{"page": "home"}}); err != nil {
		t.Errorf("a source its connector accepts: %v", err)
	}
	if err := create("files", map[string]any{"connector": api.FolderConnector, "location": map[string]any{"path": "ext"}}); err != nil {
		t.Errorf("a source of a connector declaring no check: %v", err)
	}
}
