// SPDX-License-Identifier: Apache-2.0

package redact

import (
	"strings"
	"testing"
)

func TestRedactor_EnvValues(t *testing.T) {
	t.Setenv("SMOKE_SECRET_KEY", "super-secret-value-123")
	t.Setenv("SMOKE_PLAIN", "not-secretish-name-so-kept") // name doesn't match heuristic
	t.Setenv("SHORT_KEY", "tiny")                         // below MinEnvValueLen

	r := New(RedactionSettings{})

	in := `{"prompt":"the key is super-secret-value-123 and SMOKE_PLAIN is not-secretish-name-so-kept and tiny"}`
	out := r.RedactString(in)

	if strings.Contains(out, "super-secret-value-123") {
		t.Error("secret env value survived redaction")
	}
	if !strings.Contains(out, "[REDACTED:env:SMOKE_SECRET_KEY]") {
		t.Errorf("marker missing: %s", out)
	}
	if !strings.Contains(out, "not-secretish-name-so-kept") {
		t.Error("non-secretish env value must be kept")
	}
	if !strings.Contains(out, "tiny") {
		t.Error("short value must be kept (MinEnvValueLen guard)")
	}
}

func TestRedactor_EnvValue_JSONEscapedForm(t *testing.T) {
	t.Setenv("SMOKE_TOKEN", `va"lue-with-quote-123`)
	r := New(RedactionSettings{})

	// As it appears inside a marshaled JSON line: the quote is escaped.
	in := `{"data":"va\"lue-with-quote-123"}`
	out := r.RedactString(in)
	if strings.Contains(out, `lue-with-quote-123`) {
		t.Errorf("JSON-escaped secret survived: %s", out)
	}
}

func TestRedactor_Patterns(t *testing.T) {
	r := New(RedactionSettings{})
	cases := []struct {
		in   string
		want string
	}{
		{"key sk-abcdefghij0123456789 used", "[REDACTED:api-key]"},
		{"aws AKIAIOSFODNN7EXAMPLE here", "[REDACTED:aws-key]"},
		{"gh ghp_abcdefghijklmnopqrstuvwxyz0123456789 done", "[REDACTED:github-token]"},
		{"slack xoxb-1234567890-abcdef ok", "[REDACTED:slack-token]"},
		{"Authorization: Bearer abcdefghijklmnopqrstuvwxyz123456", "[REDACTED:bearer]"},
		{"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.SflKxwRJSMeKKF2QT4fwpM ok", "[REDACTED:jwt]"},
		{"-----BEGIN RSA PRIVATE KEY----- body", "[REDACTED:private-key]"},
		{`conn "postgres://user:hunter22pass@db:5432/x"`, "[REDACTED:url-credential]"},
	}
	for _, tc := range cases {
		out := r.RedactString(tc.in)
		if !strings.Contains(out, tc.want) {
			t.Errorf("RedactString(%q) = %q, want it to contain %s", tc.in, out, tc.want)
		}
	}
}

func TestRedactor_URLCredential_KeepsUser(t *testing.T) {
	r := New(RedactionSettings{})
	out := r.RedactString("postgres://alice:hunter22pass@db:5432/x")
	if !strings.Contains(out, "alice:") {
		t.Errorf("username should be kept: %s", out)
	}
	if strings.Contains(out, "hunter22pass") {
		t.Errorf("password survived: %s", out)
	}
}

func TestRedactor_DisabledAndNil(t *testing.T) {
	if New(RedactionSettings{Disabled: true}) != nil {
		t.Error("Disabled settings must produce a nil Redactor")
	}
	var r *Redactor
	if got := r.RedactString("sk-abcdefghij0123456789"); got != "sk-abcdefghij0123456789" {
		t.Error("nil Redactor must be a no-op")
	}
}

func TestRedactor_ExtraEnvVarsAndPatterns(t *testing.T) {
	t.Setenv("INNOCUOUS_NAME", "value-that-matters-99")
	r := New(RedactionSettings{
		ExtraEnvVars:  []string{"INNOCUOUS_NAME"},
		ExtraPatterns: []string{`CUST-[0-9]{6}`},
	})
	out := r.RedactString("v value-that-matters-99 and CUST-123456")
	if strings.Contains(out, "value-that-matters-99") {
		t.Error("ExtraEnvVars value survived")
	}
	if !strings.Contains(out, "[REDACTED:custom]") {
		t.Errorf("ExtraPatterns not applied: %s", out)
	}
}

func TestWriter_RedactsAndReportsOriginalLen(t *testing.T) {
	t.Setenv("SMOKE_API_KEY", "writer-secret-000111")
	r := New(RedactionSettings{})
	var sb strings.Builder
	w := Writer(&sb, r)

	in := []byte(`{"x":"writer-secret-000111"}` + "\n")
	n, err := w.Write(in)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != len(in) {
		t.Errorf("n = %d, want original length %d", n, len(in))
	}
	if strings.Contains(sb.String(), "writer-secret-000111") {
		t.Error("secret reached the underlying writer")
	}
}

// NewWithValues masks a value given by the caller whatever its name.
func TestNewWithValuesMasksAPlainNamedValue(t *testing.T) {
	r := NewWithValues(RedactionSettings{}, map[string]string{"PLAIN_SETTING": "fixture-plain-value-4242", "SHORT": "abc"})
	got := r.RedactString("a fixture-plain-value-4242 b abc")
	if got != "a [REDACTED:env:PLAIN_SETTING] b abc" {
		t.Errorf("got %q", got)
	}
}
