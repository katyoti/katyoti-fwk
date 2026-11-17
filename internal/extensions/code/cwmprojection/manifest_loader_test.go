// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// mockCWM is a minimal in-memory implementation of cwmapi.CWMService
// covering only the four methods LoadManifestInputs calls:
// NodeGet, NodeTree, ItemList. All other methods are unimplemented
// (calling them in tests is a contract violation worth flagging).
//
// Lives in this test file rather than a shared fixture package so the
// projection package doesn't grow a test-only public surface.
type mockCWM struct {
	cwmapi.CWMService // embed to satisfy the interface; unimplemented methods panic if called
	objects           map[string]cwmapi.Object
}

func (m *mockCWM) ObjectGet(_ stdctx.Context, in cwmapi.ObjectGetInput) (cwmapi.ObjectGetOutput, error) {
	o, ok := m.objects[in.ID]
	if !ok {
		return cwmapi.ObjectGetOutput{}, errors.New("object not found: " + in.ID)
	}
	return cwmapi.ObjectGetOutput{Object: o}, nil
}

// typeBundles stands in for the declared types' capability bundles: the
// shipped `module` carries `module`, `software-module` `module` and `code`
var typeBundles = map[string][]string{
	"module":          {"container", "module"},
	"software-module": {"module", "code", "module-health"},
}

// ObjectEffectiveCapabilities mirrors the real resolution: the type's bundle
// then the instance's own groups.
func (m *mockCWM) ObjectEffectiveCapabilities(_ stdctx.Context, in cwmapi.ObjectEffectiveCapabilitiesInput) (cwmapi.ObjectEffectiveCapabilitiesOutput, error) {
	o, ok := m.objects[in.ID]
	if !ok {
		return cwmapi.ObjectEffectiveCapabilitiesOutput{}, errors.New("object not found: " + in.ID)
	}
	var out []cwmapi.CapabilityRef
	for _, c := range typeBundles[o.Type] {
		out = append(out, cwmapi.CapabilityRef{ID: c, Source: "type"})
	}
	for c := range o.Capabilities {
		out = append(out, cwmapi.CapabilityRef{ID: c, Source: "object"})
	}
	return cwmapi.ObjectEffectiveCapabilitiesOutput{Capabilities: out}, nil
}

// ObjectList mirrors the real index: filters AND together, results are sorted
// by id so the loader's own sorting is actually exercised rather than masked by
// map order.
func (m *mockCWM) ObjectList(_ stdctx.Context, in cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error) {
	var out []cwmapi.Object
	for _, o := range m.objects {
		if in.Type != "" && o.Type != in.Type {
			continue
		}
		if in.Parent != "" && o.ParentID() != in.Parent {
			continue
		}
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return cwmapi.ObjectListOutput{Objects: out, Total: len(out)}, nil
}

// ObjectTree builds the containment subtree from container.parent, the way the
// real index does — nested objects, not summaries.
func (m *mockCWM) ObjectTree(_ stdctx.Context, in cwmapi.ObjectTreeInput) (cwmapi.ObjectTreeOutput, error) {
	root, ok := m.objects[in.Root]
	if !ok {
		return cwmapi.ObjectTreeOutput{}, errors.New("object not found: " + in.Root)
	}
	var build func(id string) []cwmapi.ObjectTreeNode
	build = func(id string) []cwmapi.ObjectTreeNode {
		var kids []cwmapi.Object
		for _, o := range m.objects {
			if o.ParentID() == id {
				kids = append(kids, o)
			}
		}
		sort.Slice(kids, func(i, j int) bool { return kids[i].ID < kids[j].ID })
		var nodes []cwmapi.ObjectTreeNode
		for _, k := range kids {
			nodes = append(nodes, cwmapi.ObjectTreeNode{Object: k, Children: build(k.ID)})
		}
		return nodes
	}
	return cwmapi.ObjectTreeOutput{Roots: []cwmapi.ObjectTreeNode{{Object: root, Children: build(in.Root)}}}, nil
}

// obj builds a record object (content + placement) for the fixtures.
func obj(id, typ, parent, title string, body json.RawMessage) cwmapi.Object {
	o := cwmapi.Object{ID: id, Type: typ}
	o.Parent = parent
	o.Name = title
	o.SetFieldsJSON(body)
	return o
}

// deferredBy gives a use case the decision deferring it — an edge.
func deferredBy(o cwmapi.Object, od string) cwmapi.Object {
	o.SetEdgeTo(gcapi.EdgeDeferredBy, od)
	return o
}

// container builds a tree-placed object with a named capability group — the
// replacement for a node plus its TypeSpecific map.
func container(id, typ, parent, name string, group map[string]any, capName string) cwmapi.Object {
	o := cwmapi.Object{ID: id, Type: typ}
	o.Parent = parent
	o.Name = name
	if group != nil {
		if o.Capabilities == nil {
			o.Capabilities = map[string]map[string]any{}
		}
		o.Capabilities[capName] = group
	}
	return o
}

// rawJSON is a small helper to build a json.RawMessage from a
// struct or map for the test fixtures.
func rawJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return data
}

