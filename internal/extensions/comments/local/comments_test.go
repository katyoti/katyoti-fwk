// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/comments/api"
	commentscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/comments/contrib"
	commentslocal "github.com/katyoti/katyoti-fwk/internal/extensions/comments/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	ocapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	oclocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// COMMENTS ARE PART OF EVERY OBJECT AND NEVER CHANGE IT. A comment is added,
// answered and listed; a chat on the object sees the thread; the thread
// follows the object when a move gives it a new identity, and goes with it.
func TestCommentsLiveWithTheObjectAndNeverChangeIt(t *testing.T) {
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(fstest.MapFS{
			"capabilities/container.yaml": {Data: []byte("name: container\n")},
			"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container]\n")},
			"object-types/note.yaml":      {Data: []byte("name: note\ncapabilities: [container]\n")},
		}),
		commentscontrib.Pack(""),
		commentslocal.Extension(),
		oclocal.Extension(),
	)
	cwmapp.Module(t, ctx, cwm, "billing")
	cwmapp.Module(t, ctx, cwm, "shipping")
	o := cwmapi.Object{ID: "billing.n1", Type: "note"}
	o.Name, o.Parent = "n1", "billing"
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
		t.Fatal(err)
	}
	before, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: "billing.n1"})
	svc, err := application.UseService[api.CommentsService](ctx)
	if err != nil {
		t.Fatal(err)
	}

	ana := cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}
	q, err := svc.Add(ctx, api.AddInput{ID: "billing.n1", Text: "Is this still true?", By: ana})
	if err != nil || q.Comment.ID != "c1" || q.Comment.At == "" {
		t.Fatalf("add: %+v %v", q, err)
	}
	bot := cwmapi.Actor{ID: "cwm-session", Kind: cwmapi.ActorAgent}
	if _, err := svc.Add(ctx, api.AddInput{ID: "billing.n1", Text: "Yes.", By: bot, ReplyTo: "c1"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	for name, in := range map[string]api.AddInput{
		"an answer to nothing": {ID: "billing.n1", Text: "?", By: ana, ReplyTo: "c9"},
		"no text":              {ID: "billing.n1", Text: " ", By: ana},
		"no author":            {ID: "billing.n1", Text: "hm"},
		"an author of no kind": {ID: "billing.n1", Text: "hm", By: cwmapi.Actor{ID: "user", Kind: "user"}},
	} {
		if _, err := svc.Add(ctx, in); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%s: %v, want refused", name, err)
		}
	}
	if _, err := svc.Add(ctx, api.AddInput{ID: "billing.none", Text: "x", By: ana}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Errorf("a comment on no object: %v", err)
	}
	list, err := svc.List(ctx, api.ListInput{ID: "billing.n1"})
	if err != nil || len(list.Comments) != 2 || list.Comments[1].ReplyTo != "c1" || list.Comments[1].By != bot {
		t.Fatalf("list: %+v %v", list, err)
	}
	after, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "billing", ID: "billing.n1"})
	if b, a := marshalled(before.Object), marshalled(after.Object); a != b {
		t.Error("a comment changed the object")
	}

	oc, err := application.UseService[ocapi.ObjectContextService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	ac, err := oc.AgentContext(ctx, ocapi.AgentContextInput{ID: "billing.n1"})
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, sec := range ac.System {
		seen = seen || strings.Contains(sec.Text, "Is this still true?")
	}
	if !seen {
		t.Errorf("the thread did not reach the chat's context: %+v", ac.System)
	}

	mv, err := cwm.ObjectMove(ctx, cwmapi.ObjectMoveInput{ID: "billing.n1", NewModule: "shipping", NewParent: "shipping", Reason: "belongs there"})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	moved, err := svc.List(ctx, api.ListInput{ID: mv.ID})
	if err != nil || len(moved.Comments) != 2 {
		t.Fatalf("comments after the move: %+v %v", moved, err)
	}
	files, _ := cwm.ObjectNamedFileList(ctx, cwmapi.ObjectNamedFileListInput{ID: mv.ID})
	var thread string
	for _, f := range files.Files {
		if f.Capability == api.Capability && f.Name == api.ThreadFile {
			thread = f.Path
		}
	}
	if _, err := cwm.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{Module: "shipping", ID: mv.ID}); err != nil {
		t.Fatal(err)
	}
	if st, err := cwm.ObjectFileStat(ctx, cwmapi.ObjectFileStatInput{ID: "shipping", Path: thread}); err == nil {
		t.Errorf("the thread outlived its object: %+v", st)
	} else if !errors.Is(err, cwmapi.ErrNotFound) && !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stat the thread: %v", err)
	}
}

func marshalled(o cwmapi.Object) string {
	b, _ := json.Marshal(o)
	return string(b)
}
