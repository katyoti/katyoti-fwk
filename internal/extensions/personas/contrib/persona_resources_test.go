// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// The persona provider holds the provider contract over a REAL shipped pack
// (ktw-gocode's personas), not a fixture: the entries it enumerates open
// through Open with bytes that hash to their addresses.
func TestPersonaProvider_HoldsTheProviderContract(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "app", "ktw-gocode")
	if _, err := os.Stat(filepath.Join(root, "embed", "personas")); err != nil {
		t.Fatalf("ktw-gocode persona pack not found: %v", err)
	}
	p := PersonaProvider("ktw-gocode", loader.NewEmbedFSLoader(os.DirFS(root), "."), "embed/personas")
	entries, err := p.Enumerate(context.Background())
	if err != nil || len(entries) == 0 {
		t.Fatalf("Enumerate = %d entries, err %v — the contract would be checked over nothing", len(entries), err)
	}
	providertest.Run(t, p)
}
