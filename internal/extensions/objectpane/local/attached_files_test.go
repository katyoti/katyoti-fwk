// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// shippedFilesPanel is ktw-agents' own Files panel, as shipped.
func shippedFilesPanel(t *testing.T) string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	b, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "app", "ktw-agents", "embed", "cwm", "object-panels", "files.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// THE FILES TAB: an object with attaches edges (paths in its module) gets the
// shipped Files panel, its source lists the files — name, folder, size, media
// type, a gone file flagged — and picks the one asked, else the index. An
// object attaching nothing has no Files tab.
func TestTheFilesPanelListsWhatAnObjectAttaches(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml":  "name: thing\ntree: node\n",
		"edge-types/attaches.yaml": "name: attaches\ncardinality: M:N\ntarget: path\nobject_storage: edges\n",
		"object-panels/files.yaml": shippedFilesPanel(t),
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.study", Type: "thing"})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.lone", Type: "thing"})
	for _, p := range []string{"study/index.md", "study/01-context.md", "study/media/flow.drawio", "notes.png"} {
		if _, err := cwm.ObjectFileWrite(ctx, cwmapi.ObjectFileWriteInput{ID: "m.study", Path: p, Content: []byte("x")}); err != nil {
			t.Fatal(err)
		}
	}
	att, err := cwm.ObjectFileAttach(ctx, cwmapi.ObjectFileAttachInput{ID: "m.study", Patterns: []string{"study/*.md", "notes.png"}})
	if err != nil || len(att.Added) != 3 {
		t.Fatalf("attach: %+v %v", att, err)
	}

	if _, ok := panelByID(detail(t, ctx, svc, "m.study", false).Panels)["tab:files"]; !ok {
		t.Fatalf("no Files tab on an object that attaches files: %v", ids(detail(t, ctx, svc, "m.study", false).Panels))
	}
	if _, ok := panelByID(detail(t, ctx, svc, "m.lone", false).Panels)["tab:files"]; ok {
		t.Error("a Files tab on an object that attaches nothing")
	}

	out, err := svc.AttachedFiles(ctx, api.AttachedFilesInput{ID: "m.study"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 3 {
		t.Fatalf("files: %+v", out.Files)
	}
	root, study := out.Files[0], out.Files[2]
	if root.Path != "notes.png" || root.Folder != "" || root.MediaType != "image/png" || root.SizeBytes != 1 {
		t.Errorf("root file: %+v", root)
	}
	if study.Path != "study/index.md" || study.Folder != "study" || study.MediaType != "text/markdown" || study.ProjectPath != "modules/m/study/index.md" {
		t.Errorf("study file: %+v", study)
	}
	if out.Selected != "study/index.md" {
		t.Errorf("selected %q, want the index", out.Selected)
	}
	asked, _ := svc.AttachedFiles(ctx, api.AttachedFilesInput{ID: "m.study", File: "modules/m/study/01-context.md"})
	if asked.Selected != "study/01-context.md" {
		t.Errorf("the file asked (project path) is not selected: %q", asked.Selected)
	}

	// A file removed outside CWM: still listed (the edge is), flagged missing.
	if _, err := cwm.ObjectFileDelete(ctx, cwmapi.ObjectFileDeleteInput{ID: "m.study", Path: "study/01-context.md"}); err != nil {
		t.Fatal(err)
	}
	gone, _ := svc.AttachedFiles(ctx, api.AttachedFilesInput{ID: "m.study", File: "study/01-context.md"})
	for _, f := range gone.Files {
		if f.Path == "study/01-context.md" && !f.Missing {
			t.Errorf("a gone file is not flagged: %+v", f)
		}
	}
	if gone.Selected != "study/index.md" {
		t.Errorf("a gone file is selected: %q", gone.Selected)
	}
}

// A NAMED FILE in the Files tab: an object's own named file that exists (a
// feedback request's screenshot) is listed with its name, its media type and
// its size, beside the files it attaches; one declared but not written is not,
// nor its type's file (machinery its own panels show).
func TestTheFilesPanelListsAnObjectsNamedFiles(t *testing.T) {
	ctx, cwm, svc := paneWorld(t, map[string]string{
		"object-types/thing.yaml": "name: thing\ntree: node\nfiles:\n  record:\n    path: \"record.yaml\"\n",
	})
	create(t, ctx, cwm, cwmapi.Object{ID: "m.ask", Type: "thing"})
	if _, err := cwm.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{ID: "m.ask", Name: "record", Content: []byte("a: 1")}); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]string{"screenshot": "feedback/ask.png", "later": "feedback/later.png"} {
		if _, err := cwm.ObjectNamedFileDeclare(ctx, cwmapi.ObjectNamedFileDeclareInput{ID: "m.ask", Name: name,
			File: cwmapi.NamedFile{Path: p, Media: "image/png"}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cwm.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{ID: "m.ask", Name: "screenshot", Content: []byte("png")}); err != nil {
		t.Fatal(err)
	}

	out, err := svc.AttachedFiles(ctx, api.AttachedFilesInput{ID: "m.ask"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 {
		t.Fatalf("files: %+v", out.Files)
	}
	f := out.Files[0]
	if f.Path != "feedback/ask.png" || f.Folder != "feedback" || f.Named != "screenshot" || f.Kind != api.NamedFileKind ||
		f.MediaType != "image/png" || f.SizeBytes != 3 || f.ProjectPath != "modules/m/feedback/ask.png" {
		t.Errorf("named file: %+v", f)
	}
	if out.Selected != "feedback/ask.png" {
		t.Errorf("selected %q, want the screenshot", out.Selected)
	}
}
