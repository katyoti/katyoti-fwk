// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	codeapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/lint"
)

// ── fixtures ─────────────────────────────────────────────────────────

// fakeGoTool implements gcapi.GoToolService with canned results,
// capturing the inputs goPack sends so tests can pin the dispatch.
type fakeGoTool struct {
	gcapi.GoToolService // panic on anything not overridden

	gotTest  *gcapi.GoTestInput
	testOut  gcapi.GoTestOutput
	gotBuild *gcapi.GoBuildInput
	buildOut gcapi.GoBuildOutput
}

func (f *fakeGoTool) GoTest(_ context.Context, in gcapi.GoTestInput) (gcapi.GoTestOutput, error) {
	f.gotTest = &in
	return f.testOut, nil
}

func (f *fakeGoTool) GoBuild(_ context.Context, in gcapi.GoBuildInput) (gcapi.GoBuildOutput, error) {
	f.gotBuild = &in
	return f.buildOut, nil
}

// newPackCtx builds a minimal Application context with the fake
// GoToolService registered, mirroring the bundled-topology resolution
// path the pack uses at runtime.
func newPackCtx(t *testing.T, gotool gcapi.GoToolService) context.Context {
	t.Helper()
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	if gotool != nil {
		application.PutService[gcapi.GoToolService](app, "default", gotool)
	}
	return application.WithApplication(context.Background(), app)
}

// ── Describe ─────────────────────────────────────────────────────────

func TestGoPack_Describe(t *testing.T) {
	t.Parallel()
	info, err := (&goPack{}).Describe(context.Background(), codeapi.DescribeInput{})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if info.Language != "go" {
		t.Errorf("Language = %q, want go", info.Language)
	}
	if len(info.FileExtensions) != 1 || info.FileExtensions[0] != ".go" {
		t.Errorf("FileExtensions = %v, want [.go]", info.FileExtensions)
	}
	if info.ToolServiceSlot != "GoToolService" {
		t.Errorf("ToolServiceSlot = %q, want GoToolService", info.ToolServiceSlot)
	}
	wantGates := []string{"go-build-clean", "go-test-pass", "go-vet-clean", "golangci-lint-clean"}
	if len(info.Gates) != len(wantGates) {
		t.Fatalf("Gates = %d, want %d", len(info.Gates), len(wantGates))
	}
	for i, g := range info.Gates {
		if g.Name != wantGates[i] {
			t.Errorf("Gates[%d].Name = %q, want %q", i, g.Name, wantGates[i])
		}
		if g.Transition == "" {
			t.Errorf("Gates[%d] (%s): empty Transition", i, g.Name)
		}
	}
	if want := len(lint.AllDetectors()); len(info.CheckKinds) != want {
		t.Errorf("CheckKinds = %d, want %d (lint.AllDetectors)", len(info.CheckKinds), want)
	}
	if len(info.LayoutRules) == 0 {
		t.Error("LayoutRules empty — want the 3-package convention declared")
	}
}

// ── Discover / ProposeUCsFromTests ───────────────────────────────────

// writeMiniModule lays out a one-package Go module with one exported
// symbol and one test function, returning its root.
func writeMiniModule(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/mini\n\ngo 1.22\n",
		"pkg/greet/greet.go": `package greet

// Greet says hello.
func Greet(name string) string { return "hello " + name }
`,
		"pkg/greet/greet_test.go": `package greet

import "testing"

func TestGreet(t *testing.T) {
	if Greet("x") != "hello x" {
		t.Fatal("nope")
	}
}
`,
	}
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestGoPack_Discover(t *testing.T) {
	t.Parallel()
	root := writeMiniModule(t)
	out, err := (&goPack{}).Discover(context.Background(), codeapi.DiscoverInput{Path: root})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(out.Inventory.Packages) == 0 {
		t.Fatal("Inventory.Packages empty")
	}
	if len(out.Inventory.TestFunctions) != 1 || out.Inventory.TestFunctions[0] != "TestGreet" {
		t.Errorf("TestFunctions = %v, want [TestGreet]", out.Inventory.TestFunctions)
	}
	hasCodeFacet := false
	for _, fp := range out.FacetProposals {
		if fp.Facet.Kind == codeapi.FacetCode {
			hasCodeFacet = true
		}
	}
	if !hasCodeFacet {
		t.Errorf("FacetProposals %v: no code facet proposed", out.FacetProposals)
	}
	if len(out.UCProposals) == 0 {
		t.Error("UCProposals empty — want one seed per test cluster")
	}
	if len(out.TestCaseProposals) != 1 {
		t.Fatalf("TestCaseProposals = %d, want 1", len(out.TestCaseProposals))
	}
	ref := out.TestCaseProposals[0].Ref
	if ref.Layer != "L1" || ref.Ref != "TestGreet" {
		t.Errorf("TestCaseProposals[0].Ref = %+v, want L1/TestGreet", ref)
	}
}

