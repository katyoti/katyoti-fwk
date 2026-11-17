// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// NO AUTHORED EDGE STOPS VALIDATING (7e): every edge in the real corpus, checked
// in ERROR mode against the real packs' declarations — object types,
// capabilities and edge kinds exactly as a running binary assembles them.
// Fixtures prove the rule; only the corpus proves the rule breaks nothing.
func TestRealCorpusEdgesValidate(t *testing.T) {
	long.Test(t)
	t.Parallel() // waits mostly on the disk, beside the other corpus tests
	up := filepath.Join("..", "..", "..", "..", "..")
	if _, err := os.Stat(filepath.Join(up, api.ProjectFileName)); err != nil {
		t.Skipf("no project object at the repo root: %v", err)
	}
	packs, err := filepath.Glob(filepath.Join(up, "app", "*", "embed", "cwm"))
	// An extension ships its own pack too (milestones, gates, …), wherever its
	// host binary is.
	extPacks, _ := filepath.Glob(filepath.Join(up, "internal", "extensions", "*", "contrib", "embed", "cwm"))
	packs = append(packs, extPacks...)
	if err != nil || len(packs) < 4 {
		t.Fatalf("found only %d type packs (%v) — the glob is wrong, not the corpus", len(packs), err)
	}
	// NO BOOTSTRAP: this reads the REAL repository, and the bootstrap WRITES
	// (the system inbox, 2026-09-26) — a read-only corpus test must not.
	cfg := &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: up, ObjectTypeValidation: api.TypeValidationError}
	seenType := map[string]bool{}
	seenCap := map[string]bool{}
	seenEdge := map[api.EdgeType]bool{}
	for _, dir := range packs {
		fsys := os.DirFS(dir)
		reg, err := storage.LoadTypesLayered(fsys)
		if err != nil {
			t.Fatalf("load types %s: %v", dir, err)
		}
		ots, err := storage.LoadObjectTypesLayered(fsys)
		if err != nil {
			t.Fatalf("load object types %s: %v", dir, err)
		}
		for _, ot := range ots {
			if !seenType[ot.Name] {
				seenType[ot.Name] = true
				cfg.CatalogTypes.Objects = append(cfg.CatalogTypes.Objects, ot)
			}
		}
		caps, err := storage.LoadCapabilitiesLayered(fsys)
		if err != nil {
			t.Fatalf("load capabilities %s: %v", dir, err)
		}
		for _, c := range caps {
			if !seenCap[c.Name] {
				seenCap[c.Name] = true
				cfg.CatalogTypes.Capabilities = append(cfg.CatalogTypes.Capabilities, c)
			}
		}
		for name, e := range reg.EdgeTypes() {
			if !seenEdge[name] {
				seenEdge[name] = true
				cfg.CatalogTypes.Edges = append(cfg.CatalogTypes.Edges, e)
			}
		}
	}
	t.Logf("assembled: %d object types, %d capabilities, %d edge kinds", len(cfg.CatalogTypes.Objects), len(cfg.CatalogTypes.Capabilities), len(cfg.CatalogTypes.Edges))
	svc, err := New(stdctx.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.ObjectScan(stdctx.Background(), api.ObjectScanInput{Force: true}); err != nil {
		t.Fatal(err)
	}
	all := svc.index().list(api.ObjectListInput{})
	edges, refused := 0, 0
	for _, o := range all {
		if len(o.Edges) == 0 {
			continue
		}
		edges += len(o.Edges)
		if err := svc.checkObjectEdges(o, api.TypeValidationError, "corpus"); err != nil {
			refused++
			t.Errorf("%v", err)
		}
	}
	if edges < 50 {
		t.Fatalf("only %d authored edges seen — a vacuous pass is worse than a failure", edges)
	}
	t.Logf("corpus: %d objects, %d authored edges checked in error mode, %d refused", len(all), edges, refused)
}
