// SPDX-License-Identifier: Apache-2.0

package grep

import (
	"context"
	"strings"
	"testing"
)

// grep never reads a secret file's content, and refuses a secret root.
func TestGrep_SkipsSecretFiles(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, map[string]string{
		"a.go":             "TOKEN=public\n",
		".env":             "TOKEN=canary\n",
		".env.example":     "TOKEN=\n",
		".secret/x.txt":    "TOKEN=canary\n",
		"hub/keys/gui.key": "TOKEN=canary\n",
	})
	r := &fakeResolver{root: root}
	out, err := executeGrep(context.Background(), GrepInput{Pattern: "TOKEN", OutputMode: "content"}, r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.Content, "canary") {
		t.Errorf("grep read a secret file: %q", out.Content)
	}
	if !strings.Contains(out.Content, "a.go") || !strings.Contains(out.Content, ".env.example") {
		t.Errorf("grep lost a plain file: %q", out.Content)
	}
	if _, err := executeGrep(context.Background(), GrepInput{Pattern: "TOKEN", WorkspacePath: ".secret"}, r); err == nil ||
		!strings.Contains(err.Error(), "secret path") {
		t.Errorf("grep in .secret: %v, want a secret-path refusal", err)
	}
}
