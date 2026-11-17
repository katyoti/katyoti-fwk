// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An agent's shell starts from the allow-listed environment: a connector
// key in the parent process never shows in `env`.
func TestRunCommand_EnvHoldsNoConnectorSecret(t *testing.T) {
	bashPath := requireBash(t)
	t.Setenv("KTW_TEST_LEANIX_API_TOKEN", "canary-env-value-123456")
	t.Setenv("LEANIX_API_TOKEN", "canary-env-value-654321")
	result, err := RunCommand(context.Background(), bashPath, "env", "", defaultTimeoutMs)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("env exit %d, stderr=%q", result.ExitCode, result.Stderr)
	}
	for _, leak := range []string{"LEANIX_API_TOKEN", "canary-env-value"} {
		if strings.Contains(result.Stdout, leak) {
			t.Errorf("env output shows %q:\n%s", leak, result.Stdout)
		}
	}
	if !strings.Contains(result.Stdout, "PATH=") {
		t.Errorf("env output lost PATH:\n%s", result.Stdout)
	}
}

// A cat of a secret file is refused before the shell starts; its
// .example stays readable.
func TestRunCommand_RefusesSecretPath(t *testing.T) {
	bashPath := requireBash(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("LEANIX_API_TOKEN=canary-file-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.example"), []byte("LEANIX_API_TOKEN=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"cat .env", "cat ./.env | head", "grep TOKEN < .env"} {
		result, err := RunCommand(context.Background(), bashPath, cmd, dir, defaultTimeoutMs)
		if err == nil {
			t.Errorf("%q ran (stdout %q), want a refusal", cmd, result.Stdout)
			continue
		}
		if !strings.Contains(err.Error(), "secret path") {
			t.Errorf("%q: error %v, want a secret-path refusal", cmd, err)
		}
	}
	if _, err := RunBackground(bashPath, "cat .env", dir); err == nil {
		t.Error("background cat .env ran, want a refusal")
	}
	result, err := RunCommand(context.Background(), bashPath, "cat .env.example", dir, defaultTimeoutMs)
	if err != nil {
		t.Fatalf("cat .env.example refused: %v", err)
	}
	if !strings.Contains(result.Stdout, "LEANIX_API_TOKEN=") {
		t.Errorf("cat .env.example = %q", result.Stdout)
	}
}

// A secret a command prints anyway (a value of this process's secret
// variables, a known credential shape) is scrubbed from the result.
func TestRunCommand_RedactsSecretsInResult(t *testing.T) {
	bashPath := requireBash(t)
	t.Setenv("KTW_TEST_SECRET", "canary-redacted-value-42")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"),
		[]byte("v=canary-redacted-value-42 k=sk-abcdefghijklmnopqrstuvwxyz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := RunCommand(context.Background(), bashPath, "cat notes.txt; echo canary-redacted-value-42 >&2", dir, defaultTimeoutMs)
	if err != nil {
		t.Fatal(err)
	}
	all := result.Stdout + result.Stderr
	for _, leak := range []string{"canary-redacted-value-42", "sk-abcdefghijklmnopqrstuvwxyz"} {
		if strings.Contains(all, leak) {
			t.Errorf("result shows %q: %q", leak, all)
		}
	}
	if !strings.Contains(result.Stdout, "[REDACTED:") {
		t.Errorf("stdout carries no redaction marker: %q", result.Stdout)
	}
}