func TestGoPack_ProposeUCsFromTests(t *testing.T) {
	t.Parallel()
	root := writeMiniModule(t)
	out, err := (&goPack{}).ProposeUCsFromTests(context.Background(), codeapi.ProposeUCsInput{Path: root})
	if err != nil {
		t.Fatalf("ProposeUCsFromTests: %v", err)
	}
	if len(out.UCProposals) != 1 {
		t.Fatalf("UCProposals = %d, want 1", len(out.UCProposals))
	}
	if got := out.UCProposals[0].HintedBy; len(got) != 1 || got[0] != "TestGreet" {
		t.Errorf("HintedBy = %v, want [TestGreet]", got)
	}
}

// ── RunUnitTest ──────────────────────────────────────────────────────

func TestGoPack_RunUnitTest_DispatchAndStatuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		ref        codeapi.TestRef
		out        gcapi.GoTestOutput
		wantStatus codeapi.ACStatus
		wantRegex  string
		wantPkgs   []string
	}{
		{
			name:       "plain name + package → pass",
			ref:        codeapi.TestRef{Layer: "L1", Ref: "TestGreet", Package: "pkg/greet"},
			out:        gcapi.GoTestOutput{Passed: 1},
			wantStatus: codeapi.ACPass,
			wantRegex:  "^TestGreet$",
			wantPkgs:   []string{"./pkg/greet"},
		},
		{
			name:       "file::func ref derives package",
			ref:        codeapi.TestRef{Layer: "L1", Ref: "pkg/greet/greet_test.go::TestGreet"},
			out:        gcapi.GoTestOutput{Passed: 1},
			wantStatus: codeapi.ACPass,
			wantRegex:  "^TestGreet$",
			wantPkgs:   []string{"./pkg/greet"},
		},
		{
			name:       "failure maps to fail",
			ref:        codeapi.TestRef{Layer: "L1", Ref: "TestGreet"},
			out:        gcapi.GoTestOutput{CommonToolResult: gcapi.CommonToolResult{ExitCode: 1}, Failed: 1},
			wantStatus: codeapi.ACFail,
			wantRegex:  "^TestGreet$",
			wantPkgs:   []string{"./..."},
		},
		{
			name:       "zero matches = dead ref = fail",
			ref:        codeapi.TestRef{Layer: "L1", Ref: "TestGone"},
			out:        gcapi.GoTestOutput{},
			wantStatus: codeapi.ACFail,
			wantRegex:  "^TestGone$",
			wantPkgs:   []string{"./..."},
		},
		{
			name:       "self-skipping test maps to skipped",
			ref:        codeapi.TestRef{Layer: "L1", Ref: "TestGreet"},
			out:        gcapi.GoTestOutput{Skipped: 1},
			wantStatus: codeapi.ACSkipped,
			wantRegex:  "^TestGreet$",
			wantPkgs:   []string{"./..."},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeGoTool{testOut: tc.out}
			ctx := newPackCtx(t, fake)
			got, err := (&goPack{}).RunUnitTest(ctx, codeapi.RunUnitTestInput{Ref: tc.ref})
			if err != nil {
				t.Fatalf("RunUnitTest: %v", err)
			}
			if got.Result.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q (reason: %s)", got.Result.Status, tc.wantStatus, got.Result.Reason)
			}
			if fake.gotTest == nil {
				t.Fatal("GoTest never called")
			}
			if fake.gotTest.RunRegex != tc.wantRegex {
				t.Errorf("RunRegex = %q, want %q", fake.gotTest.RunRegex, tc.wantRegex)
			}
			if len(fake.gotTest.Packages) != len(tc.wantPkgs) || fake.gotTest.Packages[0] != tc.wantPkgs[0] {
				t.Errorf("Packages = %v, want %v", fake.gotTest.Packages, tc.wantPkgs)
			}
			if !fake.gotTest.JSON {
				t.Error("JSON = false, want true (parseable output)")
			}
		})
	}
}

