// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// apps searches applications and brings each as data carrying its code;
// ops has no free search: it browses, and reads from what apps kept by
// their code.
const (
	fromApps = `name: apps
label: Applications
description: The applications of the map.
sync: ktw-apps:apps/source-sync
search: ktw-apps:apps/search
`
	fromOps = `name: ops
label: Operations
description: The monitored entities of the kept applications.
sync: ktw-ops:ops/source-sync
browse: ktw-ops:ops/browse
from: {connector: apps, attr: code}
`
	// deps picks only from what apps kept (no search, no browse); files
	// only browses (no search, no from).
	fromDeps = `name: deps
label: Dependencies
description: The dependencies of the kept applications.
sync: ktw-ops:deps/source-sync
from: {connector: apps, attr: code}
`
	fromFiles = `name: files
label: Files
description: The files of a share.
sync: ktw-ops:files/source-sync
browse: ktw-ops:files/browse
`
)

// fromAppTitles is each application's title by its code: what a following
// source names it by.
var fromAppTitles = map[string]string{"ERP": "Gestion ERP", "CRM": "Relation client"}

// fromActions serves apps and ops, recording what ops' browse and sync
// received from apps.
type fromActions struct {
	mu   *sync.Mutex
	seen *[]*api.FromItems
}

func (fromActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a fromActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	var value any
	switch in.URI {
	case "ktw-apps:apps/source-sync":
		var sin api.ConnectorSyncInput
		_ = json.Unmarshal(in.Input, &sin)
		out := api.ConnectorSyncOutput{Items: []api.ConnectorItem{}}
		for _, code := range []string{"ERP", "CRM"} {
			name := strings.ToLower(code)
			if err := os.WriteFile(filepath.Join(sin.Dest, name+".data.json"), []byte(`{"code":"`+code+`"}`), 0o644); err != nil {
				return actionsapi.RunOutput{}, err
			}
			out.Items = append(out.Items, api.ConnectorItem{Kind: "app", Data: name + ".data.json", URI: "apps://" + name, Title: fromAppTitles[code], Key: name})
		}
		value = out
	case "ktw-apps:apps/search":
		value = api.ConnectorSearchOutput{Items: []api.PickItem{}}
	case "ktw-ops:ops/browse":
		var bin api.ConnectorBrowseInput
		_ = json.Unmarshal(in.Input, &bin)
		a.mu.Lock()
		*a.seen = append(*a.seen, bin.From)
		a.mu.Unlock()
		value = api.ConnectorBrowseOutput{Items: []api.BrowseItem{}}
	case "ktw-ops:ops/source-sync":
		var sin api.ConnectorSyncInput
		_ = json.Unmarshal(in.Input, &sin)
		a.mu.Lock()
		*a.seen = append(*a.seen, sin.From)
		a.mu.Unlock()
		if err := os.WriteFile(filepath.Join(sin.Dest, "erp-host.md"), []byte("# ERP host\n"), 0o644); err != nil {
			return actionsapi.RunOutput{}, err
		}
		value = api.ConnectorSyncOutput{Items: []api.ConnectorItem{{Text: "erp-host.md", URI: "ops://erp-host", Title: "ERP host"}}}
	case "ktw-agents:sources/source-check":
		svc, err := application.UseService[api.SourcesService](ctx)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		var cin api.SourceCheckInput
		if err := json.Unmarshal(in.Input, &cin); err != nil {
			return actionsapi.RunOutput{}, err
		}
		if value, err = svc.SourceCheck(ctx, cin); err != nil {
			return actionsapi.RunOutput{}, err
		}
	default:
		return actionsapi.RunOutput{}, fmt.Errorf("no method %s", in.URI)
	}
	raw, _ := json.Marshal(value)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// : a connector may pick with no free search - by browsing, or
// from what another source kept. Such a connector is a picker (Pickers - the
// source-set field's list - and the proposal), with no search and its
// browse; its browse and its sync receive the attribute's values on the
// items the carrier's source of the other connector brought.
func TestAConnectorPicksWithoutAFreeSearch(t *testing.T) {
	root := t.TempDir()
	seen := &[]*api.FromItems{}
	apps := loader.NewEmbedFSLoader(fstest.MapFS{"connectors/apps.yaml": {Data: []byte(fromApps)}}, ".")
	ops := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/ops.yaml":   {Data: []byte(fromOps)},
		"connectors/deps.yaml":  {Data: []byte(fromDeps)},
		"connectors/files.yaml": {Data: []byte(fromFiles)},
	}, ".")
	ctx, cwm := cwmapp.NewAt(t, root,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		contributeConnectors("ktw-apps", apps, "connectors"),
		contributeConnectors("ktw-ops", ops, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return fromActions{mu: &sync.Mutex{}, seen: seen}, nil
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

	// A picker everywhere: Pickers (what the source-set field lists) and
	// the proposal, with no search and its browse.
	list, err := svc.Pickers(ctx, api.PickersInput{})
	if err != nil {
		t.Fatal(err)
	}
	views := map[string]*api.ConnectorPickerView{}
	for i, c := range list.Connectors {
		views[c.Name] = &list.Connectors[i]
	}
	if v := views["ops"]; v == nil || v.Search != "" || v.Browse != api.SourceBrowseURI || v.From == nil || v.From.Connector != "apps" {
		t.Fatalf("a browse and from connector is a picker with no search and its browse: %+v", v)
	}
	if v := views["deps"]; v == nil || v.Search != "" || v.Browse != "" || v.From == nil || v.From.Connector != "apps" || v.From.Attr != "code" {
		t.Fatalf("a from-only connector is a picker with no search, no browse and its from: %+v", v)
	}
	if v := views["files"]; v == nil || v.Search != "" || v.Browse != api.SourceBrowseURI || v.From != nil {
		t.Fatalf("a browse-only connector is a picker with no search and its browse: %+v", v)
	}
	wiz, err := svc.ProposalWizard(ctx, api.ProposalWizardInput{Object: "m"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(wiz.Page)
	for _, name := range []string{"ops", "deps", "files"} {
		if !strings.Contains(string(raw), `"id":"`+name+`"`) {
			t.Errorf("the proposal holds a step for %s: %s", name, raw)
		}
	}

	// The create page's note under each card (mock-up v6
	// view 11), drawn only while its card is chosen and read from the
	// declaration, a callout box with its icon: apps proposes from the holder's
	// keywords (blue, sparkles); deps reads from apps - gray while the holder
	// has no apps source -; files is where files are dropped (gray).
	notes := func() map[string]map[string]any {
		t.Helper()
		out, err := svc.NewSourcePage(ctx, api.NewSourcePageInput{Parent: "m"})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(out.DSLTree)
		var tree map[string]any
		_ = json.Unmarshal(raw, &tree)
		got := map[string]map[string]any{}
		for _, c := range tree["children"].([]any) {
			n := c.(map[string]any)
			if id, _ := n["id"].(string); strings.HasPrefix(id, "new-source-note-") {
				got[strings.TrimPrefix(id, "new-source-note-")] = n
			}
		}
		return got
	}
	note := notes()
	for name, want := range map[string]string{
		"apps":  "Dès sa création, elle propose des éléments d'après les mots-clés du module",
		"deps":  "Elle lit depuis une source Applications du module : elle proposera d'après ce que celle-ci tient. Il n'y en a pas encore.",
		"files": "Vous y déposez vos fichiers : elle ne propose rien.",
	} {
		n := note[name]
		if n == nil || n["kind"] != "box" || n["when"] != "connector="+name || !strings.Contains(noteText(n), want) {
			t.Errorf("the note under %s, drawn only while it is chosen, says %q: %+v", name, want, n)
		}
	}
	if note["deps"]["callout"] != "gray" || note["files"]["callout"] != "gray" || note["apps"]["callout"] != "blue" || noteIcon(note["apps"]) != "sparkles" {
		t.Errorf("a from connector's note is gray while the holder has no source to read from, a proposing one is not: %+v", note)
	}

	// Nothing kept yet: its browse receives from with no value.
	if _, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{Connector: "ops", Object: "m"}); err != nil {
		t.Fatal(err)
	}
	// The carrier keeps two applications: ops' browse and sync receive their codes.
	appsSrc := createSource(t, ctx, cwm, "apps-src", map[string]any{"connector": "apps"})
	if res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: appsSrc}); err != nil || !res.OK {
		t.Fatalf("apps sync: %+v %v", res, err)
	}
	if n := notes()["deps"]; n == nil || n["callout"] != "blue" || noteIcon(n) != "link" ||
		!regexp.MustCompile(`^Elle lira depuis « [^»]+ » : le code de ses éléments \((ERP, CRM|CRM, ERP)\) est ce qu'elle suit\. Elle ne cherche jamais par les mots-clés\.$`).MatchString(noteText(n)) {
		t.Errorf("once the holder's apps source keeps applications, deps' note names it and the codes it follows: %+v", n)
	}
	if _, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{Connector: "ops", Object: "m"}); err != nil {
		t.Fatal(err)
	}
	opsSrc := createSource(t, ctx, cwm, "ops-src", map[string]any{"connector": "ops"})
	if res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: opsSrc}); err != nil || !res.OK {
		t.Fatalf("ops sync: %+v %v", res, err)
	}
	if len(*seen) != 3 {
		t.Fatalf("ops was browsed twice and synced once: %d", len(*seen))
	}
	if f := (*seen)[0]; f == nil || f.Connector != "apps" || f.Attr != "code" || len(f.Values) != 0 || len(f.Titles) != 0 {
		t.Errorf("before anything is kept, from names apps by code with no value: %+v", f)
	}
	for i, f := range (*seen)[1:] {
		if f == nil || strings.Join(sorted(f.Values), ",") != "CRM,ERP" {
			t.Errorf("call %d receives the codes of the applications kept: %+v", i+2, f)
		}
		// Each code comes with its application's title, so the following
		// source can name what it follows.
		for code, title := range fromAppTitles {
			if f == nil || f.Titles[code] != title || f.Title(code) != title {
				t.Errorf("call %d names %s by its application's title %q: %+v", i+2, code, title, f)
			}
		}
	}

	// SourcesOf answers, for the source following apps, the codes it
	// follows with their titles; a source whose connector declares no from
	// answers none.
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range of.Sources {
		switch src.ID {
		case opsSrc:
			if src.From == nil || src.From.Connector != "apps" || src.From.Attr != "code" || strings.Join(sorted(src.From.Values), ",") != "CRM,ERP" || src.From.Titles["ERP"] != fromAppTitles["ERP"] {
				t.Errorf("SourcesOf answers the codes the ops source follows: %+v", src.From)
			}
		case appsSrc:
			if src.From != nil {
				t.Errorf("a source whose connector declares no from follows nothing: %+v", src.From)
			}
		}
	}
}

func sorted(v []string) []string {
	out := append([]string{}, v...)
	sort.Strings(out)
	return out
}

// noteText is the text of a create page's connector note (a callout box
// holding its icon and its text).
func noteText(n map[string]any) string {
	row, _ := n["children"].([]any)
	if len(row) == 0 {
		return ""
	}
	kids, _ := row[0].(map[string]any)["children"].([]any)
	if len(kids) < 2 {
		return ""
	}
	return fmt.Sprint(kids[1].(map[string]any)["content"])
}

// noteIcon is the icon of a create page's connector note.
func noteIcon(n map[string]any) string {
	row, _ := n["children"].([]any)
	if len(row) == 0 {
		return ""
	}
	kids, _ := row[0].(map[string]any)["children"].([]any)
	if len(kids) == 0 {
		return ""
	}
	return fmt.Sprint(kids[0].(map[string]any)["icon"])
}
