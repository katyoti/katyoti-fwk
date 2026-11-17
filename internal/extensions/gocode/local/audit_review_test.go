// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	gccore "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/core"
	reviewscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/contrib"
	reviewslocal "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// auditGoTool is a toolchain whose build fails and two tests fail; the rest
// is clean.
type auditGoTool struct{ gcapi.GoToolService }

func (auditGoTool) GoBuild(context.Context, gcapi.GoBuildInput) (gcapi.GoBuildOutput, error) {
	return gcapi.GoBuildOutput{CommonToolResult: gcapi.CommonToolResult{ExitCode: 1, Stderr: "undefined: x"}}, nil
}
func (auditGoTool) GoVet(context.Context, gcapi.GoVetInput) (gcapi.GoVetOutput, error) {
	return gcapi.GoVetOutput{}, nil
}
func (auditGoTool) GoTest(context.Context, gcapi.GoTestInput) (gcapi.GoTestOutput, error) {
	return gcapi.GoTestOutput{Passed: 3, Failed: 2}, nil
}
func (auditGoTool) GoDepCheck(context.Context, gcapi.GoDepCheckInput) (gcapi.GoDepCheckOutput, error) {
	return gcapi.GoDepCheckOutput{}, nil
}
func (auditGoTool) GoPkgInventory(context.Context, gcapi.GoPkgInventoryInput) (gcapi.GoPkgInventoryOutput, error) {
	return gcapi.GoPkgInventoryOutput{}, nil
}

// An audit is the module's go-checks review, filed by the reviews service and
// closed with its rating, a finding under the module per thing found — and the
// code projections read it back as the latest audit with its open findings.
func TestAnAuditIsAReviewWithItsFindings(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.SoftwareModules(t),
		reviewscontrib.Pack(""), reviewslocal.Extension(),
		application.DeclareSimpleService[gcapi.GoToolService](func(context.Context) (gcapi.GoToolService, error) {
			return auditGoTool{}, nil
		}),
	)
	svc := &gccore.GocodeServiceImpl{}
	seedProject(t, ctx, cwm, "proj")
	mod, err := svc.ModuleCreate(ctx, gcapi.ModuleCreateInput{Name: "audited", ProjectID: "proj", Kind: gcapi.ModuleKindLibrary})
	if err != nil {
		t.Fatal(err)
	}

	out, err := svc.ModuleAudit(ctx, gcapi.ModuleAuditInput{ID: mod.Module.ID})
	if err != nil {
		t.Fatal(err)
	}
	if out.CriticalCount != 1 || out.HighCount < 1 {
		t.Fatalf("counts %+v; want the build critical, the tests high", out)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: out.ReviewID})
	if err != nil {
		t.Fatal(err)
	}
	review := got.Object
	lc, _ := review.Lifecycle()
	if review.Type != "review" || review.ParentID() != mod.Module.ID || lc.State != "closed" ||
		review.Fields["rating"] != "critical" || review.Fields["scope"] != mod.Module.ID || review.Fields["type"] != projection.GoChecksReviewType {
		t.Fatalf("the audit: %s %s state %q fields %v; want a closed review of the module, rating critical", review.Type, review.ParentID(), lc.State, review.Fields)
	}

	in, err := projection.LoadManifestInputs(ctx, cwm, mod.Module.ID)
	if err != nil {
		t.Fatal(err)
	}
	if in.LatestAudit == nil || in.LatestAudit.ID != review.ID {
		t.Fatalf("latest audit %+v; want %s", in.LatestAudit, review.ID)
	}
	byID := map[string]gcapi.AuditFinding{}
	for _, f := range in.OpenFindings {
		byID[f.Title] = f
	}
	if f := byID["go build failed"]; f.Severity != gcapi.FindingSeverityCritical || f.Perspective != gcapi.AuditPerspectiveCorrectness || f.State != gcapi.FindingStateOpen {
		t.Errorf("the build finding: %+v; want critical, correctness, open", f)
	}
	if len(byID) != out.CriticalCount+out.HighCount+out.MediumCount+out.LowCount+countInfo(in.OpenFindings) {
		t.Errorf("findings read back %d, written %+v", len(byID), out)
	}
}

func countInfo(fs []gcapi.AuditFinding) int {
	n := 0
	for _, f := range fs {
		if f.Severity == gcapi.FindingSeverityInfo {
			n++
		}
	}
	return n
}
