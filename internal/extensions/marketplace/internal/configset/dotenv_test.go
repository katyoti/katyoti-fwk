// SPDX-License-Identifier: Apache-2.0

package configset

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func envFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if body != "" {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestReadEnv_MissingFileIsEmptyNotError(t *testing.T) {
	m, err := ReadEnv(filepath.Join(t.TempDir(), "nope.env"))
	if err != nil || len(m) != 0 {
		t.Fatalf("missing .env must read as empty: m=%v err=%v", m, err)
	}
}

func TestReadEnv_ParsesQuotesCommentsExport(t *testing.T) {
	p := envFile(t, "# a comment\nA=1\n\nexport B=two\nC=\"with space\"\nD='single'\nnot-an-assignment\n")
	m, err := ReadEnv(p)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"A": "1", "B": "two", "C": "with space", "D": "single"} {
		if m[k] != want {
			t.Errorf("%s = %q, want %q", k, m[k], want)
		}
	}
	if len(m) != 4 {
		t.Errorf("unexpected extra keys: %v", m)
	}
}

// THE invariant: a .env is shared with whatever the user already set, so writing
// our keys must not disturb anything else — including comments and ordering.
func TestWriteEnv_PreservesUnrelatedContent(t *testing.T) {
	p := envFile(t, "# keep me\nUNRELATED=untouched\nLEANIX_API_TOKEN=old\n\n# trailing comment\n")
	changed, err := WriteEnv(p, map[string]string{"LEANIX_API_TOKEN": "new", "NEW_KEY": "added"})
	if err != nil {
		t.Fatal(err)
	}
	got := read(t, p)

	for _, want := range []string{"# keep me", "UNRELATED=untouched", "# trailing comment"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost unrelated content %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "LEANIX_API_TOKEN=new") || strings.Contains(got, "LEANIX_API_TOKEN=old") {
		t.Errorf("existing key not updated in place:\n%s", got)
	}
	if !strings.Contains(got, "NEW_KEY=added") {
		t.Errorf("new key not appended:\n%s", got)
	}
	if len(changed) != 2 {
		t.Errorf("changed = %v, want both keys", changed)
	}
}

// Re-applying with identical values must be a no-op: nothing rewritten, nothing
// reported as changed. That is what makes `config-set apply` idempotent.
func TestWriteEnv_IdempotentNoOp(t *testing.T) {
	p := envFile(t, "A=1\n")
	before := read(t, p)
	changed, err := WriteEnv(p, map[string]string{"A": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Errorf("unchanged value must report no change, got %v", changed)
	}
	if read(t, p) != before {
		t.Errorf("file rewritten despite no change")
	}
}

func TestWriteEnv_CreatesFileAndQuotesWhenNeeded(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", ".env")
	if _, err := WriteEnv(p, map[string]string{"PLAIN": "abc", "SPACED": "a b", "EMPTY": ""}); err != nil {
		t.Fatal(err)
	}
	got := read(t, p)
	if !strings.Contains(got, "PLAIN=abc") {
		t.Errorf("plain value should be unquoted:\n%s", got)
	}
	if !strings.Contains(got, `SPACED="a b"`) {
		t.Errorf("spaced value must be quoted:\n%s", got)
	}
	// Round-trip: what we wrote must read back identically.
	m, err := ReadEnv(p)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"PLAIN": "abc", "SPACED": "a b", "EMPTY": ""} {
		if m[k] != want {
			t.Errorf("round-trip %s = %q, want %q", k, m[k], want)
		}
	}
}

func TestWriteEnv_SecretsFileIsNotWorldReadable(t *testing.T) {
	// Windows does not model Unix permission bits: os.WriteFile ignores 0o600 and
	// os.Stat reports -rw-rw-rw- whatever we pass. The 0o600 intent still matters on
	// Unix, so assert it there rather than weaken it to something Windows can pass.
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not meaningful on Windows")
	}
	p := filepath.Join(t.TempDir(), ".env")
	if _, err := WriteEnv(p, map[string]string{"TOKEN": "s3cret"}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("a .env holds secrets; mode %v grants group/other access", perm)
	}
}

