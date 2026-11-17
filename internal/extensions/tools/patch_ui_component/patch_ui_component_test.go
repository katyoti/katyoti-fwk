// SPDX-License-Identifier: Apache-2.0

package patch_ui_component

import (
	"context"
	"testing"
)

func TestTranslateOp_Replace(t *testing.T) {
	w, err := translateOp(PatchOp{Op: "replace", Target: "kpi-open", Value: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w["kind"] != "replace" || w["target"] != "kpi-open" || w["value"] != 42 {
		t.Fatalf("bad translation: %+v", w)
	}
	if _, ok := w["op"]; ok {
		t.Fatalf("replace must not set 'op' (only kind=rfc6902 uses op)")
	}
}

func TestTranslateOp_Add(t *testing.T) {
	w, err := translateOp(PatchOp{Op: "add", Path: "/items/0", Value: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w["kind"] != "rfc6902" || w["op"] != "add" || w["path"] != "/items/0" || w["value"] != "x" {
		t.Fatalf("bad translation: %+v", w)
	}
}

func TestTranslateOp_Remove(t *testing.T) {
	w, err := translateOp(PatchOp{Op: "remove", Target: "stale"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w["kind"] != "remove" || w["target"] != "stale" {
		t.Fatalf("bad translation: %+v", w)
	}
	if _, ok := w["value"]; ok {
		t.Fatalf("remove must not set value")
	}
}

func TestTranslateOp_Append(t *testing.T) {
	w, err := translateOp(PatchOp{Op: "append", Target: "list", Value: map[string]any{"id": "t1"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w["kind"] != "append" || w["target"] != "list" {
		t.Fatalf("bad translation: %+v", w)
	}
}

func TestTranslateOp_Merge(t *testing.T) {
	w, err := translateOp(PatchOp{Op: "merge", Target: "user", Value: map[string]any{"email": "x@y"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w["kind"] != "rfc7396" || w["target"] != "user" {
		t.Fatalf("bad translation: %+v", w)
	}
	if _, ok := w["mergePatch"]; !ok {
		t.Fatalf("merge must set mergePatch")
	}
	if _, ok := w["value"]; ok {
		t.Fatalf("merge must not set value (uses mergePatch instead)")
	}
}

func TestTranslateOp_MergeRootNoTargetNoPath(t *testing.T) {
	// merge is valid at root with neither target nor path (artifact-wide merge).
	w, err := translateOp(PatchOp{Op: "merge", Value: map[string]any{"x": 1}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w["kind"] != "rfc7396" {
		t.Fatalf("bad kind: %+v", w)
	}
}

func TestTranslateOp_MissingAddressErrors(t *testing.T) {
	// non-merge ops require target or path.
	_, err := translateOp(PatchOp{Op: "replace", Value: 1})
	if err == nil {
		t.Fatalf("expected error for missing target/path")
	}
}

func TestTranslateOp_UnknownOpErrors(t *testing.T) {
	_, err := translateOp(PatchOp{Op: "frobnicate", Target: "x", Value: 1})
	if err == nil {
		t.Fatalf("expected error for unknown op")
	}
}

func TestTranslateOp_AddMissingValueErrors(t *testing.T) {
	_, err := translateOp(PatchOp{Op: "add", Path: "/x"})
	if err == nil {
		t.Fatalf("expected error for add missing value")
	}
}

func TestExecute_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := executePatch(ctx, PatchUIComponentInput{}); err == nil {
		t.Fatalf("expected error for empty input")
	}
	if _, err := executePatch(ctx, PatchUIComponentInput{ArtifactID: "x"}); err == nil {
		t.Fatalf("expected error for empty ops")
	}
}
