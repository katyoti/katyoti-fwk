// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// A declared source carrying an error reads « injoignable », red, its error as
// the note; located, without an error, it reads « pas encore lue »
func TestCardOfADeclaredSourceCarryingAnErrorSaysUnreachable(t *testing.T) {
	var u api.ConnectorUse
	card(&u, api.SourceSummary{State: api.SourceStateDeclared, Error: "connexion refusée"})
	if u.State == nil || u.State.Text != "injoignable" || u.State.Color != "red" {
		t.Fatalf("state = %+v, want injoignable/red", u.State)
	}
	if u.Note != "connexion refusée" {
		t.Fatalf("note = %q, want the error", u.Note)
	}

	var plain api.ConnectorUse
	card(&plain, api.SourceSummary{State: api.SourceStateDeclared, Located: true})
	if plain.State == nil || plain.State.Text != "pas encore lue" || plain.State.Color != "gray" || plain.Note != "" {
		t.Fatalf("state = %+v note = %q, want pas encore lue/gray, no note", plain.State, plain.Note)
	}
}

// A used source that locates and picks nothing reads « à configurer », gray,
// telling what to do; a located or a picked one not yet read still reads
// « pas encore lue », an error still « injoignable ».
func TestADeclaredSourceWithNothingPickedIsToConfigure(t *testing.T) {
	var u api.ConnectorUse
	card(&u, summaryOf(cwmapi.Object{ID: "m.s", Fields: map[string]any{"connector": "files", "location": map[string]any{"path": " "}}}, "child"))
	if u.State == nil || u.State.Text != "à configurer" || u.State.Color != "gray" || u.Note != "Choisir… pour dire quoi lire" {
		t.Fatalf("state = %+v note = %q, want à configurer/gray with what to do", u.State, u.Note)
	}
	for name, src := range map[string]api.SourceSummary{
		"located": {State: api.SourceStateDeclared, Located: true},
		"picked":  {State: api.SourceStateDeclared, Picked: 1},
		"held":    {State: api.SourceStateDeclared, Held: 1},
	} {
		var v api.ConnectorUse
		card(&v, src)
		if v.State == nil || v.State.Text != "pas encore lue" {
			t.Fatalf("%s: state = %+v, want pas encore lue", name, v.State)
		}
	}
	var failed api.ConnectorUse
	card(&failed, api.SourceSummary{State: api.SourceStateDeclared, Error: "connexion refusée"})
	if failed.State == nil || failed.State.Text != "injoignable" {
		t.Fatalf("error: state = %+v, want injoignable", failed.State)
	}
}
