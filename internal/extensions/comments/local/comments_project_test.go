// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/comments/api"
	commentscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/comments/contrib"
	commentslocal "github.com/katyoti/katyoti-fwk/internal/extensions/comments/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE PROJECT OBJECT HAS COMMENTS. It is the one object filed outside the
// modules, so its named files live in its own committed space, `<project>/.ktw/`
// (identity-and-index.md §7). Before, every named-file verb refused it ("belongs
// to no module") and the GUI's Comments section on the project showed an
// internal error.
func TestCommentsOnTheProjectObject(t *testing.T) {
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(fstest.MapFS{
			"capabilities/container.yaml": {Data: []byte("name: container\n")},
			"object-types/project.yaml":   {Data: []byte("name: project\ncapabilities: [project, container]\n")},
		}),
		commentscontrib.Pack(""),
		commentslocal.Extension(),
	)
	proj, err := cwm.ProjectInit(ctx, cwmapi.ProjectInitInput{Name: "acme", Title: "Acme"})
	if err != nil {
		t.Fatalf("ProjectInit: %v", err)
	}
	id := proj.Object.ID
	if _, err := cwm.ObjectScan(ctx, cwmapi.ObjectScanInput{}); err != nil {
		t.Fatal(err)
	}

	// The named files, straight through CWM: list, write, read.
	ls, err := cwm.ObjectNamedFileList(ctx, cwmapi.ObjectNamedFileListInput{ID: id})
	if err != nil {
		t.Fatalf("named-file list on the project: %v", err)
	}
	found := false
	for _, f := range ls.Files {
		if f.Capability == "comments" && f.Name == "thread" {
			found = true
			if f.Error != "" || f.Exists {
				t.Errorf("the thread before any comment: %+v", f)
			}
		}
	}
	if !found {
		t.Fatalf("the comments thread is not in force on the project: %+v", ls.Files)
	}
	w, err := cwm.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{
		ID: id, Capability: "comments", Name: "thread", Content: []byte("[]\n"),
	})
	if err != nil {
		t.Fatalf("named-file write on the project: %v", err)
	}
	if w.ProjectPath != ".ktw/comments/acme.yaml" {
		t.Errorf("filed at %q, want .ktw/comments/acme.yaml", w.ProjectPath)
	}
	r, err := cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: id, Capability: "comments", Name: "thread"})
	if err != nil || strings.TrimSpace(string(r.Content)) != "[]" || !r.File.Exists {
		t.Fatalf("named-file read on the project: %q exists=%v, %v", r.Content, r.File.Exists, err)
	}
	if _, err := cwm.ObjectFileDelete(ctx, cwmapi.ObjectFileDeleteInput{ID: id, Path: "comments/acme.yaml"}); err != nil {
		t.Fatal(err)
	}

	// The comments service, unchanged: an empty thread, then one comment.
	svc, err := application.UseService[api.CommentsService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := svc.List(ctx, api.ListInput{ID: id})
	if err != nil {
		t.Fatalf("list on the project: %v", err)
	}
	if len(empty.Comments) != 0 {
		t.Errorf("no comment yet, got %+v", empty.Comments)
	}
	ana := cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}
	if _, err := svc.Add(ctx, api.AddInput{ID: id, Text: "about the whole project", By: ana}); err != nil {
		t.Fatalf("add on the project: %v", err)
	}
	list, err := svc.List(ctx, api.ListInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Comments) != 1 || list.Comments[0].Text != "about the whole project" {
		t.Errorf("list = %+v", list.Comments)
	}
}
