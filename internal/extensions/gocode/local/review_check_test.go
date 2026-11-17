// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	gccore "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/core"
	reviewsapi "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/api"
	reviewscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/contrib"
	reviewslocal "github.com/katyoti/katyoti-fwk/internal/extensions/reviews/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// The go-checks review type's check answers what the audit finds as a review
// result (the build failure critical, the failing tests high, a score, a key
// per finding); filed by the reviews service, it is the module's latest audit
// for the code projections, its findings under the module counted open; filed
// again with the same findings, they are the same objects.
func TestTheGoChecksAreAReviewTypeTheProjectionsRead(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.SoftwareModules(t),
		reviewscontrib.Pack(""), reviewslocal.Extension(),
		application.DeclareSimpleService[gcapi.GoToolService](func(context.Context) (gcapi.GoToolService, error) {
			return auditGoTool{}, nil
		}),
	)
	svc := &gccore.GocodeServiceImpl{}
	seedProject(t, ctx, cwm, "proj")
	mod, err := svc.ModuleCreate(ctx, gcapi.ModuleCreateInput{Name: "checked", ProjectID: "proj", Kind: gcapi.ModuleKindLibrary})
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.ModuleReviewCheck(ctx, gcapi.ModuleReviewCheckInput{Object: mod.Module.ID, Type: projection.GoChecksReviewType})
	if err != nil {
		t.Fatal(err)
	}
	res := out.ReviewResult
	if res.Rating != reviewsapi.RatingCritical || res.Score == nil || *res.Score >= 100 || len(res.Findings) < 2 || !strings.Contains(res.Report, "# Go checks") {
		t.Fatalf("the check's result: %+v", res)
	}
	for _, f := range res.Findings {
		if f.Key == "" || f.Location == "" {
			t.Errorf("a finding without its key or location: %+v", f)
		}
	}

	reviews, err := application.UseService[reviewsapi.ReviewsService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := reviews.File(ctx, reviewsapi.FileInput{Object: mod.Module.ID, Type: projection.GoChecksReviewType, Trigger: reviewsapi.TriggerAutomatic, Result: res})
	if err != nil {
		t.Fatal(err)
	}
	in, err := projection.LoadManifestInputs(ctx, cwm, mod.Module.ID)
	if err != nil {
		t.Fatal(err)
	}
	if in.LatestAudit == nil || in.LatestAudit.ID != first.Review || in.LatestAudit.RanAt.IsZero() {
		t.Fatalf("the go-checks review is the latest audit: %+v (want %s)", in.LatestAudit, first.Review)
	}
	crit := 0
	for _, f := range in.OpenFindings {
		if f.Severity == gcapi.FindingSeverityCritical {
			crit++
		}
	}
	if len(in.OpenFindings) != len(first.New) || crit != 1 {
		t.Errorf("its open findings: %+v (filed %v)", in.OpenFindings, first.New)
	}
	again, err := reviews.File(ctx, reviewsapi.FileInput{Object: mod.Module.ID, Type: projection.GoChecksReviewType, Trigger: reviewsapi.TriggerAutomatic, Result: res})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.New) != 0 || len(again.Persisting) != len(first.New) {
		t.Errorf("the same findings checked again are the same objects: %+v", again)
	}
}

// recordingGoTool records what each check is run on; everything is clean.
type recordingGoTool struct {
	gcapi.GoToolService
	build, inventory []string
	depcheck         []string
}

