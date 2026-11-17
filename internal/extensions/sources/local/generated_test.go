// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// A document a generator marked is read like any other, marked « IA »
// (the exclusion deleted by): a sync brings the
// human page and the generated one, the anchor saying which was generated
// and by what; a picked parent brings its generated child too. No setting
// leaves it out.
func TestGeneratedDocumentsComeInLikeAnyOther(t *testing.T) {
	ctx, cwm, svc, root, _ := browseWorld(t)
	src := createSource(t, ctx, cwm, "docs", map[string]any{"connector": "gen-docs", "location": map[string]any{"site": "x"}})
	sync := func() api.SourceSyncOutput {
		t.Helper()
		res, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: src})
		if err != nil || !res.OK {
			t.Fatalf("sync: %v %+v", err, res)
		}
		return res
	}
	dir := filepath.Join(root, "modules", "m", "input", "docs")
	exists := func(name string) bool { _, err := os.Stat(filepath.Join(dir, name)); return err == nil }

	res := sync()
	if res.Items != 2 || !exists("generated.md") || !exists("human.md") {
		t.Fatalf("both pages come in: %+v", res)
	}
	var genAnchor map[string]any
	for _, id := range res.Anchors {
		if !strings.HasSuffix(id, "-generated") {
			continue
		}
		g, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
		if err != nil {
			t.Fatal(err)
		}
		_ = json.Unmarshal(g.Object.FieldsJSON(), &genAnchor)
	}
	if genAnchor["generated"] != true || genAnchor["generator"] != "katyoti" {
		t.Errorf("the generated page's anchor records its mark: %v", genAnchor)
	}

	// A picked parent brings its generated child like any other.
	g, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: src})
	var f map[string]any
	_ = json.Unmarshal(g.Object.FieldsJSON(), &f)
	f["selection"] = map[string]any{api.SelectionItems: []any{map[string]any{"key": "folder", "with_children": true}}}
	raw, _ := json.Marshal(f)
	o := g.Object
	o.SetFieldsJSON(raw)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o, Revision: g.Revision}); err != nil {
		t.Fatal(err)
	}
	if res = sync(); res.Items != 2 || !exists("generated.md") {
		t.Fatalf("a picked parent's generated child comes in: %+v", res)
	}
}
