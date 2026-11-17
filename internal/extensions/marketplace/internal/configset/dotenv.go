// SPDX-License-Identifier: Apache-2.0

package configset

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// The `.env` files this touches are SHARED with whatever the user already put
// there, so every write is a read-modify-write of just the named keys. A rewrite
// would silently drop unrelated settings — the kind of loss nobody notices until
// something else stops working.

// keyName is the grammar of a key name: an environment variable name. A name
// outside it - `*`, `LEANIX_*`, `A,B`, one holding `=` or a newline - would
// be read as a pattern or a list once written into the global declaration
// (cenv.GlobalKeysVar), declaring keys nobody imported.
var keyName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// MaxKeyNameLen bounds a key name: an environment variable name, never a
// payload.
const MaxKeyNameLen = 128

// ValidKeyName reports whether name is an environment variable name: letters,
// digits and _, not starting with a digit, at most MaxKeyNameLen long.
func ValidKeyName(name string) bool {
	return len(name) <= MaxKeyNameLen && keyName.MatchString(name)
}

// CheckKeyName refuses a name a key set may not import: one outside the key
// grammar (ValidKeyName), or one of KaTyoti's own variables (cenv.ReservedKey:
// KTW_GLOBAL_KEYS=* would declare every global key, KTW_NATS_URL or KTW_DATA
// would send every process elsewhere).
func CheckKeyName(name string) error {
	if len(name) > MaxKeyNameLen {
		return fmt.Errorf("key %.32q... is %d characters long: a key name has at most %d", name, len(name), MaxKeyNameLen)
	}
	if !ValidKeyName(name) {
		return fmt.Errorf("key %q is not an environment variable name (letters, digits and _, not starting with a digit)", name)
	}
	if cenv.ReservedKey(name) {
		return fmt.Errorf("key %q is reserved: %s* names are KaTyoti's own variables, never an imported key", name, cenv.ReservedKeyPrefix)
	}
	return nil
}

// CheckKeyValue refuses a value a .env line cannot hold: one with a line
// break (\n or \r). Written, it would end its line and start another -
// `x\nKTW_GLOBAL_KEYS=*` declares every global key, `x\nKTW_NATS_URL=...`
// sends every process elsewhere - and quoting does not help: ReadEnv and the
// loader (cenv) never undo an escape. The value itself is never echoed (it
// may be a secret).
func CheckKeyValue(name, value string) error {
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("the value of key %q holds a line break: a .env value is one line", name)
	}
	return nil
}

// ReadEnv parses a `.env` into a key→value map. A missing file is an empty map,
// not an error: collecting keys into a universe that has none yet is the normal
// first-run case.
func ReadEnv(path string) (map[string]string, error) {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := parseEnvLine(line)
		if ok {
			out[k] = v
		}
	}
	return out, nil
}

// parseEnvLine returns the key/value of a `KEY=VALUE` line, or ok=false for a
// comment, a blank line, or anything without a `=`.
func parseEnvLine(line string) (key, value string, ok bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") {
		return "", "", false
	}
	t = strings.TrimPrefix(t, "export ")
	k, v, found := strings.Cut(t, "=")
	if !found {
		return "", "", false
	}
	k = strings.TrimSpace(k)
	if k == "" {
		return "", "", false
	}
	return k, unquote(strings.TrimSpace(v)), true
}

func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

// needsQuoting reports whether a value must be quoted to survive a round-trip.
func needsQuoting(v string) bool {
	return v == "" || strings.ContainsAny(v, " \t\"'#$")
}

// WriteEnv merges values into the `.env` at path, IN PLACE:
//   - a key already present is updated where it sits (its position, and any
//     comment above it, are preserved);
//   - a new key is appended;
//   - every other line — comments, blank lines, unrelated keys, ordering — is
//     kept byte-for-byte.
//
// Returns the keys it actually changed, so callers can report honestly rather
// than claiming to have written values that were already correct.
func WriteEnv(path string, values map[string]string) ([]string, error) {
	return writeEnv(path, values, false)
}

