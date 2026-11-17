// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// The scenario reads an address as the GUI does: the examples both read
// (app/ktw-gui/frontend/tests/fixtures/addresses.json) open the same thing,
// and the refused ones are refused saying why.
func TestParseAddress_ReadsTheGUIsExamples(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "app", "ktw-gui", "frontend", "tests", "fixtures", "addresses.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Opens []struct {
			Raw  string            `json:"raw"`
			Want map[string]string `json:"want"`
		} `json:"opens"`
		Refused []struct {
			Raw   string `json:"raw"`
			Error string `json:"error"`
		} `json:"refused"`
	}
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	for _, c := range fx.Opens {
		a, err := ParseAddress(c.Raw)
		if err != nil {
			t.Errorf("%s: %v", c.Raw, err)
			continue
		}
		got := map[string]string{"kind": a.Kind, "universe": a.Universe}
		for k, v := range map[string]string{"id": a.ID, "tab": a.Tab, "section": a.Section, "entry": a.Entry, "screen": a.Screen} {
			if v != "" {
				got[k] = v
			}
		}
		if !reflect.DeepEqual(got, c.Want) {
			t.Errorf("%s: opens %v, want %v", c.Raw, got, c.Want)
		}
	}
	for _, c := range fx.Refused {
		if _, err := ParseAddress(c.Raw); err == nil || !strings.Contains(err.Error(), c.Error) {
			t.Errorf("%s: %v, want an error saying %q", c.Raw, err, c.Error)
		}
	}
	if len(fx.Opens) == 0 || len(fx.Refused) == 0 {
		t.Fatal("the fixture holds no example")
	}
}

// An address that names no universe (its first segment a place) is put in
// the case's; one that names its own keeps it.
func TestWithUniverse(t *testing.T) {
	if got := WithUniverse("object/m.F-001/links", "scn-todo"); got != "scn-todo/object/m.F-001/links" {
		t.Errorf("got %s", got)
	}
	if got := WithUniverse("dev/object/m.F-001/links", "scn-todo"); got != "dev/object/m.F-001/links" {
		t.Errorf("got %s", got)
	}
}

// A step `open:` names a GUI element by its address: checked at load like a
// guide target - an address the GUI opens passes (the case's universe when it
// names none), one that opens nothing is refused saying why; open is a kind
// of its own, never beside another.
func TestAnOpenStepIsCheckedAtLoad(t *testing.T) {
	load := func(steps string) error {
		dir := t.TempDir()
		body := "name: proof\nuniverse: {sidecars: [ktw-agents]}\nstages:\n  - id: s1\n    steps:\n" + steps
		if err := os.WriteFile(filepath.Join(dir, CaseFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(dir)
		return err
	}
	for _, ok := range []string{"object/m.F-001/links", "dev/screen/marketplace/sidecars", "rail/inbox"} {
		if err := load("      - open: " + ok + "\n"); err != nil {
			t.Errorf("open %s: %v", ok, err)
		}
	}
	for addr, why := range map[string]string{"object": "names the object", "screen/elsewhere": "no screen", "dev/nowhere/x": "no place"} {
		if err := load("      - open: " + addr + "\n"); err == nil || !strings.Contains(err.Error(), why) {
			t.Errorf("open %s: %v, want refused with %q", addr, err, why)
		}
	}
	if err := load("      - open: rail/inbox\n        run: {cmd: echo}\n"); err == nil || !strings.Contains(err.Error(), "exactly one of") {
		t.Errorf("open beside run: %v", err)
	}
}
