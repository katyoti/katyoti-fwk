// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	stdctx "context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fileURI renders an absolute path as a file:// URI the FileFetcher reads.
func fileURI(path string) string {
	slashed := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		return "file:///" + slashed
	}
	return "file://" + slashed
}

// writeSourceFile drops the source markdown in a project and returns a context
// carrying that project's root and the file's URI.
func writeSourceFile(t testing.TB, content string) (stdctx.Context, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "spec.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	return WithProjectRoot(stdctx.Background(), root), fileURI(path)
}

// docBody is a source-document body naming the section and its recorded hash,
// plus a field the typed view does not know (kept across a reanchor).
func docBody(t testing.TB, uri string, section []string, hash, snapshot string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"source_uri": uri, "section_path": section, "content_hash": hash,
		"content_snapshot": snapshot, "owner": "arch-team",
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decoded(t testing.TB, raw json.RawMessage) (SourceDocBody, map[string]any) {
	t.Helper()
	b, err := decodeSourceDocBody(raw)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return b, m
}

var section = []string{"Auth", "Rate Limits"}

func TestReanchorMatch(t *testing.T) {
	ctx, uri := writeSourceFile(t, "# Auth\n\n## Rate Limits\n\nLimit 100 rps.\n")
	hash := hashOf(normalizeText("Limit 100 rps."))
	res, err := New().Reanchor(ctx, docBody(t, uri, section, hash, "Limit 100 rps."), "", false)
	if err != nil {
		t.Fatalf("Reanchor: %v", err)
	}
	if res.Status != "match" || res.OldHash != hash || res.NewHash != hash || res.AnchorStatus != AnchorStatusValid {
		t.Fatalf("result = %+v, want a match on %s", res, hash)
	}
	b, m := decoded(t, res.Body)
	if b.LastReanchorAt.IsZero() || m["owner"] != "arch-team" {
		t.Fatalf("body = %s: want last_reanchor_at set and the unknown field kept", res.Body)
	}
}

func TestReanchorDriftMarksStaleAndKeepsTheHash(t *testing.T) {
	ctx, uri := writeSourceFile(t, "# Auth\n\n## Rate Limits\n\nLimit 50 rps.\n")
	old := hashOf(normalizeText("Limit 100 rps."))
	res, err := New().Reanchor(ctx, docBody(t, uri, section, old, "Limit 100 rps."), "", false)
	if err != nil {
		t.Fatalf("Reanchor: %v", err)
	}
	b, _ := decoded(t, res.Body)
	if res.Status != "drift" || res.AnchorStatus != AnchorStatusStale || b.AnchorStatus != AnchorStatusStale || b.ContentHash != old {
		t.Fatalf("result = %+v, body = %s: want drift, stale, the old hash kept", res, res.Body)
	}
}

func TestReanchorUpdateOnDriftRegenerates(t *testing.T) {
	ctx, uri := writeSourceFile(t, "# Auth\n\n## Rate Limits\n\nLimit 50 rps.\n")
	old := hashOf(normalizeText("Limit 100 rps."))
	res, err := New().Reanchor(ctx, docBody(t, uri, section, old, "Limit 100 rps."), "", true)
	if err != nil {
		t.Fatalf("Reanchor: %v", err)
	}
	b, _ := decoded(t, res.Body)
	if res.Status != "drift" || res.AnchorStatus != AnchorStatusValid || b.ContentHash != res.NewHash || strings.TrimSpace(b.ContentSnapshot) != "Limit 50 rps." {
		t.Fatalf("result = %+v, body = %s: want the snapshot and hash regenerated, valid", res, res.Body)
	}
}

func TestReanchorMissingSection(t *testing.T) {
	ctx, uri := writeSourceFile(t, "# Auth\n\n## Other\n\nx\n")
	res, err := New().Reanchor(ctx, docBody(t, uri, section, "h", "x"), "", false)
	if err != nil {
		t.Fatalf("Reanchor: %v", err)
	}
	if res.Status != "missing" || res.AnchorStatus != AnchorStatusMissing || res.NewHash != "" {
		t.Fatalf("result = %+v, want missing", res)
	}
}

func TestReanchorNewContentOverride(t *testing.T) {
	hash := hashOf(normalizeText("Limit 100 rps."))
	res, err := New().Reanchor(stdctx.Background(), docBody(t, "https://unreachable.example/spec.md", section, hash, ""),
		"# Auth\n\n## Rate Limits\n\nLimit 100 rps.\n", false)
	if err != nil || res.Status != "match" {
		t.Fatalf("result = %+v (%v): the supplied content must be used, no fetch", res, err)
	}
}

func TestReanchorNeedsAFetchableSource(t *testing.T) {
	for name, uri := range map[string]string{"empty": "", "no scheme": "spec.md", "unknown scheme": "gopher://x/spec.md"} {
		if _, err := New().Reanchor(stdctx.Background(), docBody(t, uri, section, "h", ""), "", false); err == nil {
			t.Errorf("%s source_uri: want an error", name)
		}
	}
}
