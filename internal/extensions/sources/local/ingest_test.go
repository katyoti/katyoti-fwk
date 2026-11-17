// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// INGEST, ON A REAL CWM: the service reads the section, hashes and snapshots it
// with the computation Reanchor uses, and writes the source-document through
// the Object API; a file:// source is read only inside the project.

func TestIngestCreatesTheDocumentAndReanchorMatches(t *testing.T) {
	ctx, cwm, root := project(t)
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "spec.md"), []byte("# Auth\n\n## Rate Limits\n\nLimit 100 rps.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, _ := application.UseService[api.SourcesService](ctx)
	in := api.IngestInput{SourceDocID: "m.spec", SourceURI: "file://./docs/spec.md", SectionPath: []string{"Auth", "Rate Limits"}, IngestedBy: "tester"}
	out, err := svc.Ingest(ctx, in)
	if err != nil || !out.Created || !strings.HasPrefix(out.ContentHash, "sha256:") {
		t.Fatalf("Ingest = %+v (%v), want a created document with a hash", out, err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.spec"})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	_ = json.Unmarshal(got.Object.FieldsJSON(), &fields)
	if got.Object.Type != api.SourceDocumentType || fields["content_hash"] != out.ContentHash || fields["anchor_status"] != "valid" || fields["source_uri"] != "file://./docs/spec.md" {
		t.Fatalf("stored %s with fields %s", got.Object.Type, got.Object.FieldsJSON())
	}
	re, err := svc.Reanchor(ctx, api.ReanchorInput{SourceDocID: "m.spec"})
	if err != nil || re.Status != "match" || re.NewHash != out.ContentHash {
		t.Fatalf("Reanchor after Ingest = %+v (%v), want match on %s", re, err, out.ContentHash)
	}

	// Ingesting again anchors the existing document afresh.
	if err := os.WriteFile(filepath.Join(root, "docs", "spec.md"), []byte("# Auth\n\n## Rate Limits\n\nLimit 50 rps.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Ingest(ctx, in)
	if err != nil || again.Created || again.ContentHash == out.ContentHash {
		t.Fatalf("re-Ingest = %+v (%v), want the existing document updated with a new hash", again, err)
	}
}

func TestIngestRefusesAFileOutsideTheProject(t *testing.T) {
	ctx, cwm, _ := project(t)
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("# Secret\n\nkey\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, _ := application.UseService[api.SourcesService](ctx)
	for _, uri := range []string{fileURI(outside), "file://./../" + filepath.Base(filepath.Dir(outside)) + "/secret.md"} {
		_, err := svc.Ingest(ctx, api.IngestInput{SourceDocID: "m.secret", SourceURI: uri})
		if !errors.Is(err, cwmapi.ErrInvalidInput) || !strings.Contains(err.Error(), "outside the project") {
			t.Errorf("Ingest(%s) = %v, want refused as outside the project", uri, err)
		}
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.secret"}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Fatalf("a refused ingest left an object: %v", err)
	}
}

func TestIngestRefusesAnUnqualifiedIDOrAnotherType(t *testing.T) {
	ctx, _, _ := fixture(t, "# Auth\n")
	svc, _ := application.UseService[api.SourcesService](ctx)
	for id, want := range map[string]error{"spec": cwmapi.ErrInvalidInput, "m.other": cwmapi.ErrInvalidInput} {
		if _, err := svc.Ingest(ctx, api.IngestInput{SourceDocID: id, SourceURI: "file://./x.md", NewContent: "x"}); !errors.Is(err, want) {
			t.Errorf("Ingest %s = %v, want %v", id, err, want)
		}
	}
}

// STALE LISTS EXACTLY the module's source-documents left stale or missing,
// with the objects derived from each.
func TestStaleListsExactlyTheStaleAndMissingAnchors(t *testing.T) {
	ctx, cwm, _ := project(t)
	create := func(local, typ, status string, edgeTo string) {
		t.Helper()
		o := cwmapi.NewContainerObject("m", local, typ, "m", local, "")
		if typ == api.SourceDocumentType {
			f := map[string]any{"source_uri": "file://./" + local + ".md", "content_hash": "sha256:x"}
			if status != "" {
				f["anchor_status"] = status
			}
			raw, _ := json.Marshal(f)
			o.SetFieldsJSON(raw)
		}
		if edgeTo != "" {
			o.Edges = []cwmapi.ObjectEdge{{Kind: api.DerivedFromEdge, To: edgeTo}}
		}
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatalf("create %s: %v", local, err)
		}
	}
	create("ok", api.SourceDocumentType, "valid", "")
	create("fresh", api.SourceDocumentType, "", "")
	create("drifted", api.SourceDocumentType, "stale", "")
	create("gone", api.SourceDocumentType, "missing", "")
	create("rule", "decision", "", "m.drifted")
	create("rule2", "decision", "", "m.drifted")

	svc, _ := application.UseService[api.SourcesService](ctx)
	out, err := svc.Stale(ctx, api.StaleInput{Module: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range out.Anchors {
		got = append(got, a.SourceDocID+"="+a.AnchorStatus+"<"+strings.Join(a.DerivedItemIDs, ","))
	}
	if want := "m.drifted=stale<m.rule,m.rule2 m.gone=missing<"; strings.Join(got, " ") != want {
		t.Fatalf("Stale = %q, want %q", strings.Join(got, " "), want)
	}
	if _, err := svc.Stale(ctx, api.StaleInput{}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("Stale without a module = %v, want ErrInvalidInput", err)
	}
}

// THE SIZE BUDGET: source-document declares max_size_bytes, and CWM refuses a
// file over it. A section of multi-byte text whose 3,500-character snapshot is
// three times the budget is still ingested: the snapshot is cut, on a
// character boundary, until the whole document fits; the hash stays the whole
// section's, so the first reanchor matches. A drift re-snapshotted fits too.
func TestIngestCutsTheSnapshotToTheTypesBudget(t *testing.T) {
	ctx, cwm, root := project(t)
	line := strings.Repeat("漢字é", 20) + "\n" // 60 characters, 140 bytes
	// No blank line under the heading: a string opening on a newline loses it
	// in CWM's YAML (yaml.v3), which would blur the character count below.
	section := "# Auth\n\n## Rate Limits\n" + strings.Repeat(line, 60)
	if err := os.WriteFile(filepath.Join(root, "big.md"), []byte(section), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, _ := application.UseService[api.SourcesService](ctx)
	out, err := svc.Ingest(ctx, api.IngestInput{SourceDocID: "m.big", SourceURI: "file://./big.md", SectionPath: []string{"Auth", "Rate Limits"}})
	if err != nil {
		t.Fatalf("Ingest of a large multi-byte section: %v", err)
	}
	snapshot := storedSnapshot(t, ctx, cwm, "m.big", 4096)
	if !utf8.ValidString(snapshot) || snapshot == "" || !strings.HasPrefix(strings.Repeat(line, 60), snapshot) {
		t.Fatalf("the snapshot is not a cut of the section on a character boundary (%d bytes)", len(snapshot))
	}
	if n := utf8.RuneCountInString(snapshot); n != out.SnapshotChars || n >= 3500 {
		t.Errorf("snapshot %d characters, Ingest says %d — want the cut length, under 3,500", n, out.SnapshotChars)
	}
	if re, err := svc.Reanchor(ctx, api.ReanchorInput{SourceDocID: "m.big"}); err != nil || re.Status != "match" {
		t.Fatalf("Reanchor after a cut snapshot = %+v (%v), want match: the hash is the whole section's", re, err)
	}

	drifted := "# Auth\n\n## Rate Limits\n" + strings.Repeat("ü"+line, 60)
	if _, err := svc.Reanchor(ctx, api.ReanchorInput{SourceDocID: "m.big", NewContent: drifted, UpdateOnDrift: true}); err != nil {
		t.Fatalf("Reanchor re-snapshotting a large drift: %v", err)
	}
	storedSnapshot(t, ctx, cwm, "m.big", 4096)
}

// storedSnapshot reads the document's file size and snapshot, failing when the
// file is over max bytes.
func storedSnapshot(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string, max int) string {
	t.Helper()
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: id})
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(res.Path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() > int64(max) {
		t.Fatalf("%s is %d bytes, over the type's %d", id, st.Size(), max)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Errors) > 0 {
		t.Fatalf("%s is stored with errors: %+v", id, got.Errors)
	}
	var fields struct {
		Snapshot string `json:"content_snapshot"`
	}
	_ = json.Unmarshal(got.Object.FieldsJSON(), &fields)
	return fields.Snapshot
}
