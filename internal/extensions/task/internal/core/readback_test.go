// SPDX-License-Identifier: Apache-2.0

package core

import (
	"reflect"
	"testing"
	"time"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine/enginetest"
)

// enginetest.ReadBack — what every machine's fidelity test reads its state
// back with — reads it back exactly as the run store does across a restart.
func TestTheRunStoreReadsBackAsMachinesTestIt(t *testing.T) {
	store, err := newRunStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.close) // its owner lock open would keep the folder
	defer store.close()
	st := engine.State{Name: "awaiting", RunID: "r-1", Version: 7, Data: map[string]any{
		"turn": 3, "big": int64(9007199254740993), "when": time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC),
		"list": []string{"a", "b"}, "nested": map[string]any{"ok": true, "n": 1.5},
	}}
	if err := store.Save("r-1", engine.Checkpoint{State: st, Wait: &engine.WaitSpec{Mode: engine.WaitAny, Timeout: time.Hour}}); err != nil {
		t.Fatal(err)
	}
	cp, err := store.Load("r-1")
	if err != nil {
		t.Fatal(err)
	}
	if want := enginetest.ReadBack(t, st); !reflect.DeepEqual(cp.State, *want) {
		t.Errorf("run store read back %#v\nenginetest read back %#v", cp.State, *want)
	}
}