// A global key name outside the env-name grammar never reaches the global
// declaration: `*` or `X_*` would be read as a pattern and `A,B` as two
// entries, declaring keys nobody imported. Nor does a
// KaTyoti variable: KTW_GLOBAL_KEYS=* from a caller would declare every key,
// KTW_NATS_URL would send every process to another hub. The file is left
// untouched.
func TestWriteGlobalEnv_RefusesANameOutsideTheGrammar(t *testing.T) {
	const body = "LEANIX_API_TOKEN=other-account\nKTW_GLOBAL_KEYS=OPENAI_*\n"
	for _, name := range []string{"*", "X_*", "A,B", "1A", "A=B", "KTW_GLOBAL_KEYS", "KTW_WORKSPACE", "KTW_UNIVERSE", "KTW_DOTENV_KEYS", "KTW_NATS_URL", "ktw_global_keys", "A\nB", ""} {
		p := envFile(t, body)
		if _, err := WriteGlobalEnv(p, map[string]string{name: "x"}, nil); err == nil {
			t.Errorf("value %q: written", name)
		}
		if _, err := WriteGlobalEnv(p, nil, []string{name}); err == nil {
			t.Errorf("declare %q: written", name)
		}
		if got := read(t, p); got != body {
			t.Errorf("%q: the file changed to %q", name, got)
		}
	}
	p := envFile(t, body)
	if _, err := WriteGlobalEnv(p, map[string]string{"SHARED_URL": "https://x.example"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := read(t, p); !strings.Contains(got, "KTW_GLOBAL_KEYS=OPENAI_*,SHARED_URL\n") {
		t.Fatalf("a valid name was not declared: %q", got)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(p), "*.tmp")); len(left) != 0 {
		t.Fatalf("the atomic write left %v", left)
	}
}

// A value holding a line break is refused by both writers, nothing written:
// `x\nKTW_GLOBAL_KEYS=*` would put its own line in the file - the loader keeps
// the first occurrence, so every global key would be declared - and
// `x\nKTW_NATS_URL=...` would move every process to another hub
func TestWriters_RefuseAValueHoldingALineBreak(t *testing.T) {
	const body = "# keep me\nSHARED_URL=https://x.example\nKTW_GLOBAL_KEYS=SHARED_URL\n"
	values := []string{"x\nKTW_GLOBAL_KEYS=*", "x\nKTW_NATS_URL=nats://evil.invalid:4222", "x\rKTW_NATS_URL=nats://evil.invalid:4222", "x\r\nKTW_DATA=/tmp", "\n"}
	for _, v := range values {
		for name, write := range map[string]func(string, map[string]string) ([]string, error){
			"universe": WriteEnv,
			"global":   func(p string, m map[string]string) ([]string, error) { return WriteGlobalEnv(p, m, nil) },
		} {
			for _, start := range []string{body, ""} {
				p := envFile(t, start)
				_, err := write(p, map[string]string{"LEANIX_API_TOKEN": v, "OTHER": "fine"})
				if err == nil || !strings.Contains(err.Error(), "line break") {
					t.Errorf("%s .env, value %q: err = %v, want a line break refused", name, v, err)
				}
				if start == "" {
					if _, serr := os.Stat(p); !os.IsNotExist(serr) {
						t.Errorf("%s .env, value %q: a file was created", name, v)
					}
				} else if got := read(t, p); got != start {
					t.Errorf("%s .env, value %q: the file changed to %q", name, v, got)
				}
			}
		}
	}
}

// A key name is bounded (MaxKeyNameLen): a name is never a payload.
func TestKeyNames_AreBounded(t *testing.T) {
	long := strings.Repeat("A", MaxKeyNameLen+1)
	if err := CheckKeyName(long); err == nil {
		t.Errorf("a %d-character name passed CheckKeyName", len(long))
	}
	if err := CheckKeyName(long[:MaxKeyNameLen]); err != nil {
		t.Errorf("a %d-character name was refused: %v", MaxKeyNameLen, err)
	}
	p := envFile(t, "")
	if _, err := WriteEnv(p, map[string]string{long: "x"}); err == nil {
		t.Errorf("WriteEnv wrote a %d-character name", len(long))
	}
	if _, err := WriteEnv(p, map[string]string{"A\nKTW_NATS_URL": "x"}); err == nil {
		t.Errorf("WriteEnv wrote a name holding a line break")
	}
}
