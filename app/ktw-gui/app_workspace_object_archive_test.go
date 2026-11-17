// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : « Retirer » archives the object through cwm's ObjectArchive
// and the hard delete is offered only when no object links to it.
func TestRetirerArchivesTheObject(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{"ktw-agents:cwm/object-archive": cwmapi.ObjectArchiveOutput{}}}
	if r := objectArchiveVia(context.Background(), s, "m.G-001"); r.Error != "" {
		t.Fatalf("archive: %s", r.Error)
	}
	if in := s.inputFor("ktw-agents:cwm/object-archive").(cwmapi.ObjectArchiveInput); in.ID != "m.G-001" {
		t.Errorf("ObjectArchive asked for %+v, want the object's id", in)
	}
	failing := &multiStub{t: t, errs: map[string]error{"ktw-agents:cwm/object-archive": errors.New("conflict")}}
	if r := objectArchiveVia(context.Background(), failing, "m.G-001"); r.Error == "" {
		t.Error("a refused archive answered no error")
	}
	if r := objectArchiveVia(context.Background(), s, ""); r.Error == "" {
		t.Error("an archive with no id answered no error")
	}
}

func TestTheHardDeleteSeesWhatPointsToTheObject(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{"ktw-agents:cwm/edge-list": cwmapi.EdgeListOutput{Edges: []cwmapi.Edge{
		{From: "b.F-2", To: "a.G-1", Kind: "serves"},
		{From: "b.F-1", To: "a.G-1", Kind: "serves"},
		{From: "b.F-1", To: "a.G-1", Kind: "depends-on"},
	}}}}
	got := objectInboundVia(context.Background(), s, "a.G-1")
	if got.Error != "" || !reflect.DeepEqual(got.From, []string{"b.F-1", "b.F-2"}) {
		t.Fatalf("inbound = %+v, want b.F-1 and b.F-2 once each", got)
	}
	if in := s.inputFor("ktw-agents:cwm/edge-list").(cwmapi.EdgeListInput); in.ObjectID != "a.G-1" || in.Direction != cwmapi.DirectionIn {
		t.Errorf("EdgeList asked for %+v, want the incoming edges of a.G-1", in)
	}
	none := &multiStub{t: t, replies: map[string]any{"ktw-agents:cwm/edge-list": cwmapi.EdgeListOutput{}}}
	if got := objectInboundVia(context.Background(), none, "a.G-1"); got.Error != "" || len(got.From) != 0 || got.From == nil {
		t.Errorf("no link: %+v, want an empty list", got)
	}
}
