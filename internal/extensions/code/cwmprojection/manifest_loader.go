// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucobj"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// LoadManifestInputs assembles a ManifestInputs from CWM for the
// module node identified by moduleID. Decodes typed Body fields
// per item kind. Returns an error if the module node is missing.
//
// Objects read anywhere in the module's subtree (a use-case under a goal or a
// feature is the module's as much as one filed at its root):
//   - use-case      (UC counters, deferred IDs)
//   - decision (open/resolved decisions)
//   - charter       (charter version)
//   - test-run      (latest one -> verify metadata)
//   - audit review  (the latest one)
//
// Items at the latest audit review:
//   - finding (open count by severity)
//
// Missing optional inputs degrade gracefully — a module with no
// audits gets m.LastAudit="" + m.AuditFindingsOpen=nil rather
// than an error.
func LoadManifestInputs(ctx stdctx.Context, cwm cwmapi.CWMService, moduleID string) (ManifestInputs, error) {
	if moduleID == "" {
		return ManifestInputs{}, fmt.Errorf("LoadManifestInputs: moduleID is required")
	}

	// 1. The module object itself
	module, err := cwmapi.ModuleOfObjectID(moduleID)
	if err != nil {
		return ManifestInputs{}, fmt.Errorf("owning module of %s: %w", moduleID, err)
	}
	nodeOut, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: moduleID})
	if err != nil {
		return ManifestInputs{}, fmt.Errorf("get module object %s: %w", moduleID, err)
	}
	// A module is what carries the `module` capability (a code-module does),
	// not an object whose type is named "module".
	isModule, err := IsModule(ctx, cwm, moduleID)
	if err != nil {
		return ManifestInputs{}, err
	}
	if !isModule {
		return ManifestInputs{}, fmt.Errorf("object %s (type %q) does not carry the %s capability", moduleID, nodeOut.Object.Type, moduleCapability)
	}
	mod, err := decodeModuleObject(nodeOut.Object)
	if err != nil {
		return ManifestInputs{}, fmt.Errorf("decode module object: %w", err)
	}

	in := ManifestInputs{Module: mod}

	// The list and the tree give the shape (ids, types, parents, capabilities);
	// every object whose fields or body is decoded below is read whole.
	whole := cwmapi.NewWholeReader(ctx, cwm, wholeTypes...)

	// 2. The module's whole subtree, read once: the counters below, the use
	// cases with their criteria and the audit bundles all come from it.
	treeOut, err := cwm.ObjectTree(ctx, cwmapi.ObjectTreeInput{Root: moduleID})
	if err != nil {
		return ManifestInputs{}, fmt.Errorf("walk module tree: %w", err)
	}
	all, err := whole.All(flattenTree(treeOut.Roots))
	if err != nil {
		return ManifestInputs{}, err
	}
	items := all

	var latestTestRun *gcapi.TestRun
	for i := range items {
		it := items[i]
		switch it.Type {
		case "use-case":
			uc, err := decodeUseCase(it)
			if err != nil {
				return ManifestInputs{}, fmt.Errorf("decode use-case %s: %w", it.ID, err)
			}
			in.UseCases = append(in.UseCases, uc)
		case "decision":
			od, err := decodeOpenDecision(it)
			if err != nil {
				return ManifestInputs{}, fmt.Errorf("decode decision %s: %w", it.ID, err)
			}
			in.OpenDecisions = append(in.OpenDecisions, od)
		case "charter":
			ch, err := decodeCharterVersion(it)
			if err != nil {
				return ManifestInputs{}, fmt.Errorf("decode charter %s: %w", it.ID, err)
			}
			if in.CharterVersion == "" || ch > in.CharterVersion {
				in.CharterVersion = ch
			}
		case "test-run":
			tr, err := decodeTestRun(it)
			if err != nil {
				return ManifestInputs{}, fmt.Errorf("decode test-run %s: %w", it.ID, err)
			}
			if latestTestRun == nil || tr.RanAt.After(latestTestRun.RanAt) {
				latestTestRun = &tr
			}
		}
	}
	in.LatestTestRun = latestTestRun

	// Sort use-cases by UCID for deterministic input → deterministic output.
	sort.Slice(in.UseCases, func(i, j int) bool {
		return in.UseCases[i].UCID < in.UseCases[j].UCID
	})
	sort.Slice(in.OpenDecisions, func(i, j int) bool {
		return in.OpenDecisions[i].ODID < in.OpenDecisions[j].ODID
	})

	// 3. The use cases with their criteria, statused by the latest test-run
	// verdicts — the reading the test plan, the charter's UC status and the
	// coverage block share. A use case's own fields never carry its status.
	verdicts, err := ucobj.LatestVerdicts(all)
	if err != nil {
		return ManifestInputs{}, fmt.Errorf("read test-run verdicts: %w", err)
	}
	if in.UCFiles, err = ucobj.FromObjects(all, module, verdicts); err != nil {
		return ManifestInputs{}, fmt.Errorf("rebuild use cases: %w", err)
	}

	// The audits are the reviews of the module held by the audit or of the
	// go-checks review type; the latest one's findings are under it (an
	// audit's) or under the module, last seen in it (a go-checks review's,
	// filed by the reviews service).
	var latestAudit *gcapi.AuditBundle
	for _, obj := range all {
		if obj.Type != gcapi.AuditReviewType || !heldByAudit(obj) {
			continue
		}
		ab := decodeAuditReview(obj)
		if latestAudit == nil || ab.RanAt.After(latestAudit.RanAt) {
			latestAudit = &ab
		}
	}
	in.LatestAudit = latestAudit
	if latestAudit != nil {
		for _, obj := range all {
			if obj.Type == gcapi.AuditFindingType && (obj.ParentID() == latestAudit.ID || obj.EdgeTo("seen-in") == latestAudit.ID) {
				in.OpenFindings = append(in.OpenFindings, decodeAuditFinding(obj))
			}
		}
	}

	return in, nil
}

