// SPDX-License-Identifier: Apache-2.0

// Package secretscan holds the secret scanner's checks (
// ), shared by the secret-scan command (a staged change, a
// commit, a folder) and by every program that must refuse to ship a secret
// (compose's marketplace build scans the release artefacts with ScanTree).
//
// No list is defined here: the secret paths come from secrets.IsSecretPath,
// the credential shapes from internal/core/redact's patterns, and a deny-list
// from the file its caller names (LoadDenyList).
package secretscan

import (
	_ "embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/redact"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/secrets"
)

// FakeMarker allows a test fixture's secret-shaped line.
const FakeMarker = "secret-scan: fake"

// Finding is one secret found.
type Finding struct {
	Path string
	Line int // 0 for a secret path or a file name
	Kind string
}

func (f Finding) String() string {
	if f.Line == 0 {
		return fmt.Sprintf("%s: %s", f.Path, f.Kind)
	}
	return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Kind)
}

// Shapes is the content check: it answers the kind of the first credential
// shape in a line, "" when none.
type Shapes func(line string) string

var marker = regexp.MustCompile(`\[REDACTED:([^\]]+)\]`)

// NewShapes builds the content check from redact's built-in patterns only
// (redact.NewShapesOnly): the environment's values do not count, and the
// process environment is neither read nor changed - the check is built inside
// the running ktw-compose sidecar.
func NewShapes() Shapes {
	r := redact.NewShapesOnly(redact.RedactionSettings{})
	return func(line string) string {
		out := r.RedactString(line)
		if out == line {
			return ""
		}
		for _, m := range marker.FindAllStringSubmatch(out, -1) {
			if strings.Count(out, m[0]) > strings.Count(line, m[0]) {
				return m[1]
			}
		}
		return "secret"
	}
}

// ListLine answers a list line's meaning, the one reading of the list format
// (tools/oss-export/deny-list.txt, read here and by the publication gate): trimmed, a
// trailing « # reason » (a # after a space or a tab) cut; "" for a blank or a # line. A
// deny-list pattern that needs a literal # after a blank writes it [#].
func ListLine(line string) string {
	l := strings.TrimSpace(line)
	if strings.HasPrefix(l, "#") {
		return ""
	}
	for i := 1; i < len(l); i++ {
		if l[i] == '#' && (l[i-1] == ' ' || l[i-1] == '\t') {
			return strings.TrimSpace(l[:i])
		}
	}
	return l
}

// LoadDenyList reads a deny-list file: one case-insensitive regular expression per
// line, each line read by ListLine (a leading BOM stripped). An error names a bad line
// by its number alone, never its pattern.
func LoadDenyList(path string) ([]*regexp.Regexp, error) {
	deny, _, err := LoadDenyListRefs(path)
	return deny, err
}

// LoadDenyListRefs is LoadDenyList with each pattern's reference - « deny-list.txt
// line 6 », the file's base name and the line - keyed by the compiled expression's
// String(): what a report prints in place of the term (RefDenyTerms).
func LoadDenyListRefs(path string) ([]*regexp.Regexp, map[string]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("deny-list: %w", err)
	}
	var out []*regexp.Regexp
	refs := map[string]string{}
	for n, line := range strings.Split(strings.TrimPrefix(string(b), string(rune(0xfeff))), "\n") {
		l := ListLine(line)
		if l == "" {
			continue
		}
		re, err := regexp.Compile("(?i)" + l)
		if err != nil {
			return nil, nil, fmt.Errorf("deny-list %s:%d: not a valid regular expression", path, n+1)
		}
		out = append(out, re)
		if _, ok := refs[re.String()]; !ok {
			refs[re.String()] = fmt.Sprintf("%s line %d", filepath.Base(path), n+1)
		}
	}
	return out, refs, nil
}

// RefDenyTerms answers the findings with each deny-list term replaced by its reference
// (LoadDenyListRefs): a report never prints a private term in clear.
func RefDenyTerms(findings []Finding, refs map[string]string) []Finding {
	out := make([]Finding, len(findings))
	for i, f := range findings {
		if pattern, ok := strings.CutPrefix(f.Kind, denyKind); ok {
			if r, ok := refs["(?i)"+pattern]; ok {
				f.Kind = denyKind + r
			} else {
				f.Kind = denyKind + "(a deny-list line)"
			}
		}
		out[i] = f
	}
	return out
}

// denyKind prefixes a deny-list finding's kind; the term follows.
const denyKind = "deny-list term "

