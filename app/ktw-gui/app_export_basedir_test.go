// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// statSender is CWM answering ObjectFileStat: a module file's path relative
// to the project.
type statSender struct{ projectPath string }

func (s statSender) Call(_ context.Context, path string, params any, _ ...ktsapi.CallOption) (json.RawMessage, error) {
	if path != "ktw-agents:cwm/object-file-stat" {
		return nil, fmt.Errorf("unexpected call %s", path)
	}
	in, _ := params.(cwmapi.ObjectFileStatInput)
	return json.Marshal(cwmapi.ObjectFileStatOutput{Entry: cwmapi.FileEntry{Path: in.Path}, ProjectPath: s.projectPath})
}

func (statSender) Notify(context.Context, string, any, ...ktsapi.CallOption) error { return nil }

// TestTheExportOfAFileResolvesItsReferencesBesideTheFile -: the
// converter reads a copy of the document in a temp folder, so the export
// hands it the folder of the file the document was read from - an object's
// module file located on the project, or an absolute path - as BaseDir;
// a document with no file leaves it empty.
func TestTheExportOfAFileResolvesItsReferencesBesideTheFile(t *testing.T) {
	root := t.TempDir()
	studyDir := filepath.Join(root, "modules", "research-example")
	if err := os.MkdirAll(filepath.Join(studyDir, "media"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(studyDir, "study.md"), []byte("![flow](media/flow.drawio)"), 0o644); err != nil {
		t.Fatal(err)
	}
	sender := statSender{projectPath: "modules/research-example/study.md"}
	objectFile := func(path string) (string, error) {
		return objectFileAbs(context.Background(), sender, []string{root}, "u", "m.F-001", path)
	}

	// An object's module file: its folder on disk.
	got := exportBaseDir(&render.Source{Path: "study.md"}, "m.F-001", objectFile)
	if got != studyDir {
		t.Fatalf("object file base dir = %q, want %q", got, studyDir)
	}
	job, failed := newExportJob(render.Document{MediaType: render.MediaTypeMarkdown, Title: "study.md", Content: "![flow](media/flow.drawio)"}, render.MediaTypeDOCX, got)
	if job == nil {
		t.Fatalf("no export job: %s", failed.Error)
	}
	defer job.discard()
	call, ok := job.call.(docsapi.RenderInput)
	if !ok || call.BaseDir != studyDir {
		t.Fatalf("render call = %#v, want BaseDir %q", job.call, studyDir)
	}

	// An absolute file: its folder.
	abs := filepath.Join(studyDir, "study.md")
	if got := exportBaseDir(&render.Source{Path: abs}, "", objectFile); got != studyDir {
		t.Errorf("absolute file base dir = %q, want %q", got, studyDir)
	}
	// No file, a contribution, a file under no project root: none.
	if got := exportBaseDir(nil, "m.F-001", objectFile); got != "" {
		t.Errorf("inline document base dir = %q, want empty", got)
	}
	if got := exportBaseDir(&render.Source{URI: "ktw-example:/skill/x"}, "", objectFile); got != "" {
		t.Errorf("contribution base dir = %q, want empty", got)
	}
	lost := func(path string) (string, error) {
		return objectFileAbs(context.Background(), statSender{projectPath: "elsewhere/study.md"}, []string{root}, "u", "m.F-001", path)
	}
	if got := exportBaseDir(&render.Source{Path: "study.md"}, "m.F-001", lost); got != "" {
		t.Errorf("unlocated file base dir = %q, want empty", got)
	}
}
