// SPDX-License-Identifier: Apache-2.0

package contribution

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"gopkg.in/yaml.v3"
)

// THE DATA-TYPE LEDGER. A kind's format is its data-type's major.minor, and a
// host reads a member by comparing the two (contribapi.JudgeFormat): within a
// major the format only GROWS. That holds only if a field set never changes
// without its version — so the ledger pins every shipped system data-type's
// field set per version:
//
//   - the fields of a version already in the ledger never change: a change
//     needs a new version;
//   - a new version is recorded with KTW_LEDGER_UPDATE=1, and refused when it
//     REMOVES a field the previous version had without bumping the major
//     (removing breaks an older host's strict read; adding is a minor).
//
// The ledger is append-only history, kept in testdata/data-type-ledger.json.

const ledgerFile = "testdata/data-type-ledger.json"

// ledger: data-type name → version → its field paths, sorted.
type ledger map[string]map[string][]string

// fieldPaths is every field a data-type DSL declares, as Type.field[.sub…].
func fieldPaths(dsl []byte) ([]string, string, error) {
	var doc struct {
		Version string         `yaml:"version"`
		Types   map[string]any `yaml:"types"`
	}
	if err := yaml.Unmarshal(dsl, &doc); err != nil {
		return nil, "", err
	}
	var out []string
	var walk func(prefix string, def any)
	walk = func(prefix string, def any) {
		m, ok := def.(map[string]any)
		if !ok {
			return
		}
		if fields, ok := m["fields"].(map[string]any); ok {
			for name, sub := range fields {
				p := prefix + "." + name
				out = append(out, p)
				walk(p, sub)
			}
		}
		for _, k := range []string{"items", "catchall"} {
			walk(prefix+"[]", m[k])
		}
	}
	for name, def := range doc.Types {
		walk(name, def)
	}
	sort.Strings(out)
	return out, doc.Version, nil
}

func readLedger(t *testing.T) ledger {
	t.Helper()
	l := ledger{}
	b, err := os.ReadFile(ledgerFile)
	if os.IsNotExist(err) {
		return l
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &l); err != nil {
		t.Fatalf("%s: %v", ledgerFile, err)
	}
	return l
}

// latestBefore is the highest version of name in the ledger below v.
func (l ledger) latestBefore(name string, v contribapi.Format) (string, bool) {
	best, found := "", false
	var bf contribapi.Format
	for ver := range l[name] {
		f, ok := contribapi.ParseFormat(ver)
		if !ok || !(f.Major < v.Major || (f.Major == v.Major && f.Minor < v.Minor)) {
			continue
		}
		if !found || f.Major > bf.Major || (f.Major == bf.Major && f.Minor > bf.Minor) {
			best, bf, found = ver, f, true
		}
	}
	return best, found
}

// ledgerVerdict judges one data-type's current field set against the ledger:
// "" when it is recorded as is, else why not — and whether recording it is
// allowed (a new version that removes nothing without a major).
func ledgerVerdict(l ledger, name, version string, fields []string) (problem string, recordable bool) {
	if have, ok := l[name][version]; ok {
		if strings.Join(have, "\n") == strings.Join(fields, "\n") {
			return "", false
		}
		return fmt.Sprintf("%s %s: its fields changed without a version bump (added %v, removed %v) — bump the data-type's version (a minor to add, a major to remove)",
			name, version, minus(fields, have), minus(have, fields)), false
	}
	v, ok := contribapi.ParseFormat(version)
	if !ok {
		return fmt.Sprintf("%s: version %q is not M.m.p", name, version), false
	}
	if prev, found := l.latestBefore(name, v); found {
		pf, _ := contribapi.ParseFormat(prev)
		if removed := minus(l[name][prev], fields); len(removed) > 0 && pf.Major == v.Major {
			return fmt.Sprintf("%s %s removes %v that %s had without a major bump — a removal breaks an older host's strict read", name, version, removed, prev), false
		}
	}
	return fmt.Sprintf("%s %s is not in the ledger — record it: KTW_LEDGER_UPDATE=1 go test ./internal/core/contribution/ -run TestDataTypeLedger", name, version), true
}

func minus(a, b []string) []string {
	in := map[string]bool{}
	for _, x := range b {
		in[x] = true
	}
	var out []string
	for _, x := range a {
		if !in[x] {
			out = append(out, x)
		}
	}
	return out
}

func TestDataTypeLedger(t *testing.T) {
	l := readLedger(t)
	update := os.Getenv("KTW_LEDGER_UPDATE") != ""
	entries, err := fs.ReadDir(systemFS, "embed/data-type")
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		src, err := fs.ReadFile(systemFS, "embed/data-type/"+e.Name())
		if err != nil {
			t.Fatal(err)
		}
		fields, version, err := fieldPaths(src)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		problem, recordable := ledgerVerdict(l, name, version, fields)
		switch {
		case problem == "":
		case recordable && update:
			if l[name] == nil {
				l[name] = map[string][]string{}
			}
			l[name][version] = fields
			changed = true
		default:
			t.Error(problem)
		}
	}
	if changed {
		b, _ := json.MarshalIndent(l, "", "  ")
		if err := os.MkdirAll(filepath.Dir(ledgerFile), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ledgerFile, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("ledger updated: %s", ledgerFile)
	}
}

// The ledger's rules, falsified on a toy history.
func TestDataTypeLedgerRules(t *testing.T) {
	l := ledger{"x": {"1.0.0": {"X.a", "X.b"}}}
	for _, row := range []struct {
		name, version string
		fields        []string
		problem       bool
		recordable    bool
	}{
		{"recorded as is", "1.0.0", []string{"X.a", "X.b"}, false, false},
		{"fields changed, same version", "1.0.0", []string{"X.a", "X.b", "X.c"}, true, false},
		{"a minor adding a field", "1.1.0", []string{"X.a", "X.b", "X.c"}, true, true},
		{"a minor removing a field", "1.1.0", []string{"X.a"}, true, false},
		{"a major removing a field", "2.0.0", []string{"X.a"}, true, true},
	} {
		problem, recordable := ledgerVerdict(l, "x", row.version, row.fields)
		if (problem != "") != row.problem || recordable != row.recordable {
			t.Errorf("%s: problem=%q recordable=%v, want problem=%v recordable=%v", row.name, problem, recordable, row.problem, row.recordable)
		}
	}
}