// wholeTypes are the types the loaders decode from fields or body, so read
// whole: an index copy (ObjectList / ObjectTree) carries neither.
var wholeTypes = []string{
	"use-case", "acceptance-criterion", "decision", "charter", "test-run", gcapi.AuditReviewType, gcapi.AuditFindingType,
}

// heldByAudit reports a review the audit held (its reviewer).
// GoChecksReviewType is ktw-gocode's check review type: its reviews are the
// module's audits too.
const GoChecksReviewType = "go-checks"

func heldByAudit(o cwmapi.Object) bool {
	var f struct {
		Type      string   `json:"type"`
		Reviewers []string `json:"reviewers"`
	}
	if raw := o.FieldsJSON(); raw != nil && json.Unmarshal(raw, &f) == nil {
		if f.Type == GoChecksReviewType {
			return true
		}
		for _, r := range f.Reviewers {
			if r == gcapi.AuditReviewer {
				return true
			}
		}
	}
	return false
}

// decodeAuditReview maps an audit's review to the typed bundle: its summary is
// the review's, and when it ran is the run's own record — the instant the
// audit names its review by (`audit-2006-01-02T15-04-05`), else the review's
// `date`. Neither: RanAt stays zero, and the charter says no time.
func decodeAuditReview(o cwmapi.Object) gcapi.AuditBundle {
	p := partsOf(o)
	ab := gcapi.AuditBundle{ID: o.ID, ParentID: p.ParentID, Name: o.Name, Immutable: true}
	var f struct {
		Summary    string `json:"summary"`
		Date       string `json:"date"`
		ReviewedAt string `json:"reviewed_at"`
	}
	if p.Fields != nil && json.Unmarshal(p.Fields, &f) == nil {
		ab.SummaryMD = f.Summary
	}
	_, local, _ := cwmapi.SplitObjectID(o.ID)
	if instant, ok := strings.CutPrefix(local, "audit-"); ok {
		if t, err := time.Parse(auditInstantLayout, instant); err == nil {
			ab.RanAt = t
			return ab
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, f.ReviewedAt); err == nil {
		ab.RanAt = t
	} else if t, err := time.Parse(time.DateOnly, f.Date); err == nil {
		ab.RanAt = t
	}
	return ab
}

// auditInstantLayout is how the audit names its review after its run's instant
// (UTC): `audit-` then this.
const auditInstantLayout = "2006-01-02T15-04-05"

// flattenTree walks a nested object tree depth-first.
func flattenTree(nodes []cwmapi.ObjectTreeNode) []cwmapi.Object {
	var out []cwmapi.Object
	for _, n := range nodes {
		out = append(out, n.Object)
		out = append(out, flattenTree(n.Children)...)
	}
	return out
}

// --- typed body decoders ---------------------------------------------------

func decodeModuleObject(o cwmapi.Object) (gcapi.Module, error) {
	m := gcapi.Module{
		ID:          o.ID,
		Name:        o.Name,
		Slug:        o.Slug,
		Description: o.Description,
		ParentID:    o.Parent,
	}
	// A module object's id IS its slug (identity collapse) — so an unstamped
	// slug is not missing data, it is the id.
	if m.Slug == "" {
		m.Slug = o.ID
	}
	// A module's code - its kind, packages, facets and tiers - is the code
	// capability's; its links and last audit stay its module
	// group's.
	for _, g := range []struct {
		fields map[string]any
		code   bool
	}{{capGroup(o, codeCapability), true}, {capGroup(o, moduleCapability), false}} {
		for k, v := range g.fields {
			if isCodeField(k) != g.code {
				continue
			}
			switch k {
			case "kind":
				if s, ok := v.(string); ok {
					m.Kind = gcapi.ModuleKind(s)
				}
			case gcapi.CodePackagesField:
				// The module's code packages are the `module` capability's
				// declared `code_packages` (cap-module) — the one source of the
				// manifest's source_paths, of verify's packages and of gocode's.
				for _, p := range anyToStringSlice(v) {
					m.Facets = append(m.Facets, gcapi.Facet{Kind: gcapi.FacetCode, Path: p})
				}
			case "facets":
				// The other pointers (app, tests, …). A `code` entry here is NOT
				// read: code has one declared home, code_packages, and a second
				// one would let the two disagree.
				for _, f := range anyToFacets(v) {
					if f.Kind != gcapi.FacetCode {
						m.Facets = append(m.Facets, f)
					}
				}
			case "tier_current":
				if i, ok := anyToInt(v); ok {
					m.TierCurrent = i
				}
			case "tier_target":
				if i, ok := anyToInt(v); ok {
					m.TierTarget = i
				}
			case "tier_state":
				if s, ok := v.(string); ok {
					m.TierState = gcapi.TierState(s)
				}
			case "linked_goals":
				m.LinkedGoals = anyToStringSlice(v)
			case "last_audit_id":
				if s, ok := v.(string); ok {
					m.LastAuditID = s
				}
			}
		}
	}
	return m, nil
}

// isCodeField is whether a field is the code capability's.
func isCodeField(k string) bool {
	switch k {
	case "kind", gcapi.CodePackagesField, "facets", "tier_current", "tier_target", "tier_state":
		return true
	}
	return false
}

func decodeUseCase(o cwmapi.Object) (gcapi.UseCase, error) {
	p := partsOf(o)
	uc := gcapi.UseCase{
		ID:     p.ID,
		NodeID: p.ParentID,
		Title:  p.Title,
		// Identity collapse: the authored id IS the human UC-NNN. Empty for a
		// not-yet-collapsed object, which lets the body fallback below fire.
		UCID: authoredLocalID(p.ID),
		Body: p.Prose, // the DoD markdown is the object's body
	}
	if sev, ok := o.Capabilities["checkable"]["severity"].(string); ok {
		uc.Severity = gcapi.UCSeverity(sev) // how much it matters is the checkable's
	}
	if len(p.Fields) == 0 {
		return uc, nil
	}
	// The use-case type's fields; its id is the authored id, its severity the
	// checkable's.
	var body struct {
		StatusCache  gcapi.UCStatus  `json:"status_cache"`
		LayerTargets []gcapi.UCLayer `json:"layer_targets"`
	}
	if err := json.Unmarshal(p.Fields, &body); err != nil {
		return uc, fmt.Errorf("unmarshal use-case body: %w", err)
	}
	uc.StatusCache = body.StatusCache
	uc.FailDeferredBy = gcapi.DeferredByLocal(o.EdgeTo(gcapi.EdgeDeferredBy))
	uc.LayerTargets = body.LayerTargets
	return uc, nil
}

func decodeOpenDecision(o cwmapi.Object) (gcapi.OpenDecision, error) {
	p := partsOf(o)
	od := gcapi.OpenDecision{
		ID:     p.ID,
		NodeID: p.ParentID,
		Title:  p.Title,
	}
	var body struct {
		ODID       string `json:"od_id"`
		State      string `json:"state"`
		Resolution string `json:"resolution"`
	}
	// An authored decision has no such fields (its state is its
	// lifecycle's, its OD-NNN its authored id), and a substrate body shape
	// differs: a decode error degrades to untyped rather than failing the
	// projection on legacy data.
	if len(p.Fields) > 0 {
		_ = json.Unmarshal(p.Fields, &body)
	}
	od.ODID = body.ODID
	od.State = odState(o, body.State)
	od.Resolution = body.Resolution
	if od.ODID == "" {
		// Fallback: the authored local id, else the title.
		od.ODID = authoredLocalID(p.ID)
		if od.ODID == "" {
			od.ODID = p.Title
		}
	}
	return od, nil
}

// odState reads whether a decision is still open. It is resolved when any of
// its records says so: its lifecycle moved past `open` (the decision workflow:
// answered, superseded), the `decision` capability carries a `resolved` date
// (what the grader reads), or the code service's ODResolve wrote
// `state: resolved` in its fields. Otherwise it is an open question — an
// decision with no state recorded is open, as its type says.
func odState(o cwmapi.Object, fieldState string) gcapi.ODState {
	if gcapi.ODState(fieldState) == gcapi.ODStateResolved {
		return gcapi.ODStateResolved
	}
	if l, ok := o.Lifecycle(); ok && l.State != "" && l.State != string(gcapi.ODStateOpen) {
		return gcapi.ODStateResolved
	}
	if r, _ := o.Capabilities["decision"]["resolved"].(string); r != "" {
		return gcapi.ODStateResolved
	}
	return gcapi.ODStateOpen
}

func decodeCharterVersion(o cwmapi.Object) (string, error) {
	p := partsOf(o)
	if len(p.Fields) == 0 {
		return "", nil
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(p.Fields, &body); err != nil {
		return "", err
	}
	return body.Version, nil
}

func decodeTestRun(o cwmapi.Object) (gcapi.TestRun, error) {
	p := partsOf(o)
	tr := gcapi.TestRun{
		ID:     p.ID,
		NodeID: p.ParentID,
	}
	if len(p.Fields) == 0 {
		return tr, nil
	}
	var body struct {
		Module        string                          `json:"module"`
		RanAt         string                          `json:"ran_at"`
		LayerFilter   string                          `json:"layer_filter"`
		Totals        gcapi.TestRunTotals             `json:"totals"`
		TierNDoDMet   bool                            `json:"tier_n_dod_met"`
		TierNBlockers []string                        `json:"tier_n_blockers"`
		Verdicts      map[string]gcapi.TestRunVerdict `json:"verdicts"`
		ArchivedTo    string                          `json:"archived_to"`
	}
	if err := json.Unmarshal(p.Fields, &body); err != nil {
		return tr, fmt.Errorf("unmarshal test-run body: %w", err)
	}
	tr.Module = body.Module
	if t, err := parseRFC3339(body.RanAt); err == nil {
		tr.RanAt = t
	}
	tr.LayerFilter = body.LayerFilter
	tr.Totals = body.Totals
	tr.TierNDoDMet = body.TierNDoDMet
	tr.TierNBlockers = body.TierNBlockers
	tr.Verdicts = body.Verdicts
	tr.ArchivedTo = body.ArchivedTo
	return tr, nil
}

// decodeAuditFinding maps a finding of an audit to the typed finding: its
// fields, and its state from its lifecycle — open or addressed is open, closed
// is closed, accepted is deferred.
func decodeAuditFinding(o cwmapi.Object) gcapi.AuditFinding {
	p := partsOf(o)
	f := gcapi.AuditFinding{ID: p.ID, NodeID: p.ParentID, Title: p.Title, Body: p.Prose, State: gcapi.FindingStateOpen}
	var fields struct {
		Severity string `json:"severity"`
		FoundBy  string `json:"found_by"`
		Location string `json:"location"`
	}
	if p.Fields != nil && json.Unmarshal(p.Fields, &fields) == nil {
		f.Severity = gcapi.FindingSeverity(fields.Severity)
		f.Perspective = gcapi.AuditPerspective(fields.FoundBy)
		if fields.FoundBy == GoChecksReviewType {
			// A go-checks finding names its review type; its perspective is
			// its location (ktw-gocode's checkResult).
			f.Perspective = gcapi.AuditPerspective(fields.Location)
		}
	}
	_, local, _ := cwmapi.SplitObjectID(o.ID)
	_, parent, _ := cwmapi.SplitObjectID(p.ParentID)
	f.FindingID = strings.TrimPrefix(local, parent+"-")
	switch state, _ := capGroup(o, cwmapi.CapLifecycle)["state"].(string); state {
	case "closed":
		f.State = gcapi.FindingStateClosed
	case "accepted":
		f.State = gcapi.FindingStateDeferred
	}
	return f
}
