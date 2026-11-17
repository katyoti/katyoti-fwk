// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
)

// TestConnectorWriteJournalPerTenant: each tenant's
// journal is a JSON-lines file under its own data folder; an append adds one
// stamped line to it, and another tenant's journal is untouched.
func TestConnectorWriteJournalPerTenant(t *testing.T) {
	a, b := api.JournalPath(filepath.Join(t.TempDir(), "tenant-a")), api.JournalPath(filepath.Join(t.TempDir(), "tenant-b"))
	if a == b || filepath.Base(a) != api.JournalFileName || api.JournalPath("") != "" {
		t.Fatalf("paths: %q %q", a, b)
	}
	for _, op := range []string{api.WriteCreate, api.WriteRefused} {
		if err := api.AppendWrite(a, api.ConnectorWrite{Run: "run-1", Actor: "jdoe", Connector: "confluence", Op: op, Target: "500"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := api.ReadWrites(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Op != api.WriteCreate || got[1].Op != api.WriteRefused || got[0].At == "" || got[0].Run != "run-1" {
		t.Fatalf("journal: %+v", got)
	}
	if other, _ := api.ReadWrites(b); len(other) != 0 {
		t.Fatalf("another tenant's journal: %+v", other)
	}
	if err := api.AppendWrite("", api.ConnectorWrite{}); err == nil {
		t.Fatal("an append with no journal was accepted")
	}
}
