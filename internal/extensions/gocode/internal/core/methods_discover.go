// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/discovery"
)

// ModuleDiscover scans Go code under Input.Path and seeds CWM items:
//
//   - Auto-commits one `code-symbol` item per exported identifier (per
//     §15.12 LOCKED decision — AST-derived, deterministic).
//   - Proposes (does NOT auto-commit) one `use-case` per test-cluster.
//   - Proposes one `test-case` per discovered Test* function.
//   - Proposes cross-module `dependency` edges from non-stdlib imports.
//
// `target_module_id` either extends an existing module (default
// when set) or — when empty — currently returns an error. We don't
// auto-create modules from this verb: the architect persona should
// resolve module decomposition decisions before discovery seeds
// items, per modules/code-gocode/concepts.md §3 retro-engineering flow.
//
// Dry-run mode returns the proposals + a 0 committed_code_symbols
// count without touching CWM. Non-dry-run honours
// auto_commit_code_symbols (defaults to true).
func (s *GocodeServiceImpl) ModuleDiscover(ctx stdctx.Context, in api.ModuleDiscoverInput) (api.ModuleDiscoverOutput, error) {
	if in.Path == "" {
		return api.ModuleDiscoverOutput{}, fmt.Errorf("%w: ModuleDiscover: path is required", api.ErrInvalidInput)
	}
	if in.TargetModuleID == "" {
		return api.ModuleDiscoverOutput{}, fmt.Errorf("%w: ModuleDiscover: target_module_id is required (module auto-creation is the architect persona's job — call gocode.module-create first)", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ModuleDiscoverOutput{}, fmt.Errorf("ModuleDiscover: resolve CWMService: %w", err)
	}

	// Scan Go source — pure data, no CWM writes yet.
	inv, err := discovery.Discover(ctx, in.Path, discovery.DefaultOptions())
	if err != nil {
		return api.ModuleDiscoverOutput{}, fmt.Errorf("ModuleDiscover: scan %s: %w", in.Path, err)
	}

	out := api.ModuleDiscoverOutput{
		ModuleID: in.TargetModuleID,
	}
	if inv.LayoutHints != nil && inv.LayoutHints.SuggestedKind != "" && inv.LayoutHints.SuggestedKind != "unclear" {
		out.SuggestedKind = api.ModuleKind(inv.LayoutHints.SuggestedKind)
	}

	// Compose proposals.
	if in.ProposeUseCases {
		out.ProposedUseCases = proposeUCsFromInventory(inv)
	}
	if in.ProposeTestCases {
		out.ProposedTestCases = proposeTCsFromInventory(inv)
	}
	out.ProposedDependencies = proposeDepsFromInventory(inv)

	// Auto-commit code-symbol items unless overridden.
	// Default true per the API doc; dry_run shortcircuits the write.
	if in.AutoCommitCodeSymbols && !in.DryRun {
		committed, warnings, err := commitCodeSymbols(ctx, cwm, in.TargetModuleID, inv)
		if err != nil {
			return out, fmt.Errorf("ModuleDiscover: commit code-symbols: %w", err)
		}
		out.CommittedCodeSymbols = committed
		out.Warnings = append(out.Warnings, warnings...)
	}

	if in.DryRun {
		out.Warnings = append(out.Warnings, fmt.Sprintf(
			"dry_run=true: %d code-symbols would have been auto-committed",
			len(inv.PublicSymbols)))
	}
	return out, nil
}

// UCProposeFromTests is the focused retro-engineering verb that
// clusters existing Test* functions by name prefix and proposes one
// use-case per cluster. The body is a placeholder DoD listing every
// L1 test in the cluster — the specifier persona then refines the
// title + severity + DoD per the cluster's intent.
//
// v1 limitation: clustering uses simple prefix-based grouping. The
// spec's "Severity heuristic" (security keywords flip severity to
// security) is implemented.
func (s *GocodeServiceImpl) UCProposeFromTests(ctx stdctx.Context, in api.UCProposeFromTestsInput) (api.UCProposeFromTestsOutput, error) {
	if in.ModuleID == "" {
		return api.UCProposeFromTestsOutput{}, fmt.Errorf("%w: UCProposeFromTests: module_id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.UCProposeFromTestsOutput{}, fmt.Errorf("UCProposeFromTests: resolve CWMService: %w", err)
	}

	// Resolve the module's code_packages so we know where to scan.
	discModule, err := cwmapi.ModuleOfObjectID(in.ModuleID)
	if err != nil {
		return api.UCProposeFromTestsOutput{}, fmt.Errorf("UCProposeFromTests: owning module: %w", err)
	}
	getOut, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: discModule, ID: in.ModuleID})
	if err != nil {
		return api.UCProposeFromTestsOutput{}, fmt.Errorf("UCProposeFromTests: ObjectGet: %w", err)
	}
	if ok, err := projection.IsModule(ctx, cwm, in.ModuleID); err != nil {
		return api.UCProposeFromTestsOutput{}, fmt.Errorf("UCProposeFromTests: %w", err)
	} else if !ok {
		return api.UCProposeFromTestsOutput{}, fmt.Errorf("%w: UCProposeFromTests: object %s (type %q) is not a module",
			api.ErrInvalidInput, in.ModuleID, getOut.Object.Type)
	}

	pkgs := codePackagesOf(getOut.Object)
	if len(pkgs) == 0 {
		return api.UCProposeFromTestsOutput{
			Proposed: nil,
		}, nil
	}

	// Discover from the first code_package root. v1 only scans one
	// path per call; a follow-up could union across all entries.
	root := pkgs[0]
	if root == "" {
		return api.UCProposeFromTestsOutput{}, nil
	}
	inv, err := discovery.Discover(ctx, root, discovery.DefaultOptions())
	if err != nil {
		return api.UCProposeFromTestsOutput{}, fmt.Errorf("UCProposeFromTests: discover %s: %w", root, err)
	}
	return api.UCProposeFromTestsOutput{
		Proposed: proposeUCsFromInventory(inv),
	}, nil
}