// ScanTree scans every regular file under root (or root itself when it is a
// file): a path the secret-path list names, a file name holding a deny-list
// term, and each line of its bytes - a binary file's printable runs included,
// so a key behind a NUL byte is seen - holding a credential shape or a
// deny-list term. Findings name the path relative to root (the base name
// when root is a file), with slashes, one per path, line and kind.
func ScanTree(root string, shapes Shapes, deny []*regexp.Regexp) ([]Finding, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	d := newDenyList(deny)
	type file struct{ path, rel string }
	var files []file
	if !info.IsDir() {
		files = append(files, file{root, filepath.Base(root)})
	} else if err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil || !e.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		files = append(files, file{path, filepath.ToSlash(rel)})
		return err
	}); err != nil {
		return nil, err
	}
	// The files are scanned in parallel: a release holds a gigabyte of
	// binaries.
	found := make([][]Finding, len(files))
	errs := make([]error, len(files))
	next := make(chan int)
	var wg sync.WaitGroup
	for range runtime.NumCPU() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				f := files[i]
				var out []Finding
				if secrets.IsSecretPath(f.rel) {
					out = append(out, Finding{Path: f.rel, Kind: "secret path"})
				}
				for _, term := range d.terms(f.rel) {
					out = append(out, Finding{Path: f.rel, Kind: denyKind + term})
				}
				b, err := os.ReadFile(f.path)
				if err != nil {
					errs[i] = err
					continue
				}
				found[i] = append(out, scanBytes(f.rel, b, shapes, d)...)
			}
		}()
	}
	for i := range files {
		next <- i
	}
	close(next)
	wg.Wait()
	var out []Finding
	for i := range files {
		if errs[i] != nil {
			return nil, errs[i]
		}
		out = append(out, found[i]...)
	}
	return out, nil
}

// denyList names the terms a text holds.
type denyList struct{ each []*regexp.Regexp }

func newDenyList(deny []*regexp.Regexp) denyList { return denyList{each: deny} }

func (d denyList) terms(s string) []string {
	var out []string
	for _, re := range d.each {
		if re.MatchString(s) {
			out = append(out, strings.TrimPrefix(re.String(), "(?i)"))
		}
	}
	return out
}

// scanBytes checks each printable run of a file's bytes - a text file's
// lines, a binary file's strings - for a credential shape (a fixture line
// carrying FakeMarker passes) and the deny-list's terms, numbering the lines
// by the newlines.
func scanBytes(rel string, b []byte, shapes Shapes, deny denyList) []Finding {
	var out []Finding
	seen := map[Finding]bool{}
	add := func(f Finding) {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	check := func(run string, line int) {
		if len(run) < 3 || strings.Contains(run, FakeMarker) {
			return
		}
		if kind := shapes(run); kind != "" {
			add(Finding{Path: rel, Line: line, Kind: kind})
		}
	}
	checkDeny := func(run string, line int) {
		for _, term := range deny.terms(run) {
			add(Finding{Path: rel, Line: line, Kind: denyKind + term})
		}
	}
	line, start := 1, -1
	for i, c := range b {
		printable := c == '\t' || (c >= 0x20 && c != 0x7f)
		if printable {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			check(string(b[start:i]), line)
			checkDeny(string(b[start:i]), line)
			start = -1
		}
		if c == '\n' {
			line++
		}
	}
	if start >= 0 {
		check(string(b[start:]), line)
		checkDeny(string(b[start:]), line)
	}
	return out
}

// Allow lets a known finding pass: a library's documentation sample or
// message shaped like a credential inside a built binary. Glob matches the
// finding's path (path.Match, on the whole relative path or its base name),
// Kind its kind ("*" any kind).
type Allow struct {
	Glob string
	Kind string
}

// LoadAllowList reads an allow-list file: one "<glob> <kind>" per line, blank
// lines and lines starting with # (the reason each entry is there) skipped.
func LoadAllowList(file string) ([]Allow, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("allow-list: %w", err)
	}
	return parseAllowList(file, strings.TrimPrefix(string(b), "\ufeff")) // a leading BOM stripped
}

func parseAllowList(file, text string) ([]Allow, error) {
	var out []Allow
	for n, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		glob, kind, ok := strings.Cut(l, " ")
		kind = strings.TrimSpace(kind)
		if !ok || kind == "" {
			return nil, fmt.Errorf("allow-list %s:%d: want \"<glob> <kind>\"", file, n+1)
		}
		if _, err := path.Match(glob, ""); err != nil {
			return nil, fmt.Errorf("allow-list %s:%d: %w", file, n+1, err)
		}
		out = append(out, Allow{Glob: glob, Kind: kind})
	}
	return out, nil
}

// Filter answers the findings no allow entry lets pass.
func Filter(findings []Finding, allow []Allow) []Finding {
	var out []Finding
	for _, f := range findings {
		allowed := false
		for _, a := range allow {
			if a.Kind != "*" && a.Kind != f.Kind {
				continue
			}
			whole, _ := path.Match(a.Glob, f.Path)
			base, _ := path.Match(a.Glob, path.Base(f.Path))
			if whole || base {
				allowed = true
				break
			}
		}
		if !allowed {
			out = append(out, f)
		}
	}
	return out
}

//go:embed release-allow.txt
var releaseAllow string

// ReleaseAllow answers the allow-list of KaTyoti's own binaries: the library
// samples and messages a release scan lets pass.
func ReleaseAllow() []Allow {
	out, err := parseAllowList("release-allow.txt", releaseAllow)
	if err != nil {
		panic(err)
	}
	return out
}
