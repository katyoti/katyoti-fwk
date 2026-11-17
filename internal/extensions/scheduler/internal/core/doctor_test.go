// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	stdreflect "reflect"
	"strings"
	"testing"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/sqlstore"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tui/render"
)

// TestTheDoctorNamesEachFaultAndFailsTheCommand: todos doctor on a sound store
// answers ok with no fault - the command exits 0; on a store whose pages are
// overwritten it names each fault with its repair (move aside, todos import),
// and its failed count makes the generated command exit non-zero (the output's
// ui-cli failure-count-field, read through the real struct tag).
func TestTheDoctorNamesEachFaultAndFailsTheCommand(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := OpenSQLiteStore(schedulerEnv{dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	ast, err := corereflect.TypeASTForType(stdreflect.TypeOf(api.DoctorOutput{}))
	if err != nil {
		t.Fatal(err)
	}
	failed := func(out api.DoctorOutput) int {
		body, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		n, ok := render.FailureCount(ast, body)
		if !ok {
			t.Fatalf("the doctor's output declares no failure count: its command cannot exit non-zero")
		}
		return n
	}

	sound, err := doctorOf(ctx, dir)
	if err != nil || !sound.Found || !sound.OK || len(sound.Faults) != 0 || failed(sound) != 0 {
		t.Fatalf("doctor of a sound store: %+v (%v), want ok, exit 0", sound, err)
	}

	path := filepath.Join(dir, sqlstore.File)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Every page after the first (the header and the schema) overwritten.
	copy(raw[4096:], bytes.Repeat([]byte{0xa5}, len(raw)-4096))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	bad, err := doctorOf(ctx, dir)
	if err != nil || !bad.Found || bad.OK || len(bad.Faults) == 0 || bad.Failed != len(bad.Faults) || failed(bad) == 0 {
		t.Fatalf("doctor of a damaged store: %+v (%v), want its faults named and a non-zero exit", bad, err)
	}
	for _, f := range bad.Faults {
		if f.Fault == "" || !strings.Contains(f.Repair, "aside") || !strings.Contains(f.Repair, "todos import") {
			t.Fatalf("fault %+v: want the fault named with its repair", f)
		}
	}
	if bad.Store != path {
		t.Fatalf("store checked = %s, want %s", bad.Store, path)
	}
}