// FeatureReverseEngineer proposes seed `feature` items by analysing
// the cross-module public-API graph. A feature is "a coherent
// capability provided by one or more module APIs and consumed by
// another module or sidecar." The heuristic in v1: a public symbol
// imported by ≥ 2 distinct downstream modules suggests a feature.
//
// Lower-confidence than ModuleDiscover (features are judgment-based)
// — every proposal carries confidence=low + a HintedBy field listing
// the consuming modules. Never auto-commits.
func (s *GocodeServiceImpl) FeatureReverseEngineer(ctx stdctx.Context, in api.FeatureReverseEngineerInput) (api.FeatureReverseEngineerOutput, error) {
	if in.ProjectID == "" {
		return api.FeatureReverseEngineerOutput{}, fmt.Errorf("%w: FeatureReverseEngineer: project_id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.FeatureReverseEngineerOutput{}, fmt.Errorf("FeatureReverseEngineer: resolve CWMService: %w", err)
	}

	// Walk the project tree, find every module child, gather its
	// code_packages, intersect import graphs. v1 limitation: this
	// requires every module to have non-empty code_packages set.
	tree, err := cwm.ObjectTree(ctx, cwmapi.ObjectTreeInput{Root: in.ProjectID})
	if err != nil {
		return api.FeatureReverseEngineerOutput{}, fmt.Errorf("FeatureReverseEngineer: ObjectTree: %w", err)
	}
	type moduleHit struct {
		nodeID string
		name   string
		pkgs   []string
	}
	var modules []moduleHit
	// A module is what carries the `module` capability in force (a code-module
	// does) — the index's own effective-capability filter, one call for the
	// project rather than one per tree entry.
	listed, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: codeCapability})
	if err != nil {
		return api.FeatureReverseEngineerOutput{}, fmt.Errorf("FeatureReverseEngineer: list modules: %w", err)
	}
	isModule := make(map[string]bool, len(listed.Objects))
	for _, o := range listed.Objects {
		isModule[o.ID] = true
	}
	// The tree's index copies carry the capabilities (code_packages is the
	// module capability's), so no per-entry fetch: fields and body are not read.
	for _, obj := range flattenObjectTree(tree.Roots) {
		if !isModule[obj.ID] {
			continue
		}
		modules = append(modules, moduleHit{
			nodeID: obj.ID,
			name:   obj.Name,
			pkgs:   codePackagesOf(obj),
		})
	}

	if len(modules) < 2 {
		return api.FeatureReverseEngineerOutput{}, nil
	}

	// Build (module-name → exported-pkg-path) and (consumer-module →
	// imported-pkg-paths) maps via Discover on each module's first
	// code_package root.
	exportedBy := map[string][]string{} // module-name → []public-import-paths
	importedBy := map[string][]string{} // module-name → []imported-paths
	for _, m := range modules {
		if len(m.pkgs) == 0 {
			continue
		}
		inv, err := discovery.Discover(ctx, m.pkgs[0], discovery.DefaultOptions())
		if err != nil {
			continue
		}
		for _, p := range inv.Packages {
			if !p.IsCmd && p.PublicExports > 0 {
				exportedBy[m.name] = append(exportedBy[m.name], p.Path)
			}
		}
		for _, imp := range inv.Imports {
			if imp.IsLocal {
				importedBy[m.name] = append(importedBy[m.name], imp.Imported)
			}
		}
	}

	// For each producer module, find its consumers (modules whose
	// imports overlap with the producer's exports). When ≥ 2
	// distinct consumers exist, propose a feature.
	var proposed []api.ProposedFeature
	for producerName, producerExports := range exportedBy {
		consumers := map[string]bool{}
		for consumerName, consumerImports := range importedBy {
			if consumerName == producerName {
				continue
			}
			for _, imp := range consumerImports {
				for _, exp := range producerExports {
					if strings.Contains(imp, exp) {
						consumers[consumerName] = true
					}
				}
			}
		}
		if len(consumers) >= 2 {
			var consumerList []string
			for c := range consumers {
				consumerList = append(consumerList, c)
			}
			sort.Strings(consumerList)
			proposed = append(proposed, api.ProposedFeature{
				Name:        producerName + " capability",
				Description: fmt.Sprintf("Consumed by %d downstream modules: %s", len(consumerList), strings.Join(consumerList, ", ")),
				Modules:     []string{producerName},
				Confidence:  "low",
			})
		}
	}
	sort.Slice(proposed, func(i, j int) bool {
		return proposed[i].Name < proposed[j].Name
	})
	return api.FeatureReverseEngineerOutput{Proposed: proposed}, nil
}

