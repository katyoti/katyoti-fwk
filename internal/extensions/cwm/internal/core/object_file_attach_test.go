// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestObjectFileAttachKeepsEdgesInStepWithAFolder — the object attaches
// exactly the files the pattern matches: the first call adds them, a second
// changes nothing, a file gone is detached, a new one attached, and an edge
// the pattern does not cover is never touched.
func TestObjectFileAttachKeepsEdgesInStepWithAFolder(t *testing.T) {
	svc, project := fileFixture(t)
	ctx := stdctx.Background()
	for _, p := range []string{"study/index.md", "study/01-a.md", "study/02-b.md", "study/media/d.drawio", "other.md"} {
		if _, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{ID: "billing.spec", Path: p, Content: []byte("x")}); err != nil {
			t.Fatal(err)
		}
	}
	// An edge the pattern does not cover: kept whatever the call does.
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.spec"})
	obj := got.Object
	obj.Edges = []api.ObjectEdge{{Kind: "attaches", To: "modules/billing/other.md"}}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "billing", Object: obj}); err != nil {
		t.Fatal(err)
	}

	attach := func(dry bool, patterns ...string) api.ObjectFileAttachOutput {
		t.Helper()
		out, err := svc.ObjectFileAttach(ctx, api.ObjectFileAttachInput{ID: "billing.spec", Patterns: patterns, DryRun: dry})
		if err != nil {
			t.Fatalf("ObjectFileAttach %v: %v", patterns, err)
		}
		return out
	}
	edges := func() []string {
		got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.spec"})
		var to []string
		for _, e := range got.Object.Edges {
			to = append(to, e.To)
		}
		return to
	}

	// A dry run reports and writes nothing.
	if out := attach(true, "study/*.md"); len(out.Added) != 3 || out.Changed {
		t.Fatalf("dry run: %+v", out)
	}
	if len(edges()) != 1 {
		t.Fatalf("a dry run wrote: %v", edges())
	}
	out := attach(false, "study/*.md")
	want := []string{"modules/billing/study/01-a.md", "modules/billing/study/02-b.md", "modules/billing/study/index.md"}
	if !reflect.DeepEqual(out.Added, want) || !out.Changed || len(out.Removed) != 0 {
		t.Fatalf("first attach: %+v", out)
	}
	if got := edges(); len(got) != 4 || got[0] != "modules/billing/other.md" {
		t.Fatalf("edges after attach: %v", got)
	}
	// Idempotent.
	if out := attach(false, "study/*.md"); out.Changed || len(out.Added)+len(out.Removed) != 0 {
		t.Fatalf("second attach changed something: %+v", out)
	}
	// A file gone is detached, a new one attached.
	if err := os.Remove(filepath.Join(project, "modules", "billing", "study", "02-b.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{ID: "billing.spec", Path: "study/03-c.md", Content: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	out = attach(false, "study/*.md")
	if !reflect.DeepEqual(out.Added, []string{"modules/billing/study/03-c.md"}) || !reflect.DeepEqual(out.Removed, []string{"modules/billing/study/02-b.md"}) {
		t.Fatalf("re-attach: %+v", out)
	}
	if got := edges(); len(got) != 4 || got[0] != "modules/billing/other.md" {
		t.Fatalf("the uncovered edge moved or went: %v", got)
	}
	// A folder stands for the files directly in it (media/ is not entered).
	if out := attach(true, "study"); len(out.Attached) != 3 {
		t.Fatalf("folder pattern: %+v", out)
	}
}

// TestObjectFileAttachRefuses — no pattern, a pattern leaving the module, a
// kind that does not target a path.
func TestObjectFileAttachRefuses(t *testing.T) {
	svc, _ := fileFixture(t)
	ctx := stdctx.Background()
	for name, in := range map[string]api.ObjectFileAttachInput{
		"no pattern":      {ID: "billing.spec"},
		"escapes":         {ID: "billing.spec", Patterns: []string{"../other/*.md"}},
		"absolute":        {ID: "billing.spec", Patterns: []string{"/etc/*"}},
		"not a path kind": {ID: "billing.spec", Patterns: []string{"*.md"}, Kind: "links"},
		"undeclared kind": {ID: "billing.spec", Patterns: []string{"*.md"}, Kind: "nope"},
	} {
		if _, err := svc.ObjectFileAttach(ctx, in); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestAttachedFileGoneIsReported — an attaches edge whose file was removed
// outside CWM is reported on the object after a scan (the validity check
// read-side), so a stale attachment is seen before the next attach removes it.
func TestAttachedFileGoneIsReported(t *testing.T) {
	svc, project := fileFixture(t)
	ctx := stdctx.Background()
	if _, err := svc.ObjectFileWrite(ctx, api.ObjectFileWriteInput{ID: "billing.spec", Path: "study/index.md", Content: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectFileAttach(ctx, api.ObjectFileAttachInput{ID: "billing.spec", Patterns: []string{"study/*.md"}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.spec"}); len(got.Errors) != 0 {
		t.Fatalf("errors before the file went: %+v", got.Errors)
	}
	if err := os.Remove(filepath.Join(project, "modules", "billing", "study", "index.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "billing", ID: "billing.spec"})
	found := false
	for _, e := range got.Errors {
		if e.Code == api.DiagnosticInvalid && strings.Contains(e.Description, "modules/billing/study/index.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("a missing attached file is not reported: %+v", got.Errors)
	}
}
