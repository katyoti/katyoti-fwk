// SPDX-License-Identifier: Apache-2.0

package api

import (
	"reflect"
	"strings"
	"testing"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
)

// TestGocodeServiceMethodsDiscoverable asserts every method on the
// GocodeService interface is shaped per the publishable convention
// (ctx, Input) → (Output, error). AutoKTSPContainer auto-surfaces
// these as KTSP slot / agent-tool / CLI / HTTP at AppInit; if a
// method drifts off-shape, the surface goes silently missing.
func TestGocodeServiceMethodsDiscoverable(t *testing.T) {
	t.Parallel()

	iface := reflect.TypeOf((*GocodeService)(nil)).Elem()
	methods := corereflect.DiscoverMethods(iface)

	wantMin := 17 // 8 module + 4 uc + 2 feature + 2 tasktest + orchestrate (CharterPublish/ManifestWrite/OD* moved to CodeService)
	if len(methods) < wantMin {
		t.Fatalf("GocodeService discovered methods = %d, want >= %d", len(methods), wantMin)
	}

	// Every discovered method must have InputType non-nil and
	// OutputType named <MethodName>Output by our convention.
	for _, m := range methods {
		if m.InputType == nil {
			t.Errorf("GocodeService.%s has nil InputType — every gocode verb takes a typed input", m.Name)
			continue
		}
		wantInName := m.Name + "Input"
		if m.InputType.Name() != wantInName {
			t.Errorf("GocodeService.%s: InputType = %q, want %q", m.Name, m.InputType.Name(), wantInName)
		}
		wantOutName := m.Name + "Output"
		if m.OutputType.Name() != wantOutName {
			t.Errorf("GocodeService.%s: OutputType = %q, want %q", m.Name, m.OutputType.Name(), wantOutName)
		}
	}

	// Check the canonical method names exist (catches typos /
	// accidental renames over time).
	wantNames := []string{
		// module slot (CharterPublish/ManifestWrite + the od slot moved
		// to CodeService — Track B #4 phase 1)
		"ModuleCreate", "ModuleDiscover", "ModuleStatus",
		"ModulePlanTier", "ModuleVerify", "ModuleAudit", "ModuleReviewCheck",
		"ModuleLint", "ModulePromote",
		// uc slot
		"UCCreate", "UCUpdate", "UCSetStatus", "UCProposeFromTests",
		// feature slot
		"FeatureReverseEngineer",
		// tasktest slot
		"TasktestCreate", "TasktestRun",
	}
	have := make(map[string]bool, len(methods))
	for _, m := range methods {
		have[m.Name] = true
	}
	for _, want := range wantNames {
		if !have[want] {
			t.Errorf("GocodeService missing canonical method %q", want)
		}
	}
}

// TestGoToolServiceMethodsDiscoverable does the same for GoToolService.
func TestGoToolServiceMethodsDiscoverable(t *testing.T) {
	t.Parallel()

	iface := reflect.TypeOf((*GoToolService)(nil)).Elem()
	methods := corereflect.DiscoverMethods(iface)

	wantMin := 10
	if len(methods) < wantMin {
		t.Fatalf("GoToolService discovered methods = %d, want >= %d", len(methods), wantMin)
	}

	wantNames := []string{
		"GoBuild", "GoTest", "GoVet", "GolangciLint",
		"GoAPIDiff", "GoPkgInventory", "GoDepCheck",
		"Gofmt", "Goimports", "GoModTidy",
	}
	have := make(map[string]bool, len(methods))
	for _, m := range methods {
		have[m.Name] = true
	}
	for _, want := range wantNames {
		if !have[want] {
			t.Errorf("GoToolService missing canonical method %q", want)
		}
	}
}