// ── proposal helpers ────────────────────────────────────────────────────

// proposeUCsFromInventory clusters Test* functions by the prefix
// before the first underscore (or first 3-4 capitalised words) and
// produces one ProposedUseCase per cluster. The DoD body lists every
// L1 test in the cluster as a starting reference for the specifier.
func proposeUCsFromInventory(inv *discovery.Inventory) []api.ProposedUseCase {
	type cluster struct {
		key   string
		tests []*discovery.TestFunctionInfo
	}
	byKey := map[string]*cluster{}
	for _, t := range inv.TestFunctions {
		if t.Kind != discovery.TestKindTest {
			continue
		}
		key := testClusterKey(t.Name)
		c, ok := byKey[key]
		if !ok {
			c = &cluster{key: key}
			byKey[key] = c
		}
		c.tests = append(c.tests, t)
	}
	var keys []string
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []api.ProposedUseCase
	for _, k := range keys {
		c := byKey[k]
		// Severity heuristic per spec.
		severity := api.UCSeverityCore
		lowerKey := strings.ToLower(k)
		switch {
		case strings.Contains(lowerKey, "auth"),
			strings.Contains(lowerKey, "permission"),
			strings.Contains(lowerKey, "sanitize"),
			strings.Contains(lowerKey, "secur"):
			severity = api.UCSeveritySecurity
		case strings.Contains(lowerKey, "display"),
			strings.Contains(lowerKey, "render"),
			strings.Contains(lowerKey, "format"):
			severity = api.UCSeverityQoL
		}

		var body strings.Builder
		body.WriteString("Proposed by gocode.module-discover.\n\n")
		body.WriteString("## DoD\n\nThe following L1 tests should pass:\n\n")
		var hinted []string
		for _, t := range c.tests {
			body.WriteString(fmt.Sprintf("- `%s` (`%s`)\n", t.Name, t.TestRef))
			hinted = append(hinted, t.TestRef)
		}
		body.WriteString("\n(specifier persona: refine the title + DoD body to match the intent)\n")

		out = append(out, api.ProposedUseCase{
			Title:    fmt.Sprintf("%s family", k),
			Severity: severity,
			BodyMD:   body.String(),
			HintedBy: hinted,
		})
	}
	return out
}

