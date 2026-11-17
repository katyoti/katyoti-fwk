// SPDX-License-Identifier: Apache-2.0

package file_write

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWrite_RefusesSecretPaths(t *testing.T) {
	_, root, run := newRunner(t, nil)
	for _, name := range []string{".env", ".env.local", ".secrets/x.txt", "id_ed25519", "a.key"} {
		_, err := run(WriteInput{WorkspacePath: name, Content: "x\n"})
		if err == nil || !strings.Contains(err.Error(), "secret path") {
			t.Errorf("write %s: %v, want a secret-path refusal", name, err)
		}
		if _, statErr := os.Stat(filepath.Join(root, name)); statErr == nil {
			t.Errorf("write %s created the file", name)
		}
	}
	if _, err := run(WriteInput{WorkspacePath: ".env.example", Content: "K=\n"}); err != nil {
		t.Errorf("write .env.example refused: %v", err)
	}
}
