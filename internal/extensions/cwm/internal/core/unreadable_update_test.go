// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	stdctx "context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A WRITE TO AN UNREADABLE OBJECT IS REFUSED. A hand
// edit that no longer parses — or a second file carrying the id — blocks every
// update of that id, in-process, with a revision or forced, naming the file;
// the file stays byte-identical; once it parses again the update passes. A
// parseable object with validation problems stays writable.

func unreadableFixture(t *testing.T) (*Service, stdctx.Context, string) {
	t.Helper()
	svc, ctx := treeFixture(t)
	path, err := svc.resolveObjectPath("m", "m.a")
	if err != nil {
		t.Fatal(err)
	}
	return svc, ctx, path
}

// outsideUpdate builds an update as a surface from outside hands it (decoded
// from JSON, so marked outside).
func outsideUpdate(t *testing.T, obj api.Object, revision string, force bool) api.ObjectUpdateInput {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"module": "m", "object": obj, "revision": revision, "force": force})
	if err != nil {
		t.Fatal(err)
	}
	var in api.ObjectUpdateInput
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestAWriteToAnUnparseableObjectIsRefusedUntilItParses(t *testing.T) {
	svc, ctx, path := unreadableFixture(t)
	good, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	broken := append(append([]byte(nil), good...), []byte("\nname: [unclosed\n")...)
	if err := os.WriteFile(path, broken, 0o644); err != nil {
		t.Fatal(err)
	}
	rev, err := fileRevision(path)
	if err != nil {
		t.Fatal(err)
	}
	obj := api.Object{ID: "m.a", Type: "note", Name: "a renamed", Parent: "m"}
	writes := map[string]api.ObjectUpdateInput{
		"in-process":      {Module: "m", Object: obj},
		"with a revision": outsideUpdate(t, obj, rev, false),
		"forced":          outsideUpdate(t, obj, "", true),
	}
	check := func(when string) {
		for name, in := range writes {
			_, err := svc.ObjectUpdate(ctx, in)
			if !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), filepath.Base(path)) || !strings.Contains(err.Error(), "does not parse") {
				t.Errorf("%s, %s: %v - want refused naming %s and its parse error", when, name, err, filepath.Base(path))
			}
			now, _ := os.ReadFile(path)
			if !bytes.Equal(now, broken) {
				t.Fatalf("%s, %s: the hand edit was overwritten", when, name)
			}
		}
	}
	check("indexed")
	// A scan drops the unparseable file from the index: still refused, still named.
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	check("after a scan")

	// Fixed by hand: the update passes.
	if err := os.WriteFile(path, good, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: obj}); err != nil {
		t.Fatalf("after the fix: %v - want written", err)
	}
	got, err := svc.ObjectGet(ctx, api.ObjectGetInput{Module: "m", ID: "m.a"})
	if err != nil || got.Object.Name != "a renamed" {
		t.Fatalf("after the fix: %v %q", err, got.Object.Name)
	}
}

func TestAWriteToADuplicatedIDIsRefused(t *testing.T) {
	svc, ctx, path := unreadableFixture(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dup := filepath.Join(filepath.Dir(path), "zz-copy"+ObjectFileSuffix)
	if err := os.WriteFile(dup, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectScan(ctx, api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	obj := api.Object{ID: "m.a", Type: "note", Name: "a renamed", Parent: "m"}
	_, err = svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: obj})
	if !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), api.DiagnosticDuplicateID) {
		t.Fatalf("duplicate id: %v - want refused", err)
	}
	// The second file removed: the update passes, even before a scan.
	if err := os.Remove(dup); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: obj}); err != nil {
		t.Fatalf("after removing the copy: %v - want written", err)
	}
}

func TestAParseableObjectWithProblemsStaysWritable(t *testing.T) {
	svc, ctx, path := unreadableFixture(t)
	// A parseable file with an unknown field: stored with a problem, not unreadable.
	raw, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(raw, []byte("\nedges:\n  - kind: serves\n    to: m.nowhere\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadObjectFile(path); err != nil {
		t.Skipf("fixture no longer parses: %v", err)
	}
	obj := api.Object{ID: "m.a", Type: "note", Name: "a renamed", Parent: "m"}
	if _, err := svc.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: "m", Object: obj}); err != nil {
		t.Fatalf("a parseable object: %v - want written", err)
	}
}
