// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A deck's drawings are saved as text through the object's named file: the
// binding sends the text as the file's bytes and reports the file written.
func TestNamedFileWriteBinding_SendsTheTextAsTheFile(t *testing.T) {
	s := &stubSender{t: t, reply: cwmapi.ObjectNamedFileWriteOutput{
		File: cwmapi.ObjectNamedFile{Name: "drawings", Path: "decks/talk.drawings.json", Media: "application/json"},
	}}
	got := namedFileWriteVia(context.Background(), s, "talks.talk", "", "drawings", `{"version":7}`)
	if s.path != "ktw-agents:cwm/object-named-file-write" {
		t.Fatalf("path %q", s.path)
	}
	in, ok := s.input.(cwmapi.ObjectNamedFileWriteInput)
	if !ok || in.ID != "talks.talk" || in.Name != "drawings" || string(in.Content) != `{"version":7}` {
		t.Fatalf("input %+v", s.input)
	}
	if got.Error != "" || !got.Exists || got.Path != "decks/talk.drawings.json" {
		t.Fatalf("result %+v", got)
	}
	if bad := namedFileWriteVia(context.Background(), failingSender{}, "x", "", "drawings", "{}"); bad.Error == "" {
		t.Errorf("a failing call must surface as the file's Error")
	}
}
