// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// A task's card carries whom the task is for, its kind and its types, read
// from its catalog body: the task lists keep a tagged universe's
// first level to the end-user tasks not dedicated to an object type.
func TestTaskInfoAudience(t *testing.T) {
	var b mbapi.TaskBody
	if err := json.Unmarshal([]byte(`{"name":"slidedeck-fast-create","type":"agent","audience":"end-user","kind":"object","types":["*"]}`), &b); err != nil {
		t.Fatal(err)
	}
	b.Normalize()
	info := taskInfoOf("ktw-docs", "ktw-docs:slidedeck-fast-create", b)
	if info.Audience != "end-user" || info.Kind != "object" || len(info.Types) != 1 || info.Types[0] != "*" {
		t.Errorf("card %+v: want audience end-user, kind object, types [*]", info)
	}
	out, _ := json.Marshal(info)
	var wire map[string]any
	_ = json.Unmarshal(out, &wire)
	if wire["audience"] != "end-user" {
		t.Errorf("the frontend reads %v as audience, want end-user", wire["audience"])
	}
}
