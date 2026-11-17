// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

type testSkill struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type testTask struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func makeManifest() *Manifest {
	body := func(v any) json.RawMessage {
		b, _ := json.Marshal(v)
		return b
	}
	return &Manifest{
		Provider: "ktw-test",
		Version:  "1.0.0",
		Contributions: []contribapi.Contribution{
			{Provider: "ktw-test", Type: "skill", ID: "intro", Layers: contribapi.InlineData(body(testSkill{ID: "intro", Title: "Intro"}))},
			{Provider: "ktw-test", Type: "skill", ID: "advanced", Layers: contribapi.InlineData(body(testSkill{ID: "advanced", Title: "Advanced"}))},
			{Provider: "ktw-test", Type: "task", ID: "deploy", Layers: contribapi.InlineData(body(testTask{Name: "deploy", Kind: "agent"}))},
			{Provider: "ktw-test", Type: "reference", ID: "templates/x.typ"},
		},
	}
}

func TestGetResources_TypedDecode(t *testing.T) {
	m := makeManifest()
	skills, err := GetContributions[testSkill](m, "skill")
	if err != nil {
		t.Fatalf("GetContributionsByType skill: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("got %d skills, want 2", len(skills))
	}
	if skills[0].ID != "intro" || skills[0].Title != "Intro" {
		t.Errorf("skills[0] = %+v", skills[0])
	}
	if skills[1].ID != "advanced" || skills[1].Title != "Advanced" {
		t.Errorf("skills[1] = %+v", skills[1])
	}

	tasks, _ := GetContributions[testTask](m, "task")
	if len(tasks) != 1 {
		t.Fatalf("got %d tasks, want 1", len(tasks))
	}
	if tasks[0].Name != "deploy" || tasks[0].Kind != "agent" {
		t.Errorf("tasks[0] = %+v", tasks[0])
	}
}

func TestGetResources_UnknownType_Empty(t *testing.T) {
	m := makeManifest()
	out, err := GetContributions[testSkill](m, "nonexistent")
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %d", len(out))
	}
}

func TestGetResources_NilManifest(t *testing.T) {
	out, err := GetContributions[testSkill](nil, "skill")
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if out != nil {
		t.Errorf("expected nil slice, got %v", out)
	}
}

func TestResourceEntry_URI(t *testing.T) {
	e := contribapi.Contribution{Provider: "ktw-docs", Type: "skill", ID: "doc-create"}
	want := "ktw-docs:/skill/doc-create"
	if e.ResolvedURI() != want {
		t.Errorf("URI = %q, want %q", e.ResolvedURI(), want)
	}
}

func TestGetResources_DecodeError_Surfaces(t *testing.T) {
	m := &Manifest{
		Contributions: []contribapi.Contribution{
			{Type: "skill", ID: "broken", Layers: contribapi.InlineData(json.RawMessage(`{not json`))},
		},
	}
	_, err := GetContributions[testSkill](m, "skill")
	if err == nil {
		t.Errorf("expected decode error, got nil")
	}
}