// TestLoadManifestInputs_HappyPath wires up a module node with a
// representative set of items + an audit (a review the audit held), and
// asserts every decoder lands the right typed value into the
// ManifestInputs.
func TestLoadManifestInputs_HappyPath(t *testing.T) {
	t.Parallel()
	const (
		moduleID = "mod-1"
		auditID  = "mod-1.audit-1"
	)
	ctx := stdctx.Background()

	mod := container(moduleID, "module", "", "acceptance", map[string]any{
		"kind": "library",
		// The declared field is the source; a `code` entry under the
		// undeclared `facets` is not read (it would be a second source).
		"code_packages": []any{"internal/extensions/acceptance/"},
		"facets": []any{
			map[string]any{"kind": "code", "path": "internal/ignored/"},
			map[string]any{"kind": "app", "path": "app/ktw-acceptance/"},
		},
		"tier_current": float64(0), // YAML/JSON decoded as float64
		"tier_target":  float64(1),
		"tier_state":   "verifying",
	}, codeCapability)
	// Its links stay the module group's; its code is the code group's
	//
	mod.Capabilities[moduleCapability] = map[string]any{"linked_goals": []any{"rock-solid.md"}}
	audit := obj(auditID, "review", moduleID, "audit-2026-05-18_09-00", rawJSON(t, map[string]any{
		"reviewers": []string{gcapi.AuditReviewer}, "summary": "Findings: 1 high.", "date": "2026-05-18",
	}))
	// A later review a person held is not an audit.
	other := obj("mod-1.rv-2", "review", moduleID, "design review", rawJSON(t, map[string]any{
		"reviewers": []string{"jdoe"}, "date": "2026-05-19",
	}))
	closedFinding := obj("mod-1.audit-1-lint-1", "finding", auditID, "a lint", rawJSON(t, map[string]any{
		"severity": "critical", "found_by": "lint",
	}))
	closedFinding.Capabilities = map[string]map[string]any{"lifecycle": {"state": "closed"}}

	uc1Body := rawJSON(t, map[string]any{
		"id":           "UC-001",
		"severity":     "core",
		"status_cache": "validated",
	})
	uc2Body := rawJSON(t, map[string]any{
		"id":           "UC-005",
		"severity":     "qol",
		"status_cache": "fail-deferred",
	})
	odBody := rawJSON(t, map[string]any{
		"od_id": "OD-002",
		"state": "open",
	})
	trBody := rawJSON(t, map[string]any{
		"module": "acceptance",
		"ran_at": "2026-05-18T10:00:00Z",
		"totals": map[string]int{"pass": 0, "fail": 0, "fail_deferred": 1, "no_test": 0, "uc_total": 1},
		// UC-001 has NO verdict: its body's status_cache ("validated") must
		// not make it pass anywhere a status is shown.
		"verdicts": map[string]any{"UC-005": map[string]any{"verdict": "fail-deferred"}},
	})
	chBody := rawJSON(t, map[string]any{
		"version": "0.1",
		"status":  "draft",
	})
	findingBody := rawJSON(t, map[string]any{
		"severity": "high",
		"found_by": "security",
	})

	// Ids are MODULE-QUALIFIED, the collapsed shape: rather than a
	// uuid plus a separate uc_id. The tree is built from container.parent, so no
	// separate tree fixture is needed — placement IS the tree.
	m := &mockCWM{objects: map[string]cwmapi.Object{
		moduleID:     mod,
		auditID:      audit,
		"mod-1.rv-2": other,
		// A finding of the person's review is not one of the audit's.
		"mod-1.rv-2-fd-1":      obj("mod-1.rv-2-fd-1", "finding", "mod-1.rv-2", "naming", rawJSON(t, map[string]any{"severity": "low"})),
		"mod-1.audit-1-lint-1": closedFinding,
		"mod-1.UC-001":         obj("mod-1.UC-001", "use-case", moduleID, "UC-001", uc1Body),
		"mod-1.UC-005":         deferredBy(obj("mod-1.UC-005", "use-case", moduleID, "UC-005", uc2Body), "mod-1.OD-002"),
		"mod-1.OD-002":         obj("mod-1.OD-002", "decision", moduleID, "Pick X", odBody),
		"mod-1.run-1":          obj("mod-1.run-1", "test-run", moduleID, "verify 2026-05-18", trBody),
		"mod-1.charter":        obj("mod-1.charter", "charter", moduleID, "charter v0.1", chBody),
		"mod-1.a-rule":         obj("mod-1.a-rule", "rule", moduleID, "ignored — wrong kind", nil), // skipped
		"mod-1.audit-1-sec-1":  obj("mod-1.audit-1-sec-1", "finding", auditID, "sec-1", findingBody),
	}}

	in, err := LoadManifestInputs(ctx, m, moduleID)
	if err != nil {
		t.Fatalf("LoadManifestInputs: %v", err)
	}

	// Module decoded
	if in.Module.Name != "acceptance" {
		t.Errorf("Module.Name = %q", in.Module.Name)
	}
	if in.Module.Kind != gcapi.ModuleKindLibrary {
		t.Errorf("Module.Kind = %q, want library", in.Module.Kind)
	}
	if in.Module.TierTarget != 1 || in.Module.TierCurrent != 0 {
		t.Errorf("Tier = (%d, %d)", in.Module.TierCurrent, in.Module.TierTarget)
	}
	if in.Module.TierState != gcapi.TierStateVerifying {
		t.Errorf("TierState = %q, want verifying", in.Module.TierState)
	}
	if got := in.Module.FacetPaths(gcapi.FacetCode); len(got) != 1 || got[0] != "internal/extensions/acceptance/" {
		t.Errorf("code facets = %v, want only code_packages' internal/extensions/acceptance/", got)
	}
	if got := in.Module.FirstFacet(gcapi.FacetApp); got != "app/ktw-acceptance/" {
		t.Errorf("app facet = %q, want the facets group's app/ktw-acceptance/", got)
	}

	// UCs decoded (rule item skipped)
	if len(in.UseCases) != 2 {
		t.Errorf("UseCases len = %d, want 2 (rule item should be skipped)", len(in.UseCases))
	}
	// Sorted by UCID
	if in.UseCases[0].UCID != "UC-001" || in.UseCases[1].UCID != "UC-005" {
		t.Errorf("UseCases not sorted: %+v", []string{in.UseCases[0].UCID, in.UseCases[1].UCID})
	}
	if in.UseCases[0].StatusCache != gcapi.UCStatusValidated {
		t.Errorf("UC-001 StatusCache = %q", in.UseCases[0].StatusCache)
	}
	if in.UseCases[1].FailDeferredBy != "OD-002" {
		t.Errorf("UC-005 FailDeferredBy = %q", in.UseCases[1].FailDeferredBy)
	}

	// OD decoded
	if len(in.OpenDecisions) != 1 || in.OpenDecisions[0].ODID != "OD-002" {
		t.Errorf("OpenDecisions = %+v", in.OpenDecisions)
	}

	// Charter version captured
	if in.CharterVersion != "0.1" {
		t.Errorf("CharterVersion = %q, want 0.1", in.CharterVersion)
	}

	// Latest test-run captured
	if in.LatestTestRun == nil {
		t.Fatal("LatestTestRun is nil")
	}
	if got := in.LatestTestRun.RanAt.Format(time.DateOnly); got != "2026-05-18" {
		t.Errorf("LatestTestRun.RanAt formatted = %q, want 2026-05-18", got)
	}

	// The audit is the review the audit held — not the person's later one.
	if in.LatestAudit == nil {
		t.Fatal("LatestAudit is nil")
	}
	if in.LatestAudit.Name != "audit-2026-05-18_09-00" || in.LatestAudit.SummaryMD != "Findings: 1 high." ||
		!in.LatestAudit.RanAt.Equal(time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("LatestAudit = %+v", in.LatestAudit)
	}

	// Its findings decoded: severity, perspective and id from the finding, the
	// state from its lifecycle.
	if len(in.OpenFindings) != 2 {
		t.Fatalf("OpenFindings len = %d, want 2", len(in.OpenFindings))
	}
	byID := map[string]gcapi.AuditFinding{}
	for _, f := range in.OpenFindings {
		byID[f.FindingID] = f
	}
	if f := byID["sec-1"]; f.Severity != gcapi.FindingSeverityHigh || f.Perspective != "security" || f.State != gcapi.FindingStateOpen {
		t.Errorf("sec-1 = %+v; want high, security, open", f)
	}
	if f := byID["lint-1"]; f.State != gcapi.FindingStateClosed {
		t.Errorf("lint-1 = %+v; want closed (its lifecycle)", f)
	}

	// Now build the manifest from these inputs and assert the
	// end-to-end pipeline produces expected counters.
	manifest := BuildManifest(in)
	if c := manifest.Coverage; c == nil || c.UCTotal != 2 || c.UCByStatus["pass"] != 0 ||
		c.UCByStatus["fail-deferred"] != 1 || c.UCByStatus["no-test"] != 1 {
		t.Errorf("manifest coverage: %+v", manifest.Coverage)
	}
	if manifest.LastAudit != "audit-2026-05-18_09-00" {
		t.Errorf("manifest LastAudit = %q", manifest.LastAudit)
	}
}

// TestLoadManifestInputs_EmptyID rejects the obvious mistake.
func TestLoadManifestInputs_EmptyID(t *testing.T) {
	t.Parallel()
	_, err := LoadManifestInputs(stdctx.Background(), &mockCWM{}, "")
	if err == nil {
		t.Fatal("expected error on empty moduleID; got nil")
	}
}

// TestLoadManifestInputs_NonModuleNodeRejected catches the case
// where the caller passes a project / feature / etc. UUID.
func TestLoadManifestInputs_NonModuleNodeRejected(t *testing.T) {
	t.Parallel()
	m := &mockCWM{objects: map[string]cwmapi.Object{
		"x": container("x", "project", "", "Proj", nil, ""),
	}}
	_, err := LoadManifestInputs(stdctx.Background(), m, "x")
	if err == nil {
		t.Fatal("expected error decoding non-module node; got nil")
	}
}
