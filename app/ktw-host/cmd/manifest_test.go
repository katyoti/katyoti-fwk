// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// The host reads a folder's ktw-host.yaml whole: the declaration it shares
// with the hub's folder member (identity, dirs) and its own `config:` block.
func TestTheHostReadsTheDeclarationAndItsConfig(t *testing.T) {
	dir := t.TempDir()
	body := "name: acme\nversion: 1.2.0\ndirs:\n  cwm: docs/cwm\nconfig:\n  apiKeys:\n    - envName: ACME_KEY\n"
	if err := os.WriteFile(filepath.Join(dir, "ktw-host.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := readManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "acme" || m.Version != "1.2.0" || m.Dirs["cwm"] != "docs/cwm" {
		t.Fatalf("declaration = %+v", m.FolderDecl)
	}
	if len(m.Config.APIKeys) != 1 || m.Config.APIKeys[0].EnvName != "ACME_KEY" {
		t.Fatalf("config = %+v: the declared key is not read", m.Config)
	}
}
