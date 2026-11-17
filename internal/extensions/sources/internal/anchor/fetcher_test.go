// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	stdctx "context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileFetcher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec.md")
	body := "# Heading\n\nSome content.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Build a file:// URI. On Windows, file:///C:/path is the right
	// shape; on POSIX, file:///abs.
	var uri string
	if runtime.GOOS == "windows" {
		uri = "file:///" + filepath.ToSlash(path)
	} else {
		uri = "file://" + filepath.ToSlash(path)
	}
	f := FileFetcher{}
	if f.Scheme() != "file" {
		t.Errorf("Scheme: got %q want file", f.Scheme())
	}
	got, err := f.Fetch(WithProjectRoot(stdctx.Background(), dir), uri)
	if err != nil {
		t.Fatalf("Fetch(%s): %v", uri, err)
	}
	if string(got) != body {
		t.Errorf("Fetch payload mismatch: got %q want %q", got, body)
	}
}

func TestFileFetcherMissingFile(t *testing.T) {
	_, err := (FileFetcher{}).Fetch(WithProjectRoot(stdctx.Background(), t.TempDir()), "file://./does/not/exist/spec.md")
	if err == nil {
		t.Errorf("expected error for missing file")
	}
}

func TestFileFetcherWrongScheme(t *testing.T) {
	_, err := (FileFetcher{}).Fetch(stdctx.Background(), "http://example.com/x")
	if err == nil {
		t.Errorf("expected error for wrong scheme")
	}
}

func TestRegisterFetcherReplaces(t *testing.T) {
	r := newFetcherRegistry()
	r.register("file", FileFetcher{})

	if _, ok := r.lookup("file"); !ok {
		t.Errorf("file scheme not registered")
	}
	if _, ok := r.lookup("nope"); ok {
		t.Errorf("unknown scheme should miss")
	}

	// Replace: register a stub with the same scheme.
	stub := stubFetcher{scheme: "file"}
	r.register("file", stub)
	got, ok := r.lookup("file")
	if !ok {
		t.Fatalf("file scheme not registered after replace")
	}
	if _, isStub := got.(stubFetcher); !isStub {
		t.Errorf("Fetcher not replaced: got %T", got)
	}

	// Case insensitive on scheme.
	r.register("HTTP", stub)
	if _, ok := r.lookup("http"); !ok {
		t.Errorf("scheme normalization broken")
	}
}

func TestSchemeOf(t *testing.T) {
	cases := map[string]string{
		"file:///foo":         "file",
		"https://x.example/y": "https",
		"confluence://a/b":    "confluence",
		"no-scheme/path":      "",
		"":                    "",
	}
	for in, want := range cases {
		if got := schemeOf(in); got != want {
			t.Errorf("schemeOf(%q) = %q want %q", in, got, want)
		}
	}
}

type stubFetcher struct{ scheme string }

func (s stubFetcher) Scheme() string { return s.scheme }
func (s stubFetcher) Fetch(stdctx.Context, string) ([]byte, error) {
	return []byte("stub"), nil
}