func (r *recordingGoTool) GoBuild(_ context.Context, in gcapi.GoBuildInput) (gcapi.GoBuildOutput, error) {
	r.build = in.Packages
	return gcapi.GoBuildOutput{}, nil
}
func (r *recordingGoTool) GoVet(context.Context, gcapi.GoVetInput) (gcapi.GoVetOutput, error) {
	return gcapi.GoVetOutput{}, nil
}
func (r *recordingGoTool) GoTest(context.Context, gcapi.GoTestInput) (gcapi.GoTestOutput, error) {
	return gcapi.GoTestOutput{Passed: 1}, nil
}
func (r *recordingGoTool) GoDepCheck(_ context.Context, in gcapi.GoDepCheckInput) (gcapi.GoDepCheckOutput, error) {
	r.depcheck = append(r.depcheck, in.ModulePath)
	return gcapi.GoDepCheckOutput{}, nil
}
func (r *recordingGoTool) GoPkgInventory(_ context.Context, in gcapi.GoPkgInventoryInput) (gcapi.GoPkgInventoryOutput, error) {
	r.inventory = in.Packages
	return gcapi.GoPkgInventoryOutput{}, nil
}

// The checks run on the module's own code: each declared package as a local
// root with its sub-packages (a bare path would read as a standard-library
// import path), the dependency check and the package inventory scoped to them
// — never the whole workspace for one module.
func TestTheGoChecksRunOnTheModulesOwnPackages(t *testing.T) {
	tool := &recordingGoTool{}
	ctx, cwm := cwmapp.New(t, cwmapp.SoftwareModules(t),
		reviewscontrib.Pack(""),
		application.DeclareSimpleService[gcapi.GoToolService](func(context.Context) (gcapi.GoToolService, error) { return tool, nil }),
	)
	svc := &gccore.GocodeServiceImpl{}
	seedProject(t, ctx, cwm, "proj")
	mod, err := svc.ModuleCreate(ctx, gcapi.ModuleCreateInput{Name: "scoped", ProjectID: "proj", Kind: gcapi.ModuleKindLibrary,
		Facets: []gcapi.Facet{{Kind: gcapi.FacetCode, Path: "internal/extensions/scoped"}, {Kind: gcapi.FacetCode, Path: "./app/scoped/..."}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.ModuleReviewCheck(ctx, gcapi.ModuleReviewCheckInput{Object: mod.Module.ID})
	if err != nil {
		t.Fatal(err)
	}
	if out.Rating != "good" {
		t.Errorf("a clean module is rated good: %+v", out.ReviewResult)
	}
	if got := strings.Join(tool.build, " "); got != "./internal/extensions/scoped/... ./app/scoped/..." {
		t.Errorf("go build ran on %q", got)
	}
	if got := strings.Join(tool.inventory, " "); got != "./internal/extensions/scoped/... ./app/scoped/..." {
		t.Errorf("the inventory ran on %q", got)
	}
	if got := strings.Join(tool.depcheck, " "); got != "./internal/extensions/scoped ./app/scoped" {
		t.Errorf("the dependency check ran on %q", got)
	}
	if out.Coverage != "" {
		t.Errorf("a whole-module run covers all of it: %q", out.Coverage)
	}

	// A change's run covers only its packages; a change touching no Go file
	// of the module checks nothing - the reviews then file nothing.
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: mod.Module.ID})
	if err != nil || res.ProjectRoot == "" {
		t.Fatalf("the module's project root: %q %v", res.ProjectRoot, err)
	}
	changed := filepath.Join(res.ProjectRoot, "internal", "extensions", "scoped", "a.go")
	if err := os.MkdirAll(filepath.Dir(changed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte("package scoped\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	part, err := svc.ModuleReviewCheck(ctx, gcapi.ModuleReviewCheckInput{Object: mod.Module.ID, Paths: []string{"internal/extensions/scoped/a.go"}})
	if err != nil || part.Coverage != reviewsapi.CoveragePartial {
		t.Errorf("a change's run is partial: %q %v", part.Coverage, err)
	}
	none, err := svc.ModuleReviewCheck(ctx, gcapi.ModuleReviewCheckInput{Object: mod.Module.ID, Paths: []string{"modules/scoped/concepts.md"}})
	if err != nil || none.Coverage != reviewsapi.CoverageNone {
		t.Errorf("a change with no Go file of the module checks nothing: %q %v", none.Coverage, err)
	}
}
