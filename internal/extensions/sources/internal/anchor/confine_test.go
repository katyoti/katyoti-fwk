// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	api "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE FILE FETCHER READS ONLY INSIDE THE PROJECT. A source_uri is an object
// field anyone can write; resolved as any path the process can reach, it would
// read any file of the machine into a snapshot.

// project lays out <tmp>/project/docs/spec.md and <tmp>/secret.md beside it.
func project(t *testing.T) (root, outside string) {
	t.Helper()
	base := t.TempDir()
	root = filepath.Join(base, "project")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "spec.md"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside = filepath.Join(base, "secret.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, outside
}

func TestFileFetcherReadsRelativeToTheProject(t *testing.T) {
	root, _ := project(t)
	ctx := WithProjectRoot(stdctx.Background(), root)
	for _, uri := range []string{"file://./docs/spec.md", "file://docs/spec.md", fileURI(filepath.Join(root, "docs", "spec.md"))} {
		got, err := (FileFetcher{}).Fetch(ctx, uri)
		if err != nil || string(got) != "inside" {
			t.Errorf("Fetch(%s) = %q, %v; want the project's file", uri, got, err)
		}
	}
}

func TestFileFetcherRefusesOutsideTheProject(t *testing.T) {
	root, outside := project(t)
	ctx := WithProjectRoot(stdctx.Background(), root)
	cases := map[string]string{
		"absolute elsewhere": fileURI(outside),
		"dot-dot escape":     "file://./../secret.md",
		"bare dot-dot":       "file://../secret.md",
		"deep escape":        "file://./docs/../../secret.md",
	}
	link := filepath.Join(root, "docs", "link.md")
	if err := os.Symlink(outside, link); err == nil {
		cases["symlink escape"] = "file://./docs/link.md"
	} else {
		t.Logf("no symlink on this system (%v): the symlink case is not run", err)
	}
	for name, uri := range cases {
		got, err := (FileFetcher{}).Fetch(ctx, uri)
		if !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "outside the project") {
			t.Errorf("%s: Fetch(%s) = %q, %v; want refused as outside the project", name, uri, got, err)
		}
	}
}

func TestFileFetcherRefusesWithoutAProjectRoot(t *testing.T) {
	root, _ := project(t)
	_, err := (FileFetcher{}).Fetch(stdctx.Background(), fileURI(filepath.Join(root, "docs", "spec.md")))
	if !errors.Is(err, api.ErrInvalidInput) || !strings.Contains(err.Error(), "no project root") {
		t.Fatalf("Fetch with no project root = %v, want refused", err)
	}
}

// THE SNAPSHOT IS CUT ON CHARACTERS: a cut inside a UTF-8 sequence would store
// an invalid string.
func TestSnapshotIsCutOnCharacters(t *testing.T) {
	section := "a" + strings.Repeat("é", maxSnapshotChars+100) // 'é' is two bytes
	anc, found := anchorSection(section, nil)
	if !found {
		t.Fatal("whole document not found")
	}
	if !utf8.ValidString(anc.Snapshot) || utf8.RuneCountInString(anc.Snapshot) != maxSnapshotChars {
		t.Fatalf("snapshot: valid=%v, %d characters; want valid UTF-8 of %d characters",
			utf8.ValidString(anc.Snapshot), utf8.RuneCountInString(anc.Snapshot), maxSnapshotChars)
	}
	if short := "déjà vu"; truncate(short, maxSnapshotChars) != short {
		t.Fatal("a short text must be kept whole")
	}
}

// INGEST AND REANCHOR HASH THE SAME WAY: a source ingested and re-anchored
// unchanged matches, and the two hashes are one.
func TestIngestHashIsTheReanchorHash(t *testing.T) {
	ctx, uri := writeSourceFile(t, "# Auth\n\n## Rate Limits\n\n  Limit 100   rps.\n\n## Next\n")
	ing, err := New().Ingest(ctx, nil, uri, section, "", "tester")
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	b, _ := decoded(t, ing.Body)
	if b.ContentHash != ing.Hash || b.AnchorStatus != AnchorStatusValid || b.IngestedAt.IsZero() || b.IngestedBy != "tester" {
		t.Fatalf("ingested body = %s", ing.Body)
	}
	res, err := New().Reanchor(ctx, ing.Body, "", false)
	if err != nil {
		t.Fatalf("Reanchor: %v", err)
	}
	if res.Status != "match" || res.NewHash != ing.Hash {
		t.Fatalf("Reanchor after Ingest = %+v, want match on %s", res, ing.Hash)
	}
}

func TestIngestRefusesAMissingSection(t *testing.T) {
	ctx, uri := writeSourceFile(t, "# Auth\n\n## Other\n\nx\n")
	if _, err := New().Ingest(ctx, nil, uri, section, "", ""); !errors.Is(err, api.ErrInvalidInput) {
		t.Fatalf("Ingest of an absent section = %v, want ErrInvalidInput", err)
	}
}
