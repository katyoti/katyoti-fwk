// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// The editor's Save must not rewrite members it did not touch. A content member
// (host + folder + env) and a store-pinned member the inventory could not see
// both survive verbatim; a newly selected name gets a fresh spec; a deselected
// name goes.
func TestMergeSidecarSelection_PreservesExistingMembers(t *testing.T) {
	old := []api.SidecarSpec{
		{Name: "ktw-example", Transport: "nats", Binary: `D:\root\bin\store\ktw-example\v1\ktw-example.exe`, Args: []string{"sc", "ktsp", "nats"}},
		{Name: "acme-playbooks", Transport: "nats", Binary: `D:\root\bin\ktw-host.exe`, ContentRoot: `D:\root\bin\store-content\acme-playbooks\v1`,
			Args: []string{"--folder", `D:\root\bin\store-content\acme-playbooks\v1`, "sc", "ktsp", "nats"}, Env: map[string]string{"KTW_CONTENT_ROOT": `D:\root\bin\store-content\acme-playbooks\v1`}},
		{Name: "ktw-weather", Transport: "nats", Binary: `D:\root\bin\store\ktw-weather\v1\ktw-weather.exe`},
	}
	sel := []api.CreateSidecarSelection{
		{Name: "ktw-example", BinaryPath: ""},                                        // inventory could not see it: must NOT become a bare name
		{Name: "acme-playbooks", BinaryPath: ""},                                     // content: must keep host + folder + env
		{Name: "ktw-docs", BinaryPath: `D:\root\bin\store\ktw-docs\v1\ktw-docs.exe`}, // new
		// ktw-weather deselected
	}
	got := mergeSidecarSelection(old, sel)
	if len(got) != 3 {
		t.Fatalf("want 3 members, got %+v", got)
	}
	if got[0].Binary != old[0].Binary {
		t.Fatalf("existing pinned member was rewritten: %+v", got[0])
	}
	if got[1].ContentRoot != old[1].ContentRoot || len(got[1].Args) != 5 || got[1].Env["KTW_CONTENT_ROOT"] == "" {
		t.Fatalf("content member lost its host/folder/env: %+v", got[1])
	}
	if got[2].Name != "ktw-docs" || got[2].Binary != `D:\root\bin\store\ktw-docs\v1\ktw-docs.exe` || got[2].Transport != "nats" {
		t.Fatalf("new member not built from its selection: %+v", got[2])
	}
	for _, sc := range got {
		if sc.Name == "ktw-weather" {
			t.Fatal("deselected member must be dropped")
		}
	}
}
