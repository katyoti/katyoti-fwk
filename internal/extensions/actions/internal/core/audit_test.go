// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	journallocal "github.com/katyoti/katyoti-fwk/internal/extensions/journal/local"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// An action run on an object is in the object's history (the journal): its
// URI and how it ended — completed, a task launched, or failed with what
// failed, cut to a line. No object is written for it.
func TestAnInvocationIsJournaled(t *testing.T) {
	ctx, cwm := cwmapp.New(t, cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml": {Data: []byte("name: module\n")},
		"object-types/node.yaml":   {Data: []byte("name: node\n")},
	}), journalcontrib.Pack(""), journallocal.Extension(), cwmapp.SystemModules(t))
	cwmapp.Module(t, ctx, cwm, "m")
	node := cwmapi.NewContainerObject("m", "node", "node", "m", "Node", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: node}); err != nil {
		t.Fatal(err)
	}
	s := &Service{app: application.AppFromContext(ctx)}
	uri, err := mbapi.ParseToolURI("ktw-test:inbox/file")
	if err != nil {
		t.Fatal(err)
	}
	input := []byte(`{"node_id":"m.node","description":"why"}`)
	s.recordInvocation(ctx, uri, input, actionsapi.RunOutput{}, nil)
	s.recordInvocation(ctx, uri, input, actionsapi.RunOutput{TaskID: "run-7"}, nil)
	s.recordInvocation(ctx, uri, input, actionsapi.RunOutput{}, errors.New(strings.Repeat("the action failed: ", 600)))

	journal, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := journal.Query(ctx, journalapi.QueryInput{ObjectID: "m.node", Actions: []string{journalapi.ActionInvoked}})
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]journalapi.Record{}
	for _, r := range got.Records {
		status[r.Status] = r
	}
	if len(got.Records) != 3 || status["ok"].URI != "ktw-test:inbox/file" || status["launched"].Note != "task run-7 started" {
		t.Fatalf("records %+v; want completed, launched and failed", got.Records)
	}
	if f := status["failed"]; !strings.HasPrefix(f.Note, "the action failed") || utf8.RuneCountInString(f.Note) > 201 {
		t.Errorf("the failure's note: %d characters; want what failed, cut to 200", utf8.RuneCountInString(f.Note))
	}
	if list, _ := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "m"}); len(list.Objects) != 2 {
		t.Errorf("objects %d; want the module and its node only", len(list.Objects))
	}
}
