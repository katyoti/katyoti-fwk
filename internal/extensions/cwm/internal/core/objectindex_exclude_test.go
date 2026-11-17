// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// writeFiles writes each project-relative path with its content.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// projectExcluding is a project object whose scan leaves the globs out.
func projectExcluding(globs ...string) string {
	s := "id: acme\ntype: project\ncapabilities:\n  project:\n    scan:\n      exclude:\n"
	for _, g := range globs {
		s += "        - \"" + g + "\"\n"
	}
	return s
}

// scanned runs one scan and answers the index's ids and the unreadable paths.
func scanned(t *testing.T, svc *Service) (api.ObjectScanOutput, map[string]bool, []api.InvalidObject) {
	t.Helper()
	ctx := stdctx.Background()
	out, err := svc.ObjectScan(ctx, api.ObjectScanInput{})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	ids := map[string]bool{}
	for id := range svc.index().byID {
		ids[id] = true
	}
	errs, err := svc.ObjectErrorList(ctx, api.ObjectErrorListInput{})
	if err != nil {
		t.Fatal(err)
	}
	return out, ids, errs.Unreadable
}

// The corrupt file in an excluded folder: no id, so reading it would REPORT
// it — which is how a walk of a pruned folder shows.
const corrupt = "type: feature\n"

// TestScanExcludeLeavesOutAFolderAndAFile pins `scan.exclude`: a folder glob
// keeps everything under it out of the index, a file glob one file, and the
// objects beside them are indexed.
func TestScanExcludeLeavesOutAFolderAndAFile(t *testing.T) {
	svc, root := newSvc(t)
	writeFiles(t, root, map[string]string{
		api.ProjectFileName:                   projectExcluding("modules/*/drafts/**", "modules/a/objects/skip.cwm.yaml"),
		"modules/a/objects/x.cwm.yaml":        "id: a.x\ntype: feature\n",
		"modules/a/objects/skip.cwm.yaml":     "id: a.skip\ntype: feature\n",
		"modules/a/drafts/y.cwm.yaml":         "id: a.y\ntype: feature\n",
		"modules/a/drafts/deep/bad.cwm.yaml":  corrupt,
		"modules/b/objects/drafts/z.cwm.yaml": "id: b.z\ntype: feature\n", // drafts not directly under the module
	})
	_, ids, bad := scanned(t, svc)
	for _, id := range []string{"a.x", "b.z", "acme"} {
		if !ids[id] {
			t.Errorf("%s is not indexed; the index holds %v", id, ids)
		}
	}
	for _, id := range []string{"a.y", "a.skip"} {
		if ids[id] {
			t.Errorf("%s is indexed although the project excludes it", id)
		}
	}
	if len(bad) != 0 {
		t.Errorf("an excluded folder was read: %+v", bad)
	}
}

// TestScanExcludePrunesTheFolder pins that a matching folder is not walked
// at all: the glob names the folder alone, so nothing under it matches — only
// pruning keeps its objects, and its corrupt file, out.
func TestScanExcludePrunesTheFolder(t *testing.T) {
	svc, root := newSvc(t)
	writeFiles(t, root, map[string]string{
		api.ProjectFileName:              projectExcluding("modules/a/scratch"),
		"modules/a/objects/x.cwm.yaml":   "id: a.x\ntype: feature\n",
		"modules/a/scratch/z.cwm.yaml":   "id: a.z\ntype: feature\n",
		"modules/a/scratch/bad.cwm.yaml": corrupt,
	})
	_, ids, bad := scanned(t, svc)
	if !ids["a.x"] {
		t.Errorf("a.x is not indexed")
	}
	if ids["a.z"] {
		t.Error("an object under a pruned folder is indexed")
	}
	if len(bad) != 0 {
		t.Errorf("a pruned folder was walked: %+v", bad)
	}
}

// TestScanExcludeRemovesANewlyExcludedObject pins that excluding an object
// already indexed takes it out on the next scan, reported as removed.
func TestScanExcludeRemovesANewlyExcludedObject(t *testing.T) {
	svc, root := newSvc(t)
	writeFiles(t, root, map[string]string{
		api.ProjectFileName:           projectExcluding(),
		"modules/a/drafts/y.cwm.yaml": "id: a.y\ntype: feature\n",
	})
	if _, ids, _ := scanned(t, svc); !ids["a.y"] {
		t.Fatal("a.y is not indexed before its exclusion")
	}
	writeFiles(t, root, map[string]string{api.ProjectFileName: projectExcluding("modules/*/drafts/**")})
	out, ids, _ := scanned(t, svc)
	if ids["a.y"] || !slices.Contains(out.Removed, "a.y") {
		t.Fatalf("a newly excluded object: indexed=%v removed=%v", ids["a.y"], out.Removed)
	}
}

// TestScanExcludeInvalidGlobIsReported pins that a glob doublestar refuses is
// reported on the project object, naming it, and the scan runs on the
// defaults: one bad setting never makes the project unopenable.
func TestScanExcludeInvalidGlobIsReported(t *testing.T) {
	svc, root := newSvc(t)
	writeFiles(t, root, map[string]string{
		api.ProjectFileName:            projectExcluding("modules/[a"),
		"modules/a/objects/x.cwm.yaml": "id: a.x\ntype: feature\n",
	})
	_, ids, bad := scanned(t, svc)
	if !ids["a.x"] {
		t.Error("a.x is not indexed: the scan did not run on the defaults")
	}
	if len(bad) != 1 || bad[0].Path != api.ProjectFileName || !strings.Contains(strings.Join(bad[0].Problems, " "), "modules/[a") {
		t.Fatalf("the invalid glob is not reported on the project object: %+v", bad)
	}
}
