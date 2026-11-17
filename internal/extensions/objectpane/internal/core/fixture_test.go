// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// THE FIXTURE SOURCE: a panel declared with it answers the JSON file its
// extension ships for the state asked - given in the panel's input, else read
// from the object, else `default`; an unknown state, a state no file name can
// carry or a missing file answers nothing (the panel's empty state), never an
// error; a fixture that is not a reference URI is the panel's authoring error.
func TestTheFixtureSourceAnswersTheFileOfTheState(t *testing.T) {
	const base = "acme:/reference/mock/board"
	files := map[string]string{
		base + ".default.json": `{"rows":[{"name":"default"}]}`,
		base + ".full.json":    `{"rows":[{"name":"one"},{"name":"two"}]}`,
		base + ".broken.json":  `{"rows":`,
	}
	read := func(_ context.Context, uri string) ([]byte, bool, error) {
		b, ok := files[uri]
		return []byte(b), ok, nil
	}
	study := cwmapi.Object{ID: "m.study", Type: "thing", Fields: map[string]any{"mock": "full"},
		Capabilities: map[string]map[string]any{"demo": {"stage": "default"}}}
	object := func(_ context.Context, id string) (cwmapi.Object, error) {
		if id != study.ID {
			return cwmapi.Object{}, cwmapi.ErrNotFound
		}
		return study, nil
	}
	ctx := context.Background()
	answer := func(in api.FixtureInput) string {
		t.Helper()
		in.Fixture = base
		got, err := fixture(ctx, in, read, object)
		if err != nil {
			t.Fatalf("%+v: %v", in, err)
		}
		return string(got)
	}
	cases := []struct {
		name string
		in   api.FixtureInput
		want string
	}{
		{"the state asked", api.FixtureInput{State: "full"}, files[base+".full.json"]},
		{"no state: default", api.FixtureInput{}, files[base+".default.json"]},
		{"the object's field", api.FixtureInput{ID: "m.study", StateField: "fields.mock"}, files[base+".full.json"]},
		{"a capability's field", api.FixtureInput{ID: "m.study", StateField: "capabilities.demo.stage"}, files[base+".default.json"]},
		{"the state asked wins", api.FixtureInput{ID: "m.study", StateField: "fields.mock", State: "default"}, files[base+".default.json"]},
		{"a field the object lacks: default", api.FixtureInput{ID: "m.study", StateField: "fields.none"}, files[base+".default.json"]},
		{"an unknown state: empty", api.FixtureInput{State: "gone"}, ""},
		{"a path as state: empty", api.FixtureInput{State: "../board.full"}, ""},
	}
	for _, c := range cases {
		if got := answer(c.in); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}

	// A missing file answers nothing, whatever the state.
	if got, err := fixture(ctx, api.FixtureInput{Fixture: "acme:/reference/mock/other"}, read, object); err != nil || got != nil {
		t.Errorf("missing file: got %q, %v; want nothing", got, err)
	}
	// Authoring errors say so: a fixture that is no reference URI, a file
	// that is not JSON.
	for _, bad := range []string{"", "board", "acme:/skill/board"} {
		if _, err := fixture(ctx, api.FixtureInput{Fixture: bad}, read, object); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("fixture %q: err %v, want invalid input", bad, err)
		}
	}
	if _, err := fixture(ctx, api.FixtureInput{Fixture: base, State: "broken"}, read, object); err == nil {
		t.Error("a fixture that is not JSON answered no error")
	}
}

// The fixture source is every application's: its code names none.
func TestTheFixtureSourceNamesNoApplication(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(".", "fixture.go"))
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`(?i)archi|asis|etude|dossier|v6`).FindString(string(b)); m != "" {
		t.Errorf("fixture.go names an application: %q", m)
	}
	if !strings.Contains(string(b), "func fixture(") {
		t.Fatal("fixture.go no longer holds the fixture source")
	}
}
