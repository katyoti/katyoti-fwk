// SPDX-License-Identifier: Apache-2.0

package records

import (
	"encoding/json"
	"testing"

	crudapi "github.com/katyoti/katyoti-fwk/internal/extensions/crud/api"
)

// TestParseConnURI pins the external record-URI grammar:
// <connection>:/<relative-uri> (cwm-v3 §2.4 round-trip rule).
func TestParseConnURI(t *testing.T) {
	cases := []struct {
		in       string
		conn     string
		relative string
		wantErr  bool
	}{
		{"memory:/book/123", "memory", "book/123", false},
		{"memory:/book", "memory", "book", false},
		{"sql:/order/X/line/Y", "sql", "order/X/line/Y", false},
		{"sql:/order/X/line?status=open", "sql", "order/X/line?status=open", false},
		// Relative form without a connection prefix is rejected — the
		// tool surface always addresses through a named connection.
		{"book/123", "", "", true},
		{":/book", "", "", true},
		{"memory:/", "", "", true},
		{"", "", "", true},
	}
	for _, c := range cases {
		conn, rel, err := parseConnURI(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseConnURI(%q): expected error, got (%q, %q)", c.in, conn, rel)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseConnURI(%q): %v", c.in, err)
			continue
		}
		if conn != c.conn || rel != c.relative {
			t.Errorf("parseConnURI(%q) = (%q, %q), want (%q, %q)", c.in, conn, rel, c.conn, c.relative)
		}
	}
}

func TestFlattenRecord(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"title": "Dune", "rating": 5})
	rec := crudapi.Record{Type: "book", ID: "123", Body: body}

	value := flattenRecord(rec)
	if value["id"] != "123" {
		t.Errorf("id not injected: got %v", value["id"])
	}
	if value["title"] != "Dune" {
		t.Errorf("title lost: got %v", value["title"])
	}
}

func TestFlattenRecordEmptyBody(t *testing.T) {
	rec := crudapi.Record{Type: "book", ID: "empty-1"}
	value := flattenRecord(rec)
	if value["id"] != "empty-1" {
		t.Errorf("id not injected on empty body: got %v", value["id"])
	}
}

func TestFlattenRecordNonObjectBody(t *testing.T) {
	rec := crudapi.Record{Type: "note", ID: "n1", Body: json.RawMessage(`"just a string"`)}
	value := flattenRecord(rec)
	if value["_value"] != "just a string" {
		t.Errorf("non-object body not preserved under _value: got %v", value["_value"])
	}
	if value["id"] != "n1" {
		t.Errorf("id not injected: got %v", value["id"])
	}
}
