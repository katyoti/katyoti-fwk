// SPDX-License-Identifier: Apache-2.0

package core

import "testing"

// TestLocksAreOneModuleEach: a writer runs alone on its module, readers share,
// and a module's lock is that module's only — a parent's writer and its
// sub-module's writer run together; a step changing the sub-module too lists
// it in its writes. portfolio sits inside planification.
func TestLocksAreOneModuleEach(t *testing.T) {
	r := func(m ...string) []string { return m }
	cases := []struct {
		name         string
		heldR, heldW []string
		askR, askW   []string
		conflict     bool
	}{
		{"parent writer held, child writer asked", nil, r("planification"), nil, r("portfolio"), false},
		{"child writer held, parent writer asked", nil, r("portfolio"), nil, r("planification"), false},
		{"parent writer held, child reader asked", nil, r("planification"), r("portfolio"), nil, false},
		{"parent reader held, child writer asked", r("planification"), nil, nil, r("portfolio"), false},
		{"a writer listing the child held, child writer asked", nil, r("planification", "portfolio"), nil, r("portfolio"), true},
		{"a writer listing the child held, child reader asked", nil, r("planification", "portfolio"), r("portfolio"), nil, true},
		{"a reader listing the child held, child writer asked", r("planification", "portfolio"), nil, nil, r("portfolio"), true},
		{"same module, two readers", r("portfolio"), nil, r("portfolio"), nil, false},
		{"same module, reader held, writer asked", r("portfolio"), nil, nil, r("portfolio"), true},
		{"same module, writer held, reader asked", nil, r("portfolio"), r("portfolio"), nil, true},
		{"same module, two writers", nil, r("portfolio"), nil, r("portfolio"), true},
		{"unrelated modules, two writers", nil, r("portfolio"), nil, r("scheduler"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := &locks{}
			l.take(c.heldR, c.heldW)
			why := l.conflict(c.askR, c.askW)
			if (why != "") != c.conflict {
				t.Fatalf("conflict = %q, want conflict %v", why, c.conflict)
			}
		})
	}
}
