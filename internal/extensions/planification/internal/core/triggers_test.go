// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// The shipped triggers load; a trigger that could not fire or could not file
// is refused.
func TestTriggersAreChecked(t *testing.T) {
	got, err := loadTriggers()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, tr := range got {
		names = append(names, tr.Name)
	}
	if strings.Join(names, ",") != "decision-prepared,feature-changed,feature-size,goal-changed,goal-served,use-case-changed" {
		t.Errorf("shipped: %v", names)
	}
	for _, bad := range []string{
		"name: x\non: change\nfile: {kind: review, title: t}\n",                                          // no types
		"name: x\non: sometimes\ntypes: [goal]\nfile: {kind: review, title: t}\n",                        // no such on
		"name: x\non: rule\ntypes: [goal]\nfile: {kind: review, title: t}\n",                             // a rule with no check
		"name: x\non: change\ntypes: [goal]\nfile: {kind: dream, title: t}\n",                            // no such kind
		"name: x\non: change\ntypes: [goal]\ngrace: 1m\nfile: {kind: review, title: t}\n",                // a grace on a change
		"name: x\non: rule\ntypes: [goal]\ncheck: 'true'\ngrace: soon\nfile: {kind: review, title: t}\n", // a grace that is no duration
	} {
		if _, err := readTriggers(fstest.MapFS{"x.yaml": {Data: []byte(bad)}}, "."); err == nil {
			t.Errorf("accepted:\n%s", bad)
		}
	}
}

// What an object says changed — never what the work's process wrote on it.
func TestAChangeIsWhatTheObjectSays(t *testing.T) {
	base := cwmapi.Object{ID: "b.G-001", Type: "goal", Name: "Invoices are sent", Body: "sent",
		Capabilities: map[string]map[string]any{"want": {"reachability": "closable"}, "lifecycle": {"state": "accepted"}}}
	clone := func() cwmapi.Object {
		o := base
		o.Capabilities = map[string]map[string]any{}
		for k, v := range base.Capabilities {
			o.Capabilities[k] = v
		}
		return o
	}
	for _, c := range []struct {
		name string
		edit func(o *cwmapi.Object)
		want string
	}{
		{"nothing", func(*cwmapi.Object) {}, ""},
		{"a request filed on it", func(o *cwmapi.Object) { o.Capabilities[api.CapRequest] = map[string]any{"kind": "analyse"} }, ""},
		{"its plan", func(o *cwmapi.Object) { o.Capabilities[api.CapPlan] = map[string]any{"state": "ready"} }, ""},
		{"its state", func(o *cwmapi.Object) { o.Capabilities["lifecycle"] = map[string]any{"state": "decomposed"} }, ""},
		{"a gate confirmed", func(o *cwmapi.Object) { o.Capabilities["gates"] = map[string]any{"x": 1} }, ""},
		{"a release targeted", func(o *cwmapi.Object) { o.Edges = []cwmapi.ObjectEdge{{Kind: api.EdgeTargets, To: "b.REL-1"}} }, ""},
		{"its body", func(o *cwmapi.Object) { o.Body = "sent the same day" }, "its body changed"},
		{"its want", func(o *cwmapi.Object) { o.Capabilities["want"] = map[string]any{"reachability": "asymptotic"} }, "its want changed"},
		{"its own link", func(o *cwmapi.Object) { o.Edges = []cwmapi.ObjectEdge{{Kind: "serves", To: "b.G-002"}} }, "its links changed"},
	} {
		now := clone()
		c.edit(&now)
		if got := contentChange(base, now); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
