// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// DELIVERY'S « OPEN INCREMENTS » LOADS ON THE SELECTION IT OPENS ON
// : the display library holds a loaded section back
// while a selection key its args name (`${selection.<key>}`) is unset or
// empty (display-lib selectionReady). The tab opens on no search, so its
// section's args name no key that is empty then - nor once the person clears
// the search -, and read against that selection they ask the table's page,
// which lists the open increments; a search typed still narrows it.
func TestOpenIncrementsLoadsOnItsOpeningSelection(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	sm, inc := streams(t, ctx), increments(t, ctx)
	for _, title := range []string{"Invoices by mail", "Reminders"} {
		if _, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, By: person}); err != nil {
			t.Fatal(err)
		}
	}
	def, err := sm.DeliveryPage(ctx, api.DeliveryPageInput{Target: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	var data struct {
		Selection map[string]any `json:"selection"`
	}
	raw, _ := json.Marshal(def.Data)
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	selection := data.Selection
	tree, _ := json.Marshal(def.DSLTree)
	load := openLoad(t, tree)
	if load.Method != "ktw-agents:increments/open-page" {
		t.Fatalf("the Open increments section loads %q", load.Method)
	}

	ready := func(sel map[string]any) []string {
		var waits []string
		for _, k := range selectionRefs(load.Args) {
			if v, ok := sel[k]; !ok || v == nil || v == "" {
				waits = append(waits, k)
			}
		}
		return waits
	}
	if w := ready(selection); len(w) > 0 {
		t.Fatalf("on the selection the tab opens on (%v) its section waits on %v: the table never loads", selection, w)
	}
	rows := func(sel map[string]any) string {
		t.Helper()
		var in api.OpenIncrementsInput
		b, _ := json.Marshal(resolveSelection(load.Args, sel))
		if err := json.Unmarshal(b, &in); err != nil {
			t.Fatalf("the section's args %s do not read as the table's input: %v", b, err)
		}
		page, err := inc.OpenPage(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		out, _ := json.Marshal(page.DSLTree)
		return string(out)
	}
	if got := rows(selection); !strings.Contains(got, "Invoices by mail") || !strings.Contains(got, "Reminders") {
		t.Errorf("the table on its opening selection lists %s; want both open increments", got)
	}
	// Typed, then cleared: the search narrows the table, and clearing it
	// never holds the section back.
	typed := map[string]any{}
	for k, v := range selection {
		typed[k] = v
	}
	typed["open_search"] = "invoice"
	if got := rows(typed); !strings.Contains(got, "Invoices by mail") || strings.Contains(got, "Reminders") {
		t.Errorf("a search typed keeps only its line: %s", got)
	}
	typed["open_search"] = ""
	if w := ready(typed); len(w) > 0 {
		t.Errorf("the search cleared, the section waits on %v", w)
	}
}

type sectionLoad struct {
	Method string         `json:"method"`
	Args   map[string]any `json:"args"`
}

// openLoad is the load of the tab's « open-increments » section.
func openLoad(t *testing.T, tree []byte) sectionLoad {
	t.Helper()
	var root any
	if err := json.Unmarshal(tree, &root); err != nil {
		t.Fatal(err)
	}
	var find func(n any) map[string]any
	find = func(n any) map[string]any {
		switch v := n.(type) {
		case map[string]any:
			if v["id"] == "open-increments" && v["load"] != nil {
				return v
			}
			for _, c := range v {
				if f := find(c); f != nil {
					return f
				}
			}
		case []any:
			for _, c := range v {
				if f := find(c); f != nil {
					return f
				}
			}
		}
		return nil
	}
	sec := find(root)
	if sec == nil {
		t.Fatal("no open-increments section")
	}
	var l sectionLoad
	b, _ := json.Marshal(sec["load"])
	_ = json.Unmarshal(b, &l)
	return l
}

// selectionRef is display-lib's SELECTION_REF (section-load.tsx).
var selectionRef = regexp.MustCompile(`\$\{\s*selection\.([^.}\s]+)[^}]*\}`)

// selectionRefs are the selection keys the args name, as display-lib's
// selectionKeys reads them.
func selectionRefs(args map[string]any) []string {
	var keys []string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case string:
			for _, m := range selectionRef.FindAllStringSubmatch(x, -1) {
				keys = append(keys, m[1])
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		case map[string]any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(args)
	return keys
}

// resolveSelection fills whole `${selection}` / `${selection.<key>}` args
// against the selection, as display-lib's resolveActionArgs does.
func resolveSelection(args map[string]any, sel map[string]any) map[string]any {
	whole := regexp.MustCompile(`^\$\{\s*selection(?:\.([^}\s]+))?\s*\}$`)
	out := map[string]any{}
	for k, v := range args {
		s, ok := v.(string)
		m := whole.FindStringSubmatch(s)
		switch {
		case !ok || m == nil:
			out[k] = v
		case m[1] == "":
			out[k] = sel
		default:
			if got, ok := sel[m[1]]; ok {
				out[k] = got
			} else {
				out[k] = ""
			}
		}
	}
	return out
}
