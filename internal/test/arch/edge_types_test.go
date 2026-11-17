// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestEveryEdgeTypeDeclarationLoads reads every edge type a pack embeds and
// asks CWM's own check whether it would load: one it refuses is skipped at
// boot with a warning only, and every edge of its kind is then refused at
// run time as "declared by no pack" (realized-by, declared N:N).
func TestEveryEdgeTypeDeclarationLoads(t *testing.T) {
	root := repoRoot(t)
	var files []string
	for _, pattern := range []string{
		"internal/extensions/*/contrib/embed/cwm/edge-types/*.yaml",
		"app/*/embed/cwm/edge-types/*.yaml",
	} {
		got, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, got...)
	}
	if len(files) == 0 {
		t.Fatal("no edge type declaration found: the globs no longer match the packs")
	}
	for _, f := range files {
		if strings.HasSuffix(f, ".i18n.yaml") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var r cwmapi.EdgeTypeRegistration
		if err := config.DecodeBytes(data, marshal.YAML, &r); err != nil {
			t.Errorf("%s: %v", rel(root, f), err)
			continue
		}
		if err := r.Validate(); err != nil {
			t.Errorf("%s would be skipped at load: %v", rel(root, f), err)
		}
	}
}