func TestGoPack_RunUnitTest_NonL1Skipped(t *testing.T) {
	t.Parallel()
	// No GoToolService registered — proves non-L1 short-circuits
	// before any toolchain resolution.
	got, err := (&goPack{}).RunUnitTest(context.Background(), codeapi.RunUnitTestInput{
		Ref: codeapi.TestRef{Layer: "L3", Ref: "scenario-x"},
	})
	if err != nil {
		t.Fatalf("RunUnitTest: %v", err)
	}
	if got.Result.Status != codeapi.ACSkipped {
		t.Errorf("Status = %q, want skipped", got.Result.Status)
	}
}

// ── GateCheck ────────────────────────────────────────────────────────

func TestGoPack_GateCheck(t *testing.T) {
	t.Parallel()
	facets := []codeapi.Facet{
		{Kind: codeapi.FacetCode, Path: "internal/extensions/greet"},
		{Kind: codeapi.FacetDocs, Path: "docs/modules/greet"},
	}

	t.Run("build clean → allow, code facets only", func(t *testing.T) {
		fake := &fakeGoTool{}
		ctx := newPackCtx(t, fake)
		out, err := (&goPack{}).GateCheck(ctx, codeapi.GateCheckInput{Gate: "go-build-clean", Facets: facets})
		if err != nil {
			t.Fatalf("GateCheck: %v", err)
		}
		if !out.Allow {
			t.Errorf("Allow = false, reason: %s", out.Reason)
		}
		want := "./internal/extensions/greet/..."
		if fake.gotBuild == nil || len(fake.gotBuild.Packages) != 1 || fake.gotBuild.Packages[0] != want {
			t.Errorf("GoBuild packages = %+v, want [%s]", fake.gotBuild, want)
		}
	})

	t.Run("test failure → deny with reason", func(t *testing.T) {
		fake := &fakeGoTool{testOut: gcapi.GoTestOutput{
			CommonToolResult: gcapi.CommonToolResult{ExitCode: 1, Stderr: "boom"},
			Failed:           2,
		}}
		ctx := newPackCtx(t, fake)
		out, err := (&goPack{}).GateCheck(ctx, codeapi.GateCheckInput{Gate: "go-test-pass", Facets: facets})
		if err != nil {
			t.Fatalf("GateCheck: %v", err)
		}
		if out.Allow || out.Reason == "" {
			t.Errorf("want deny with reason, got allow=%v reason=%q", out.Allow, out.Reason)
		}
	})

	t.Run("unknown gate errors", func(t *testing.T) {
		ctx := newPackCtx(t, &fakeGoTool{})
		if _, err := (&goPack{}).GateCheck(ctx, codeapi.GateCheckInput{Gate: "nope"}); err == nil {
			t.Fatal("want error for unknown gate")
		}
	})
}

// ── Named registration round-trip ────────────────────────────────────

// TestGoPack_NamedRegistration pins the bundled-topology resolution
// path: PutService under SlotType instance "go" → UseNamedService.
func TestGoPack_NamedRegistration(t *testing.T) {
	t.Parallel()
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	application.PutService[codeapi.LanguagePack](app, "go", &goPack{})
	ctx := application.WithApplication(context.Background(), app)

	pack, err := application.UseNamedService[codeapi.LanguagePack](ctx, "go")
	if err != nil {
		t.Fatalf("UseNamedService(go): %v", err)
	}
	info, err := pack.Describe(ctx, codeapi.DescribeInput{})
	if err != nil {
		t.Fatalf("Describe via resolved pack: %v", err)
	}
	if info.Language != "go" {
		t.Errorf("Language = %q, want go", info.Language)
	}
}
