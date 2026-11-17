// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// errorKeyLiteral is a message key a service gives a refusal (usermsg.Wrap,
// cwmapi.Invalid, a typed error's UserMsg): "<domain>.<name>".
var errorKeyLiteral = regexp.MustCompile(`"((?:cwm|actions)\.[a-z][A-Za-z]*)"`)

// Every message key a service attaches to a refusal is in the GUI's `errors`
// namespace (English, and so — TestLocales_SameKeysInEveryLanguage — French):
// a key the GUI lacks would show the English text to a French reader.
func TestErrorKeys_EveryServiceKeyIsTranslated(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("frontend", "src", "locales", "en", "errors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, k := range flattenKeys("", tree) {
		have[k] = true
	}
	found := 0
	for _, dir := range []string{"../../internal/extensions/cwm", "../../internal/extensions/actions"} {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			src, _ := os.ReadFile(p)
			s := string(src)
			if !strings.Contains(s, "usermsg") && !strings.Contains(s, "api.Invalid(") {
				return nil
			}
			for _, m := range errorKeyLiteral.FindAllStringSubmatch(s, -1) {
				found++
				if !have[m[1]] {
					t.Errorf("%s: message key %q is not in locales/en/errors.json", filepath.ToSlash(p), m[1])
				}
			}
			return nil
		})
	}
	if found < 20 {
		t.Fatalf("only %d message keys found in the services: the scan is broken", found)
	}
}
