// SPDX-License-Identifier: Apache-2.0

package file_edit

import (
	"strings"
	"testing"
)

func TestRunEdit_RefusesSecretPaths(t *testing.T) {
	_, _, run := newRunner(t, map[string]string{".env.test": "K=old\n", "x.pem": "old\n"})
	for _, name := range []string{".env.test", "x.pem"} {
		_, err := run(EditInput{WorkspacePath: name, OldString: "old", NewString: "new"})
		if err == nil || !strings.Contains(err.Error(), "secret path") {
			t.Errorf("edit %s: %v, want a secret-path refusal", name, err)
		}
	}
}
