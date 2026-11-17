// SPDX-License-Identifier: Apache-2.0

package file_read

import (
	"strings"
	"testing"
)

// The file tools refuse secret paths; a .env's .example stays readable.
func TestRunRead_RefusesSecretPaths(t *testing.T) {
	files := map[string]string{
		".env":             "LEANIX_API_TOKEN=canary\n",
		".env.build":       "K=canary\n",
		".secret/note.txt": "canary\n",
		"certs/server.pem": "canary\n",
		"keys/api.token":   "canary\n",
		"hub/keys/gui.key": "canary\n",
		".env.example":     "LEANIX_API_TOKEN=\n",
	}
	_, _, run := newRunner(t, files)
	for name := range files {
		out, err := run(ReadInput{WorkspacePath: name})
		if name == ".env.example" {
			if err != nil {
				t.Errorf("read %s refused: %v", name, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("read %s answered %+v, want a refusal", name, out)
			continue
		}
		if !strings.Contains(err.Error(), "secret path") {
			t.Errorf("read %s: %v, want a secret-path refusal", name, err)
		}
	}
}
