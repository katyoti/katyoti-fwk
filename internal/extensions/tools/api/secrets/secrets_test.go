// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"strings"
	"testing"
)

func TestIsSecretPath(t *testing.T) {
	secret := []string{
		".env", "a/.env", `C:\p\.env.build`, "/x/.env.test", ".ENV.Local",
		".secret/x", "a/.secrets/b/c.txt", "/data/hub/keys/gui.key",
		`D:\u\hub\keys`, "hub/keys/cli.seed",
		"cert.pem", "a/b/server.key", "x.p12", "x.pfx", "x.jks", "x.keystore",
		"api.token", "~/.ssh/id_rsa", "id_ed25519", "/home/u/.netrc",
		".pgpass", ".git-credentials", "prod.env", `deploy\Prod.ENV`,
	}
	for _, p := range secret {
		if !IsSecretPath(p) {
			t.Errorf("IsSecretPath(%q) = false, want true", p)
		}
	}
	plain := []string{
		"", ".env.example", ".env.build.example", "a/.env.test.example",
		"main.go", "README.md", "id_rsa.pub", "keys/x.txt", "hub/x",
		"environment.go", ".envrc.md", "prod.env.example", "env.go", "secret.go", "token.go", "a/keys.go",
	}
	for _, p := range plain {
		if IsSecretPath(p) {
			t.Errorf("IsSecretPath(%q) = true, want false", p)
		}
	}
}

func TestGitignorePatternsCoverTheMatcher(t *testing.T) {
	// Every pattern's literal sample is a secret path - the matcher and the
	// list cannot drift apart silently.
	for _, pat := range GitignorePatterns() {
		if strings.HasPrefix(pat, "!") {
			if IsSecretPath(strings.ReplaceAll(pat[1:], "*", "build")) {
				t.Errorf("negated pattern %q sample is a secret path", pat)
			}
			continue
		}
		sample := strings.ReplaceAll(strings.TrimPrefix(pat, "**/"), "*", "x")
		if strings.HasSuffix(sample, "/") {
			sample += "f"
		}
		if !IsSecretPath(sample) {
			t.Errorf("pattern %q: sample %q is not a secret path", pat, sample)
		}
	}
}

func TestCommandSecretPath(t *testing.T) {
	cases := map[string]string{
		"cat .env":                         ".env",
		`cat "$HOME/.secret/token"`:        "$HOME/.secret/token",
		"grep KEY < .env.build":            ".env.build",
		"cp a.pem /tmp/x && echo ok":       "a.pem",
		"curl -d @hub/keys/gui.key http:x": "hub/keys/gui.key",
		"ls -la":                           "",
		"cat .env.example":                 "",
		"go test ./internal/...":           "",
	}
	for cmd, want := range cases {
		if got := CommandSecretPath(cmd); got != want {
			t.Errorf("CommandSecretPath(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func TestAgentEnvKeepsOnlyTheAllowList(t *testing.T) {
	in := []string{
		"PATH=/bin", "Path=C:\\Windows", "HOME=/home/u", "TEMP=/tmp",
		"KTW_HOME=/k", "LC_ALL=C",
		"LEANIX_API_TOKEN=s3cr3t", "ANTHROPIC_API_KEY=sk-x", "CONFLUENCE_PASSWORD=p",
		"KTW_CONNECTOR_KEY=k", "KATYOTI_SECRET=x", "AWS_SECRET_ACCESS_KEY=a",
		"GITHUB_TOKEN=g", "MY_RANDOM=1", "NOEQUALS",
	}
	got := AgentEnv(in)
	want := []string{"PATH=/bin", "Path=C:\\Windows", "HOME=/home/u", "TEMP=/tmp", "KTW_HOME=/k", "LC_ALL=C"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("AgentEnv = %v, want %v", got, want)
	}
}
