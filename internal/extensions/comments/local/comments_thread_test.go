// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/comments/api"
	commentscontrib "github.com/katyoti/katyoti-fwk/internal/extensions/comments/contrib"
	commentslocal "github.com/katyoti/katyoti-fwk/internal/extensions/comments/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THREADS AND ORDER. Answers hang under the comment they answer (an answer may
// answer an answer), several threads share one object, and the list is oldest
// first with ids that follow the order of saying. An object nobody commented
// on lists nothing; an unknown object is not found.
func TestCommentsThreadsAndOrder(t *testing.T) {
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(fstest.MapFS{
			"capabilities/container.yaml": {Data: []byte("name: container\n")},
			"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container]\n")},
			"object-types/note.yaml":      {Data: []byte("name: note\ncapabilities: [container]\n")},
		}),
		commentscontrib.Pack(""),
		commentslocal.Extension(),
	)
	cwmapp.Module(t, ctx, cwm, "billing")
	for _, name := range []string{"n1", "quiet"} {
		o := cwmapi.Object{ID: "billing." + name, Type: "note"}
		o.Name, o.Parent = name, "billing"
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := application.UseService[api.CommentsService](ctx)
	if err != nil {
		t.Fatal(err)
	}

	ana := cwmapi.Actor{ID: "ana", Kind: cwmapi.ActorPerson}
	bot := cwmapi.Actor{ID: "bot", Kind: cwmapi.ActorAgent}
	mod := cwmapi.Actor{ID: "billing", Kind: "module"}
	says := []api.AddInput{
		{Text: "first thread", By: ana},
		{Text: "answer to c1", By: bot, ReplyTo: "c1"},
		{Text: "second thread", By: mod},
		{Text: "answer to the answer", By: ana, ReplyTo: "c2"},
		{Text: "answer to c3", By: bot, ReplyTo: "c3"},
	}
	for i, in := range says {
		in.ID = "billing.n1"
		out, err := svc.Add(ctx, in)
		if err != nil {
			t.Fatalf("add %d: %v", i, err)
		}
		if want := "c" + string(rune('1'+i)); out.Comment.ID != want {
			t.Errorf("add %d got id %q, want %q", i, out.Comment.ID, want)
		}
	}

	list, err := svc.List(ctx, api.ListInput{ID: "billing.n1"})
	if err != nil {
		t.Fatal(err)
	}
	type row struct{ id, text, replyTo string }
	var got []row
	for _, c := range list.Comments {
		got = append(got, row{c.ID, c.Text, c.ReplyTo})
		if c.At == "" {
			t.Errorf("%s has no time", c.ID)
		}
	}
	want := []row{
		{"c1", "first thread", ""},
		{"c2", "answer to c1", "c1"},
		{"c3", "second thread", ""},
		{"c4", "answer to the answer", "c2"},
		{"c5", "answer to c3", "c3"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("list =\n %v\nwant\n %v", got, want)
	}
	if list.Comments[2].By != mod {
		t.Errorf("a module's comment kept its author? %+v", list.Comments[2].By)
	}

	// A reply names a comment of THIS object: c1 of billing.n1 is not one of billing.quiet's.
	if _, err := svc.Add(ctx, api.AddInput{ID: "billing.quiet", Text: "x", By: ana, ReplyTo: "c1"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("an answer to another object's comment: %v, want refused", err)
	}
	quiet, err := svc.List(ctx, api.ListInput{ID: "billing.quiet"})
	if err != nil || quiet.Comments == nil || len(quiet.Comments) != 0 {
		t.Errorf("an object nobody commented on: %#v %v, want an empty list", quiet.Comments, err)
	}
	if _, err := svc.List(ctx, api.ListInput{ID: "billing.none"}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Errorf("the comments of no object: %v, want not found", err)
	}
	if _, err := svc.List(ctx, api.ListInput{}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("list without an id: %v, want refused", err)
	}
}
