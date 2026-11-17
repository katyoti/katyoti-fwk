// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// : a case drives an attended task's forms as the GUI does. A
// start_task step starts the task with the GUI chat's context (_attended,
// _target); an answer_form step finds, in the session's user journal, the form
// a run shows and has not had answered - the one whose question it names -
// submits its value as a ui_event on the form's artifact, and knows the run
// took it once its user_input follows.
func TestACaseAnswersAnAttendedTasksFormsAsTheGUIDoes(t *testing.T) {
	in := startTaskInput("new-study-v1", "dossier.M-1", map[string]any{"user_input": "go"})
	ctx := in["context"].(map[string]any)
	if in["taskName"] != "new-study-v1" || ctx["_attended"] != true || ctx["_target"] != "dossier.M-1" || in["input"].(map[string]any)["user_input"] != "go" {
		t.Fatalf("StartTask input %+v; want the task attended on its target", in)
	}
	if c := startTaskInput("t", "", nil)["context"].(map[string]any); c["_target"] != nil || c["_attended"] != true {
		t.Fatalf("on no object the context is %+v", c)
	}

	journal := []byte(strings.Join([]string{
		`{"type":"task_started","seq":1,"data":{"emitter":"run-a"}}`,
		`{"type":"prompt_shown","seq":7,"data":{"emitter":"run-a","question":"How would you like to create the dossier?","channel":"form","artifactId":"ask-user-data:run-a","title":"New dossier"}}`,
		`{"type":"user_input","seq":8,"data":{"emitter":"run-a","answer":"(formulaire envoyé)"}}`,
		`{"type":"prompt_shown","seq":12,"data":{"emitter":"run-a","question":"Le dossier","channel":"form","artifactId":"ask-user-data:run-a","title":"Dossier d'architecture"}}`,
		`{"type":"prompt_shown","seq":13,"data":{"emitter":"run-b","question":"Une question","channel":"text"}}`,
	}, "\n"))
	f, ok := openForm(journal, "")
	if !ok || f.Seq != 12 || f.ArtifactID != "ask-user-data:run-a" {
		t.Fatalf("the open form is %+v %v; want the unanswered one of seq 12", f, ok)
	}
	if g, ok := openForm(journal, "dossier d'ARCHITECTURE"); !ok || g.Seq != 12 {
		t.Fatalf("by its title: %+v %v", g, ok)
	}
	if _, ok := openForm(journal, "How would you like"); ok {
		t.Fatal("an answered form is offered again")
	}
	if formTaken(journal, f) {
		t.Fatal("a form not answered reads taken")
	}
	if !formTaken(append(journal, []byte("\n"+`{"type":"user_input","seq":14,"data":{"emitter":"run-a"}}`)...), f) {
		t.Fatal("a form answered reads not taken")
	}

	cmd, err := submitCmd("r1", f, map[string]any{"mode": "new"})
	if err != nil {
		t.Fatal(err)
	}
	var sent struct {
		Type   string `json:"type"`
		Params struct {
			ArtifactID string         `json:"artifactId"`
			EventType  string         `json:"eventType"`
			Value      map[string]any `json:"value"`
		} `json:"params"`
	}
	if err := json.Unmarshal(cmd["command"].(json.RawMessage), &sent); err != nil || cmd["runID"] != "r1" || sent.Type != "ui_event" || sent.Params.ArtifactID != f.ArtifactID || sent.Params.EventType != "submit" || sent.Params.Value["mode"] != "new" {
		t.Fatalf("the submit is %+v (%v)", sent, err)
	}

	// The journal is read where the case's hub keeps its sessions: its data.
	rt := Runtime{Dir: t.TempDir()}
	if got, want := rt.Sessions(), filepath.Join(rt.Data(), "sessions"); got != want {
		t.Fatalf("sessions read in %s; want the case's %s", got, want)
	}
}
