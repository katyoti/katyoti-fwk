// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/redact"
)

// TestRecorder_RedactsSecretsInBothJournals — the threat-T10 redaction
// half, end-to-end at the recorder level: a secret env value logged through
// either journal must land on disk as a marker, never verbatim.
func TestRecorder_RedactsSecretsInBothJournals(t *testing.T) {
	// An invented api key, in two pieces so that no credential shape sits in the
	// source; the joined value is the shape the redactor's pattern layer catches.
	const fakeKey = "sk-" + "abcdefghij0123456789"
	t.Setenv("E2E_SMOKE_SECRET_KEY", "journal-secret-value-42")

	dir := t.TempDir()
	rec, err := NewSessionRecorder(dir, "redact-smoke", "")
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}
	rec.SetRedactor(redact.New(redact.RedactionSettings{}))

	rec.LLMJournal().Log(JournalLLMRequest, map[string]any{
		"prompt": "use the key journal-secret-value-42 and the api key " + fakeKey,
	})
	rec.UserJournal().Log(UserMessageShown, 1, "", map[string]any{
		"text": "your secret is journal-secret-value-42",
	})
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	for _, name := range []string{"llm.jsonl", "user.jsonl"} {
		raw, err := os.ReadFile(filepath.Join(rec.SessionDir(), name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		content := string(raw)
		if strings.Contains(content, "journal-secret-value-42") {
			t.Errorf("%s: secret env value reached disk verbatim", name)
		}
		if !strings.Contains(content, "[REDACTED:env:E2E_SMOKE_SECRET_KEY]") {
			t.Errorf("%s: env marker missing — redaction not applied:\n%s", name, content)
		}
	}

	// The pattern layer fires too (llm journal carried an sk- key).
	raw, _ := os.ReadFile(filepath.Join(rec.SessionDir(), "llm.jsonl"))
	if !strings.Contains(string(raw), "[REDACTED:api-key]") {
		t.Errorf("llm.jsonl: pattern-layer marker missing")
	}
	if strings.Contains(string(raw), fakeKey) {
		t.Errorf("llm.jsonl: pattern-shaped key reached disk verbatim")
	}
}

// TestRecorder_NoRedactor_IsNoOp — a recorder without SetRedactor (or with
// a Disabled config) writes verbatim; pins the nil-safety of the chokepoint.
func TestRecorder_NoRedactor_IsNoOp(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewSessionRecorder(dir, "no-redact", "")
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}
	rec.LLMJournal().Log(JournalLLMRequest, map[string]any{"x": "plain-content"})
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(rec.SessionDir(), "llm.jsonl"))
	if !strings.Contains(string(raw), "plain-content") {
		t.Error("nil redactor must not alter content")
	}
}