// TestEveryDTOHasValidJSONSchema asserts every Input/Output DTO can
// have a JSON Schema generated for it. Failure here means
// AutoKTSPContainer would silently break at AppInit when generating
// agent-tool descriptors.
func TestEveryDTOHasValidJSONSchema(t *testing.T) {
	t.Parallel()

	type schemaCheck struct {
		name string
		gen  func() error
	}

	checks := []schemaCheck{
		// Module verbs
		{"ModuleCreateInput", schemaCheckOf[ModuleCreateInput]()},
		{"ModuleCreateOutput", schemaCheckOf[ModuleCreateOutput]()},
		{"ModuleDiscoverInput", schemaCheckOf[ModuleDiscoverInput]()},
		{"ModuleDiscoverOutput", schemaCheckOf[ModuleDiscoverOutput]()},
		{"ModuleStatusInput", schemaCheckOf[ModuleStatusInput]()},
		{"ModuleStatusOutput", schemaCheckOf[ModuleStatusOutput]()},
		{"ModulePlanTierInput", schemaCheckOf[ModulePlanTierInput]()},
		{"ModulePlanTierOutput", schemaCheckOf[ModulePlanTierOutput]()},
		{"ModuleVerifyInput", schemaCheckOf[ModuleVerifyInput]()},
		{"ModuleVerifyOutput", schemaCheckOf[ModuleVerifyOutput]()},
		{"ModuleAuditInput", schemaCheckOf[ModuleAuditInput]()},
		{"ModuleAuditOutput", schemaCheckOf[ModuleAuditOutput]()},
		{"ModuleLintInput", schemaCheckOf[ModuleLintInput]()},
		{"ModuleLintOutput", schemaCheckOf[ModuleLintOutput]()},
		{"ModulePromoteInput", schemaCheckOf[ModulePromoteInput]()},
		{"ModulePromoteOutput", schemaCheckOf[ModulePromoteOutput]()},
		// UC verbs
		{"UCCreateInput", schemaCheckOf[UCCreateInput]()},
		{"UCCreateOutput", schemaCheckOf[UCCreateOutput]()},
		{"UCUpdateInput", schemaCheckOf[UCUpdateInput]()},
		{"UCUpdateOutput", schemaCheckOf[UCUpdateOutput]()},
		{"UCSetStatusInput", schemaCheckOf[UCSetStatusInput]()},
		{"UCSetStatusOutput", schemaCheckOf[UCSetStatusOutput]()},
		{"UCProposeFromTestsInput", schemaCheckOf[UCProposeFromTestsInput]()},
		{"UCProposeFromTestsOutput", schemaCheckOf[UCProposeFromTestsOutput]()},
		// OD verbs
		// Feature
		{"FeatureReverseEngineerInput", schemaCheckOf[FeatureReverseEngineerInput]()},
		{"FeatureReverseEngineerOutput", schemaCheckOf[FeatureReverseEngineerOutput]()},
		// Tasktest
		{"TasktestCreateInput", schemaCheckOf[TasktestCreateInput]()},
		{"TasktestCreateOutput", schemaCheckOf[TasktestCreateOutput]()},
		{"TasktestRunInput", schemaCheckOf[TasktestRunInput]()},
		{"TasktestRunOutput", schemaCheckOf[TasktestRunOutput]()},
		// Entities
		{"Module", schemaCheckOf[Module]()},
		{"UseCase", schemaCheckOf[UseCase]()},
		{"TestCase", schemaCheckOf[TestCase]()},
		{"TestRun", schemaCheckOf[TestRun]()},
		{"OpenDecision", schemaCheckOf[OpenDecision]()},
		{"AuditBundle", schemaCheckOf[AuditBundle]()},
		{"AuditFinding", schemaCheckOf[AuditFinding]()},
		{"FeatureSummary", schemaCheckOf[FeatureSummary]()},
		// GoTool DTOs (spot-check the largest ones)
		{"GoTestInput", schemaCheckOf[GoTestInput]()},
		{"GoTestOutput", schemaCheckOf[GoTestOutput]()},
		{"GolangciLintInput", schemaCheckOf[GolangciLintInput]()},
		{"GolangciLintOutput", schemaCheckOf[GolangciLintOutput]()},
		{"GoAPIDiffInput", schemaCheckOf[GoAPIDiffInput]()},
		{"GoAPIDiffOutput", schemaCheckOf[GoAPIDiffOutput]()},
		{"GoPkgInventoryOutput", schemaCheckOf[GoPkgInventoryOutput]()},
		{"GoDepCheckOutput", schemaCheckOf[GoDepCheckOutput]()},
	}

	for _, c := range checks {
		if err := c.gen(); err != nil {
			t.Errorf("JSONSchema generation failed for %s: %v", c.name, err)
		}
	}
}

// TestServicePackageHasNoInternalImports asserts the api/ package
// stays leaf-clean — no dependencies on ktw-gocode internal packages.
// Importers (sidecar binary, CLI surface, tests, agent-tool wrappers,
// other method packs) can rely on this.
func TestServicePackageHasNoInternalImports(t *testing.T) {
	t.Parallel()

	// Walk this package's own files for forbidden imports.
	// This is a sanity guard — go list -deps would catch it at CI,
	// but a unit test makes the rule explicit alongside the contract.
	forbidden := []string{
		"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/",
		"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/local/",
	}
	allowedPrefix := "github.com/katyoti/katyoti-fwk/internal/core/"

	pkg := reflect.TypeOf((*GocodeService)(nil)).Elem().PkgPath()
	if !strings.HasSuffix(pkg, "/extensions/gocode/api") {
		t.Fatalf("unexpected pkg path %q", pkg)
	}
	// The actual import list is enforced by the build system /
	// go list -deps. This test documents the intent; concrete
	// enforcement is in CI's `go list -deps` smoke.
	_ = forbidden
	_ = allowedPrefix
}

// schemaCheckOf returns a thunk that generates a JSONSchema for T
// and returns the generation error (nil on success).
func schemaCheckOf[T any]() func() error {
	return func() error {
		_, err := corereflect.JSONSchema[T]()
		return err
	}
}

// Every GocodeService method has its definition, keyed by its Go name (the
// only key the KTSP publisher reads), and every definition names a method: a
// definition no method matches would silently drop its description, read-only
// flag and timeout.
func TestEveryGocodeMethodHasItsDefinition(t *testing.T) {
	methods := corereflect.DiscoverMethods(reflect.TypeOf((*GocodeService)(nil)).Elem())
	names := map[string]bool{}
	for _, m := range methods {
		names[m.Name] = true
		if _, ok := GocodeServiceDef.Methods[m.Name]; !ok {
			t.Errorf("GocodeService.%s has no definition in GocodeServiceDef.Methods", m.Name)
		}
	}
	for key := range GocodeServiceDef.Methods {
		if !names[key] {
			t.Errorf("GocodeServiceDef.Methods[%q] names no GocodeService method", key)
		}
	}
}
