// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// pemPrivateRE is a PEM private key block, its body captured.
var pemPrivateRE = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----(.*?)-----END [A-Z ]*PRIVATE KEY-----`)

// TestNoPrivateKeyIsTracked: no tracked file holds a private signing key -
// a KaTyoti or minisign secret key (an `untrusted comment:` line, then a key
// blob, and no `trusted comment:` line as a signature has), or a PEM private
// key block long enough to be a real key. A key a test needs
// is generated in t.TempDir().
func TestNoPrivateKeyIsTracked(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	var found []string
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel == "" {
			continue
		}
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<10 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		if why := privateKeyIn(data); why != "" {
			found = append(found, rel+": "+why)
		}
	}
	if len(found) > 0 {
		t.Fatalf("tracked private keys (remove them; a key a test needs is generated in t.TempDir()):\n  %s", strings.Join(found, "\n  "))
	}
}

// privateKeyIn says which private key data holds; "" when none.
func privateKeyIn(data []byte) string {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	for _, m := range pemPrivateRE.FindAllStringSubmatch(text, -1) {
		body := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\t' || r == '#' || r == '"' || r == '\\' {
				return -1
			}
			return r
		}, m[1])
		if len(body) >= 200 {
			return "a PEM private key"
		}
	}
	if strings.Contains(text, "\ntrusted comment:") {
		return "" // a signature: its blob looks like a key's
	}
	lines := strings.Split(text, "\n")
	for i := 0; i+1 < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "untrusted comment:") {
			continue
		}
		blob, err := base64.StdEncoding.DecodeString(strings.TrimSpace(lines[i+1]))
		if err != nil || len(blob) < 2 || string(blob[:2]) != "Ed" {
			continue
		}
		// KaTyoti's unencrypted secret key: Ed + key id (8) + ed25519 private
		// key (64); minisign's encrypted one: 158 bytes.
		if len(blob) == 74 || len(blob) == 158 {
			return "a signing secret key"
		}
	}
	return ""
}
