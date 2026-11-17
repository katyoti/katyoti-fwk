// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// Versions order by their numbers, a pre-release below its release, the
// pre-release's number as a number.
func TestCutVersionsOrderAsTheyAreNumbered(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"1.004", "1.003", 1},
		{"1.010", "1.009", 1},
		{"v1.003", "1.003", 0},
		{"1.003", "1.3", 0},
		{"2026.10", "2026.09", 1},
		{"2026.10", "1.008", 1},
		{"1.003", "v1.003.rc03", 1},
		{"1.003-rc2", "1.003-rc10", -1},
		{"v1.003.rc03", "v1.003.rc3", 0},
		{"1.004-rc1", "1.003", 1},
		{"v1.1.0", "v1.0.9", 1},
		{"1.2.3+abc", "1.2.3", 0},
	} {
		if got := CompareCutVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareCutVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := CompareCutVersions(c.b, c.a); got != -c.want {
			t.Errorf("CompareCutVersions(%q, %q) = %d, want %d", c.b, c.a, got, -c.want)
		}
	}
}

func catalogOf(versions ...string) mktapi.Catalog {
	var vs []mktapi.ItemVersion
	for _, v := range versions {
		vs = append(vs, mktapi.ItemVersion{Version: v})
	}
	return mktapi.Catalog{Items: []mktapi.Item{{Name: "ktw-agents", Versions: vs}, {Name: "seed", Versions: []mktapi.ItemVersion{{Version: "proof-n1"}}}}}
}

// A cut is refused when its version is already published or not above the
// last one; it names the last one.
func TestACutVersionIsNewAndAboveTheLastPublished(t *testing.T) {
	cat := catalogOf("1.005", "1.004-rc2", "1.004")
	for _, c := range []struct {
		version, refusal string
	}{
		{"1.005", "already published"},
		{"1.004", "already published"},
		{"1.003", "lower than 1.005"},
		{"1.005-rc1", "lower than 1.005"},
		{"v1.005", "the same as 1.005"},
		{"proof-n1", "already published"},
		{"1.006", ""},
		{"1.006-rc1", ""},
		{"2026.10", ""},
		{"proof-n2", ""},
	} {
		last, err := checkCutVersion(cat, c.version)
		switch {
		case c.refusal == "" && err != nil:
			t.Errorf("%s refused: %v", c.version, err)
		case c.refusal != "" && (err == nil || !strings.Contains(err.Error(), c.refusal)):
			t.Errorf("%s: want a refusal saying %q, got %v", c.version, c.refusal, err)
		}
		if c.refusal == "" && last != "1.005" {
			t.Errorf("%s: the last published is %q, want 1.005", c.version, last)
		}
	}
}

// Read from the published tree: an absent catalog takes any first version, a
// published one refuses its versions.
func TestCheckCutVersionReadsThePublishedCatalog(t *testing.T) {
	dir := t.TempDir()
	if last, err := CheckCutVersion(dir, "1.000"); err != nil || last != "" {
		t.Fatalf("a first cut: last=%q err=%v", last, err)
	}
	body := "apiVersion: 1\nmarketplace: {name: team}\nitems:\n  - name: ktw-agents\n    versions:\n      - version: \"1.002\"\n      - version: \"1.001\"\n"
	if err := os.WriteFile(filepath.Join(dir, "katyoti-marketplace.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CheckCutVersion(dir, "1.001"); err == nil || !strings.Contains(err.Error(), "already published") {
		t.Fatalf("a published version was let through: %v", err)
	}
	if last, err := CheckCutVersion(dir, "1.003"); err != nil || last != "1.002" {
		t.Fatalf("the next version: last=%q err=%v", last, err)
	}
}