// writeEnv is WriteEnv; atomic writes through a temporary file renamed over
// path (writeFileAtomic).
func writeEnv(path string, values map[string]string, atomic bool) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	// Every line written is one KEY=VALUE line: a name outside the grammar or
	// a value with a line break is refused before anything is written.
	for k, v := range values {
		if !ValidKeyName(k) {
			return nil, fmt.Errorf("key %.64q is not an environment variable name (letters, digits and _, not starting with a digit, at most %d): nothing written to %s", k, MaxKeyNameLen, path)
		}
		if err := CheckKeyValue(k, v); err != nil {
			return nil, fmt.Errorf("%w: nothing written to %s", err, path)
		}
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var lines []string
	if len(existing) > 0 {
		lines = strings.Split(strings.ReplaceAll(string(existing), "\r\n", "\n"), "\n")
	}

	var changed []string
	seen := map[string]bool{}
	for i, line := range lines {
		k, v, ok := parseEnvLine(line)
		if !ok {
			continue
		}
		newV, want := values[k]
		if !want {
			continue
		}
		seen[k] = true
		if v == newV {
			continue // already correct — do not rewrite, do not report
		}
		lines[i] = formatEnvLine(k, newV)
		changed = append(changed, k)
	}

	for k, v := range values {
		if seen[k] {
			continue
		}
		lines = append(lines, formatEnvLine(k, v))
		changed = append(changed, k)
	}
	if len(changed) == 0 {
		return nil, nil
	}

	// Keep exactly one trailing newline.
	body := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if atomic {
		if err := writeFileAtomic(path, []byte(body)); err != nil {
			return nil, err
		}
		return changed, nil
	}
	// 0o600: these files hold secrets. WriteFile keeps an existing file's mode,
	// so a .env created readable by others is tightened here.
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return nil, fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return nil, fmt.Errorf("chmod %s: %w", path, err)
	}
	return changed, nil
}

// writeFileAtomic writes body to a temporary file in path's folder, then
// renames it over path, so a process starting meanwhile reads the old file
// or the new one, never a short one. 0o600: the file holds secrets.
func writeFileAtomic(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	name := tmp.Name()
	defer os.Remove(name) // a no-op once renamed
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		_ = tmp.Close()
		return fmt.Errorf("chmod %s: %w", name, err)
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// WriteGlobalEnv is WriteEnv for the global .env: it writes values and
// declares global (cenv.GlobalKeysVar, in the same file) every name of values
// and of declare the declaration does not name yet. The global .env fills a
// universe's process only with the keys it declares: a key
// written there as scope global and left undeclared is never seen by any
// member. declare names keys already in the file that must be seen too (a key
// set's global key imported earlier, before it was declared). The keys it
// returns are those written or newly declared - each now reaches the members.
// A name CheckKeyName refuses is refused, nothing written: in the declaration
// `*` or `A,B` would declare keys nobody named, and a caller never writes the
// declaration itself (KTW_GLOBAL_KEYS) - only WriteGlobalEnv does. The file is
// replaced atomically, so a process starting meanwhile never reads it short.
func WriteGlobalEnv(path string, values map[string]string, declare []string) ([]string, error) {
	for k := range values {
		if err := CheckKeyName(k); err != nil {
			return nil, fmt.Errorf("global %w", err)
		}
	}
	for _, k := range declare {
		if err := CheckKeyName(k); err != nil {
			return nil, fmt.Errorf("global %w", err)
		}
	}
	cur, err := ReadEnv(path)
	if err != nil {
		return nil, err
	}
	decl := cur[cenv.GlobalKeysVar]
	names := map[string]bool{}
	for k := range values {
		names[k] = true
	}
	for _, k := range declare {
		if _, ok := cur[k]; ok || values[k] != "" {
			names[k] = true
		}
	}
	var added []string
	for k := range names {
		if k != cenv.GlobalKeysVar && !cenv.GlobalKeyDeclared(decl, k) {
			added = append(added, k)
		}
	}
	sort.Strings(added)
	writes := values
	if len(added) > 0 {
		writes = make(map[string]string, len(values)+1)
		for k, v := range values {
			writes[k] = v
		}
		parts := []string{}
		if strings.TrimSpace(decl) != "" {
			parts = append(parts, strings.TrimSpace(decl))
		}
		writes[cenv.GlobalKeysVar] = strings.Join(append(parts, added...), ",")
	}
	changed, err := writeEnv(path, writes, true)
	if err != nil {
		return nil, err
	}
	out := []string{}
	seen := map[string]bool{}
	for _, k := range append(changed, added...) {
		if k != cenv.GlobalKeysVar && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func formatEnvLine(k, v string) string {
	if needsQuoting(v) {
		return fmt.Sprintf("%s=%q", k, v)
	}
	return k + "=" + v
}