// proposeTCsFromInventory maps every discovered Test* function to a
// ProposedTestCase. Layer defaults to L1 since these are *_test.go
// functions; L2 / L3 scenarios live in .tasktest files and aren't
// reached by the AST walker.
func proposeTCsFromInventory(inv *discovery.Inventory) []api.ProposedTestCase {
	var out []api.ProposedTestCase
	for i, t := range inv.TestFunctions {
		if t.Kind != discovery.TestKindTest {
			continue
		}
		tcid := fmt.Sprintf("L1-tc%03d-%s", i+1, strings.ToLower(strings.TrimPrefix(t.Name, "Test")))
		out = append(out, api.ProposedTestCase{
			TCID:    tcid,
			Layer:   api.UCLayerL1,
			TestRef: t.TestRef,
		})
	}
	return out
}

// proposeDepsFromInventory emits one ProposedDependency per
// (importer, imported-non-stdlib) pair where the import is local
// (within the same go.mod). Cross-module deps in a polyrepo are
// out of scope for v1.
func proposeDepsFromInventory(inv *discovery.Inventory) []api.ProposedDependency {
	seen := map[string]bool{}
	var out []api.ProposedDependency
	for _, imp := range inv.Imports {
		if !imp.IsLocal {
			continue
		}
		key := imp.Importer + "→" + imp.Imported
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, api.ProposedDependency{
			FromModule: imp.Importer,
			ToModule:   imp.Imported,
		})
	}
	return out
}

// commitCodeSymbols writes one `code-symbol` item per exported
// identifier in the inventory. Idempotent on consecutive runs in
// the §15.12 sense: re-scanning the same source proposes the same
// items. Real idempotency vs. existing items is a follow-up — v1
// always appends.
func commitCodeSymbols(ctx stdctx.Context, cwm cwmapi.CWMService, moduleID string, inv *discovery.Inventory) (int, []string, error) {
	committed := 0
	var warnings []string
	for _, sym := range inv.PublicSymbols {
		body := map[string]any{
			"symbol_name": sym.Name,
			"symbol_kind": string(sym.Kind),
			"package":     sym.Package,
			"signature":   sym.Signature,
			"doc":         sym.Doc,
		}
		if sym.Receiver != "" {
			body["receiver"] = sym.Receiver
		}
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("marshal code-symbol %s: %v", sym.Name, err))
			continue
		}
		title := sym.Package + "." + sym.Name
		symModule, err := cwmapi.ModuleOfObjectID(moduleID)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("code-symbol %s: owning module: %v", title, err))
			continue
		}
		symObj := cwmapi.NewRecordObject(symModule, title, "code-symbol", moduleID, title, bodyBytes)
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: symModule, Object: symObj}); err != nil {
			// code-symbol may not be allowed in every workspace, and a re-scan
			// hits an existing object. Capture the warning + continue.
			warnings = append(warnings, fmt.Sprintf("ObjectCreate code-symbol %s: %v", title, err))
			continue
		}
		committed++
	}
	_ = time.Now // keep time import alive for future timestamp usage
	return committed, warnings, nil
}

// testClusterKey derives a clustering key from a test name. Strips
// the "Test" prefix + everything after the first underscore (Go's
// idiomatic test sub-suite separator).
//
// Examples:
//
//	"TestAdd_Overflow"          → "Add"
//	"TestSubtract_OK"           → "Subtract"
//	"TestErrOverflow_message"   → "ErrOverflow"
//	"TestValidatePair"          → "ValidatePair"
func testClusterKey(name string) string {
	name = strings.TrimPrefix(name, "Test")
	if i := strings.Index(name, "_"); i > 0 {
		return name[:i]
	}
	return name
}

// codePackagesOf reads a module object's code_packages, coping with the list
// arriving as []any from YAML or []string from a typed writer.
func codePackagesOf(o cwmapi.Object) []string {
	group, ok := o.Capability(codeCapability)
	if !ok {
		return nil
	}
	switch v := group[api.CodePackagesField].(type) {
	case []string:
		return v
	case []any:
		var out []string
		for _, e := range v {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// flattenObjectTree walks the nested tree into a flat slice, depth-first.
func flattenObjectTree(nodes []cwmapi.ObjectTreeNode) []cwmapi.Object {
	var out []cwmapi.Object
	for _, n := range nodes {
		out = append(out, n.Object)
		out = append(out, flattenObjectTree(n.Children)...)
	}
	return out
}
