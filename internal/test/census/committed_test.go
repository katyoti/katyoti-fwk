// SPDX-License-Identifier: Apache-2.0

package census

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	taskcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib"
)

// A bless in a CRLF working copy and a check on an LF clean checkout of the
// same commit agree: the census hashes what is committed. A real producer
// reads the same declaration once with CRLF line endings (a Windows working
// copy) and once with LF (a clean checkout); the raw layer hashes differ, the
// census lines do not.
func TestCensus_ABlessOnACRLFCopyPassesOnAnLFCopy(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "internal", "core", "contribution", "embed", "contribution-type", "guide-recipe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	lf := bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	crlf := bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))

	blessed, rawCRLF := censusOfCopy(t, crlf)
	checked, rawLF := censusOfCopy(t, lf)

	if rawCRLF == rawLF {
		t.Fatalf("the fixture proves nothing: the producer hashes CRLF and LF alike\n%s", rawLF)
	}
	if blessed != checked {
		t.Fatalf("a census blessed on a CRLF copy fails on an LF copy:\nblessed (CRLF):\n%s\nchecked (LF):\n%s", blessed, checked)
	}
	if !strings.Contains(checked, "source=") {
		t.Fatalf("the fixture carries no source layer, so it proves nothing about raw bytes:\n%s", checked)
	}
}

// Binary content keeps its own hash: an image's bytes may hold "\r\n" that are
// not line endings.
func TestCensus_BinaryLayersKeepTheirHash(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	if _, ok := lfHash(png); ok {
		t.Fatal("binary bytes were read as text")
	}
	if _, ok := lfHash([]byte("a\nb\n")); ok {
		t.Fatal("LF text was remapped")
	}
	if h, ok := lfHash([]byte("a\r\nb\r\n")); !ok || h != contribapi.HashContent([]byte("a\nb\n")) {
		t.Fatalf("CRLF text hashed %q, %v; want the LF hash", h, ok)
	}
}

// censusOfCopy writes decl as a kind declaration in a fresh folder, has the
// contribution-type producer enumerate it, and returns the census lines of
// that manifest, then the same lines with the raw hashes (nothing remapped).
func censusOfCopy(t *testing.T, decl []byte) (census, raw string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "contribution-types"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "contribution-types", "guide-recipe.yaml"), decl, 0o644); err != nil {
		t.Fatal(err)
	}
	p := manifestcontrib.NewContributionTypeProvider("fixture", loader.NewFSLoader(dir), "contribution-types")
	ctx := context.Background()
	items, err := p.Enumerate(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("enumerate: %d items, %v", len(items), err)
	}
	m := &mbapi.Manifest{
		Contributions: items,
		Providers:     []mbapi.ProviderInfo{{Name: p.Provider(), Type: p.Type(), Digest: contribapi.DigestOf(items)}},
	}
	census = strings.Join(censusLines(m, committedFrom(ctx, []contribapi.ContributionProvider{p}, m)), "\n")
	raw = strings.Join(censusLines(m, committedHashes{}), "\n")
	return census, raw
}

// A frontmatter kind folds its file's body into its data: a task blessed on a
// CRLF working copy and checked on an LF clean checkout agree on the data
// layer too, not only on the source (the data carries the CRLF JSON-escaped,
// which a raw-bytes normalisation never sees).
func TestCensus_ATaskBlessedOnACRLFCopyPassesOnAnLFCopy(t *testing.T) {
	lf := []byte("---\nname: fixture\ntype: agent\ndescription: \"a fixture task\"\nconfig:\n  mode: worker\n---\n\n# Fixture\n\nOne line.\nAnother line.\n")
	crlf := bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))

	blessed, rawCRLF := taskCensusOfCopy(t, crlf)
	checked, rawLF := taskCensusOfCopy(t, lf)
	if rawCRLF == rawLF {
		t.Fatalf("the fixture proves nothing: the producer hashes CRLF and LF alike\n%s", rawLF)
	}
	if blessed != checked {
		t.Fatalf("a task census blessed on a CRLF copy fails on an LF copy:\nblessed (CRLF):\n%s\nchecked (LF):\n%s", blessed, checked)
	}
}

// An escaped backslash before `r\n` is not a CRLF: only a `\r\n` escape whose
// backslash is not itself escaped is remapped.
func TestCensus_EscapedCRLFOnlyWhereItIsOne(t *testing.T) {
	if _, ok := lfEscapedHash([]byte(`{"body":"a\\r\\nb"}`)); ok {
		t.Fatal("an escaped backslash followed by r was read as a CRLF escape")
	}
	if h, ok := lfEscapedHash([]byte(`{"body":"a\r\nb\\"}`)); !ok || h != contribapi.HashContent([]byte(`{"body":"a\nb\\"}`)) {
		t.Fatalf("a CRLF escape hashed %q, %v; want the LF escape's hash", h, ok)
	}
}

// taskCensusOfCopy writes body as tasks/fixture.md in a fresh folder, has the
// task producer enumerate it, and returns the census lines of that manifest,
// then the same lines with the raw hashes.
func taskCensusOfCopy(t *testing.T, body []byte) (census, raw string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tasks", "fixture.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var providers []contribapi.ContributionProvider
	m := &mbapi.Manifest{}
	for _, p := range taskcontrib.TaskProviders("fixture", loader.NewFSLoader(dir), "tasks") {
		items, err := p.Enumerate(ctx)
		if err != nil {
			t.Fatalf("enumerate: %v", err)
		}
		providers = append(providers, p)
		m.Contributions = append(m.Contributions, items...)
		m.Providers = append(m.Providers, mbapi.ProviderInfo{Name: p.Provider(), Type: p.Type(), Digest: contribapi.DigestOf(items)})
	}
	if len(m.Contributions) != 1 {
		t.Fatalf("enumerate: %d entries, want the one task", len(m.Contributions))
	}
	census = strings.Join(censusLines(m, committedFrom(ctx, providers, m)), "\n")
	raw = strings.Join(censusLines(m, committedHashes{}), "\n")
	return census, raw
}
