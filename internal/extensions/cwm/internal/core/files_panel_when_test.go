// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE FILES PANEL'S CONDITION reads an object's own named files: the shipped
// panel's `when`, evaluated as the object pane does (ObjectEvaluate, no costly
// function), admits an object whose only file is a named one (a request's
// screenshot) and refuses one that names and attaches nothing.
func TestTheFilesPanelShowsOnAnObjectWithANamedFile(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "..", "app", "ktw-agents", "embed", "cwm", "object-panels", "files.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var panel struct {
		Filter api.ObjectFilter `yaml:"filter"`
	}
	if err := config.DecodeBytes(raw, marshal.YAML, &panel); err != nil || panel.Filter.When == "" {
		t.Fatalf("the shipped Files panel has no when: %+v %v", panel.Filter, err)
	}
	when := panel.Filter.When

	svc, _ := sourcesFixture(t)
	ctx := stdctx.Background()
	shown := func() any {
		t.Helper()
		out, err := svc.ObjectEvaluate(ctx, api.ObjectEvaluateInput{ID: "billing.spec", Expressions: []string{when}, NoCostly: true})
		if err != nil || len(out.Results) != 1 || out.Results[0].Error != "" {
			t.Fatalf("evaluate %q: %+v %v", when, out, err)
		}
		return out.Results[0].Value
	}
	if v := shown(); v != false {
		t.Errorf("an object naming and attaching nothing: the Files panel's when answers %v, want false", v)
	}
	if _, err := svc.ObjectNamedFileDeclare(ctx, api.ObjectNamedFileDeclareInput{ID: "billing.spec", Name: "screenshot",
		File: api.NamedFile{Path: "feedback/ask.png", Media: "image/png"}}); err != nil {
		t.Fatal(err)
	}
	if v := shown(); v != true {
		t.Errorf("an object whose only file is its screenshot: the Files panel's when answers %v, want true", v)
	}
}
