// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"testing"
	"testing/fstest"
)

// The pack loader reads every declaration kind through its strict codec: a
// file carrying a key no field takes is skipped (logged, naming it), and its
// valid neighbour in the same folder still loads — one bad file never takes
// the pack down.
func TestThePackLoaderSkipsADeclarationWithAnUnknownKey(t *testing.T) {
	src := fstest.MapFS{
		"capabilities/good.yaml": {Data: []byte("name: good\nlabel: Good\n")},
		"capabilities/bad.yaml":  {Data: []byte("name: bad\nstorage: inline\n")},
		"edge-types/good.yaml":   {Data: []byte("name: good\nfrom: any\nto: any\n")},
		"edge-types/bad.yaml":    {Data: []byte("name: bad\nfrom: any\nto: any\ncardinalty: N:1\n")},
		"aspects/good.yaml":      {Data: []byte("name: good\ncapabilities: [good]\n")},
		"aspects/bad.yaml":       {Data: []byte("name: bad\ncapabilities: [good]\ncwm_context: {}\n")},
		"workflows/good.yaml":    {Data: []byte("name: good\ninitial: a\nstates: [{name: a}]\n")},
		"workflows/bad.yaml":     {Data: []byte("name: bad\nintial: a\nstates: [{name: a}]\n")},
		"policies/good.yaml":     {Data: []byte("name: good\napplies: {types: ['*']}\n")},
		"policies/bad.yaml":      {Data: []byte("name: bad\napplies: {types: ['*']}\nchecks: []\n")},
		"hooks/good.yaml":        {Data: []byte("name: good\ntype: lint\nuri: x:y/z\n")},
		"hooks/bad.yaml":         {Data: []byte("name: bad\ntype: lint\nuri: x:y/z\nwehn: 'true'\n")},
		"hook-types/good.yaml":   {Data: []byte("name: good\ntiming: on-demand\nanswer: findings\n")},
		"hook-types/bad.yaml":    {Data: []byte("name: bad\ntimming: on-demand\nanswer: findings\n")},
	}
	count := func(kind string, n int, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if n != 1 {
			t.Errorf("%s: loaded %d, want 1 (the one with an unknown key is skipped)", kind, n)
		}
	}
	caps, err := LoadCapabilitiesLayered(src)
	count("capability", len(caps), err)
	reg, err := LoadTypesLayered(src)
	n := 0
	if reg != nil {
		n = len(reg.EdgeTypes())
	}
	count("edge type", n, err)
	asp, err := LoadAspectsLayered(src)
	count("aspect", len(asp), err)
	wf, err := LoadWorkflowsLayered(src)
	count("workflow", len(wf), err)
	pol, err := LoadPoliciesLayered(src)
	count("policy", len(pol), err)
	hooks, err := LoadHooksLayered(src)
	count("hook", len(hooks), err)
	hts, err := LoadHookTypesLayered(src)
	count("hook type", len(hts), err)
}

// An aspect's `pane:` is an opaque section CWM carries for the object pane: it
// decodes as written, and a key beside it that no field takes is still
// refused.
func TestAnAspectCarriesItsPaneSectionAndStillRefusesUnknownKeys(t *testing.T) {
	src := fstest.MapFS{
		"aspects/study.yaml": {Data: []byte("name: study\ncapabilities: [good]\npane:\n  tabs: [cadrage, files]\n")},
		"aspects/bad.yaml":   {Data: []byte("name: bad\ncapabilities: [good]\npanes:\n  tabs: [files]\n")},
	}
	asp, err := LoadAspectsLayered(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(asp) != 1 || asp[0].Name != "study" {
		t.Fatalf("aspects = %+v, want study alone (bad's `panes` is refused)", asp)
	}
	tabs, _ := asp[0].Pane["tabs"].([]any)
	if len(tabs) != 2 || tabs[0] != "cadrage" || tabs[1] != "files" {
		t.Errorf("study pane = %#v, want tabs [cadrage files]", asp[0].Pane)
	}
}
