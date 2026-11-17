// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// CheckCutVersion is whether a new cut may publish version into the marketplace
// published at outDir: refused when an item of its catalog already carries it,
// or when it is not above the highest version the catalog carries. It answers
// that highest version ("" for an empty or absent catalog), which the cut
// names. A version that is not numbered (a git revision, a word) is only
// checked for being already published: it has no order.
func CheckCutVersion(outDir, version string) (last string, err error) {
	version = strings.TrimSpace(version)
	cat, err := loadCatalogFile(filepath.Join(outDir, "katyoti-marketplace.yaml"))
	if err != nil {
		return "", err
	}
	return checkCutVersion(cat, version)
}

func checkCutVersion(cat mktapi.Catalog, version string) (last string, err error) {
	var published []string
	for _, it := range cat.Items {
		for _, v := range it.Versions {
			if v.Version == version {
				return version, fmt.Errorf("%s is already published (%s@%s): a published version is immutable - cut a new version", version, it.Name, version)
			}
			published = append(published, v.Version)
		}
	}
	for _, p := range published {
		if _, ok := parseCutVersion(p); !ok {
			continue
		}
		if last == "" || CompareCutVersions(p, last) > 0 {
			last = p
		}
	}
	if last == "" {
		return "", nil
	}
	if _, ok := parseCutVersion(version); !ok {
		return last, nil
	}
	if c := CompareCutVersions(version, last); c <= 0 {
		what := "lower than"
		if c == 0 {
			what = "the same as"
		}
		return last, fmt.Errorf("%s is %s %s, the last version the marketplace published: cut a higher version", version, what, last)
	}
	return last, nil
}

// cutVersion is a version split for ordering: its numbers ("v1.003" → 1, 3),
// then its pre-release identifiers ("rc03"), which put it below the same
// numbers without one.
type cutVersion struct {
	nums []int
	pre  []string
}

// parseCutVersion reads "v1.2.3", "1.003", "2026.10", "1.003-rc1",
// "v1.003.rc03" or "1.2.3+build": numbers separated by dots, then anything
// else as the pre-release (dots, dashes; a build suffix after + ignored). A
// version that does not start with a number is not ordered.
func parseCutVersion(v string) (cutVersion, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	var out cutVersion
	parts := strings.FieldsFunc(v, func(r rune) bool { return r == '.' || r == '-' })
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || len(out.pre) > 0 {
			if i == 0 {
				return cutVersion{}, false
			}
			out.pre = append(out.pre, p)
			continue
		}
		out.nums = append(out.nums, n)
	}
	return out, len(out.nums) > 0
}

// CompareCutVersions orders two numbered versions: number by number (a
// missing number is 0), then a pre-release below the release, then the
// pre-release identifiers ("rc2" below "rc10"). Unnumbered versions compare
// as text.
func CompareCutVersions(a, b string) int {
	pa, oka := parseCutVersion(a)
	pb, okb := parseCutVersion(b)
	if !oka || !okb {
		return strings.Compare(a, b)
	}
	for i := 0; i < len(pa.nums) || i < len(pb.nums); i++ {
		x, y := numAt(pa.nums, i), numAt(pb.nums, i)
		if x != y {
			return signOf(x - y)
		}
	}
	switch {
	case len(pa.pre) == 0 && len(pb.pre) == 0:
		return 0
	case len(pa.pre) == 0:
		return 1
	case len(pb.pre) == 0:
		return -1
	}
	for i := 0; i < len(pa.pre) && i < len(pb.pre); i++ {
		if c := comparePre(pa.pre[i], pb.pre[i]); c != 0 {
			return c
		}
	}
	return signOf(len(pa.pre) - len(pb.pre))
}

// comparePre orders one pre-release identifier: its letters as text, then its
// number as a number ("rc03" = "rc3" < "rc10").
func comparePre(a, b string) int {
	la, na := splitPre(a)
	lb, nb := splitPre(b)
	if c := strings.Compare(strings.ToLower(la), strings.ToLower(lb)); c != 0 {
		return c
	}
	if na >= 0 && nb >= 0 {
		return signOf(na - nb)
	}
	return strings.Compare(a, b)
}

func splitPre(s string) (string, int) {
	i := strings.IndexFunc(s, unicode.IsDigit)
	if i < 0 {
		return s, -1
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil {
		return s, -1
	}
	return s[:i], n
}

func numAt(nums []int, i int) int {
	if i < len(nums) {
		return nums[i]
	}
	return 0
}

func signOf(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
