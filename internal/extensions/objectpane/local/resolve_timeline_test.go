// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	coreschemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	schemasvc "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// filtering is a schema authority over one parsed data-type that filters its
// type-level extensions as the real one does (type and name, "*" any).
type filtering struct {
	schemasvc.SchemaService
	uri    string
	schema *coreschemaapi.Schema
	asked  []string
}

func (f *filtering) GetDataTypeWithExtensions(_ context.Context, in schemasvc.GetDataTypeWithExtensionsInput) (schemasvc.GetDataTypeOutput, error) {
	if in.URI != f.uri {
		return schemasvc.GetDataTypeOutput{}, schemasvc.ErrDataTypeNotFound
	}
	for _, r := range in.Extensions {
		f.asked = append(f.asked, r.Type+":"+r.Name)
	}
	clone := *f.schema
	clone.Types = map[string]*coreschemaapi.Type{}
	for n, t := range f.schema.Types {
		c := *t
		c.Extensions = nil
		for _, e := range t.Extensions {
			name, _ := e.Payload["name"].(string)
			for _, r := range in.Extensions {
				if (r.Type == "*" || r.Type == e.Kind) && (r.Name == "*" || r.Name == name) {
					c.Extensions = append(c.Extensions, e)
					break
				}
			}
		}
		clone.Types[n] = &c
	}
	return schemasvc.GetDataTypeOutput{Schema: &clone}, nil
}

// A timeline-lanes node of the committed stream-lane data type
// resolves to a schema keeping its ui-timeline-lanes variant and the ui-view
// variants it names: lane-header on StreamLane, card on ReleaseCard.
func TestResolvePanelKeepsTheTimelineViews(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(here), "..", "..", "planification", "contrib", "embed", "cwm", "data-types", "stream-lane.dsl.yaml")
	parsed, err := parser.ParseFile(context.Background(), coreschemaapi.ParseFileInput{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	sch := &filtering{uri: "ktw-agents:/data-type/stream-lane", schema: parsed.Schema}
	ctx, _, svc := paneWorld(t, nil,
		application.DeclareSimpleService[schemasvc.SchemaService](func(context.Context) (schemasvc.SchemaService, error) { return sch, nil }),
	)
	def := render.Def{Kind: "page", DSLTree: render.VBox(
		render.Node{"kind": "timeline-lanes", "schema": "ktw-agents:/data-type/stream-lane", "data": []any{}},
	)}
	out, err := svc.ResolvePanel(ctx, api.ResolvePanelInput{Def: def})
	if err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	b, _ := json.Marshal(out.Def)
	if err := json.Unmarshal(b, &tree); err != nil {
		t.Fatal(err)
	}
	node := tree["dslTree"].(map[string]any)["children"].([]any)[0].(map[string]any)
	types := node["schema"].(map[string]any)["types"].(map[string]any)
	kept := func(tn string) string {
		var got []string
		exts, _ := types[tn].(map[string]any)["extensions"].([]any)
		for _, e := range exts {
			m := e.(map[string]any)
			got = append(got, m["type"].(string)+":"+m["name"].(string))
		}
		sort.Strings(got)
		return strings.Join(got, ",")
	}
	if got := kept("StreamLane"); got != "ui-timeline-lanes:default,ui-view:lane-header" {
		t.Errorf("StreamLane keeps %s", got)
	}
	if got := kept("ReleaseCard"); got != "ui-view:card" {
		t.Errorf("ReleaseCard keeps %s", got)
	}
	if got := kept("BacklogItem"); got != "" {
		t.Errorf("BacklogItem keeps %s (no backlogView names its chip)", got)
	}
	sort.Strings(sch.asked)
	if got := strings.Join(sch.asked, ","); got != "ui-timeline-lanes:*,ui-view:card,ui-view:lane-header" {
		t.Errorf("fetches = %s", got)
	}
}
