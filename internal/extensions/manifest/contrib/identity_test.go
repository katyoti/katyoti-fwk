// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"embed"
	"os"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

//go:embed testdata
var fixedFS embed.FS

// TestIdentityFollowsSource: an embedded source is provably fixed, a directory
// is not. Getting this wrong in the "immutable" direction serves stale content
// forever, so the default must be mutable.
func TestIdentityFollowsSource(t *testing.T) {
	fixed := &EmbedFSResourceProvider{Loader: loader.NewEmbedFSLoader(fixedFS, "testdata"), Root: "testdata", ResourceType: "x", ProviderName: "p"}
	if fixed.Identity().Mutable {
		t.Error("an embed.FS source must be reported immutable")
	}
	live := &EmbedFSResourceProvider{Loader: loader.NewFSLoader(t.TempDir()), Root: ".", ResourceType: "x", ProviderName: "p"}
	if !live.Identity().Mutable {
		t.Error("a directory source must be reported mutable — assuming otherwise serves stale content")
	}
	unknown := &EmbedFSResourceProvider{Loader: loader.NewEmbedFSLoader(os.DirFS(t.TempDir()), "."), Root: ".", ResourceType: "x", ProviderName: "p"}
	if !unknown.Identity().Mutable {
		t.Error("an fs.FS that is not provably fixed must default to mutable")
	}
}

// TestImmutableEnumeratesOnce: the whole point of declaring immutability.
func TestImmutableEnumeratesOnce(t *testing.T) {
	p := &EmbedFSResourceProvider{
		Loader: loader.NewEmbedFSLoader(fixedFS, "testdata"), Root: "testdata",
		ResourceType: "x", ProviderName: "p", FileExtensions: []string{"golden"},
	}
	a, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	b, _ := p.Enumerate(context.Background())
	if len(a) != len(b) {
		t.Fatalf("second enumeration differs: %d vs %d", len(a), len(b))
	}
	if len(a) > 0 && &a[0] != &b[0] {
		t.Error("an immutable source must be enumerated once and shared, not re-walked")
	}
}
