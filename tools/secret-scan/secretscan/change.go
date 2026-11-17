// SPDX-License-Identifier: Apache-2.0

package secretscan

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/secrets"
)

// THE CHANGE SCAN - what a change would
// commit: its paths (a secret path is refused) and the lines it adds (a
// credential shape is refused; a line carrying FakeMarker passes). The
// secret-scan command and the pre-commit hook read the change from git
// (Change, ParseAdded, Scan); a program that writes git objects itself - the
// scheduler's export - scans the contents it is about to commit in process
// (ScanFiles), before it moves a ref.

// AddedLine is one line the change adds.
type AddedLine struct {
	Path string
	Line int
	Text string
}

// Scan answers the findings of a change: its staged paths and added lines.
func Scan(paths []string, added []AddedLine, shapes Shapes) []Finding {
	var out []Finding
	for _, p := range paths {
		if secrets.IsSecretPath(p) {
			out = append(out, Finding{Path: p, Kind: "secret path"})
		}
	}
	for _, a := range added {
		if strings.Contains(a.Text, FakeMarker) {
			continue
		}
		if kind := shapes(a.Text); kind != "" {
			out = append(out, Finding{Path: a.Path, Line: a.Line, Kind: kind})
		}
	}
	return out
}

// ScanFiles answers the findings of contents about to be committed, by path:
// a secret path, and in each content - binary bytes read as their printable
// runs - a credential shape. Findings come by path, then line.
func ScanFiles(files map[string][]byte, shapes Shapes) []Finding {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	var out []Finding
	for _, p := range paths {
		if secrets.IsSecretPath(p) {
			out = append(out, Finding{Path: p, Kind: "secret path"})
		}
		out = append(out, scanBytes(p, files[p], shapes, denyList{})...)
	}
	return out
}

var hunk = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// ParseAdded reads the added lines of a unified diff (-U0, no colour, paths
// unquoted). A hunk's header counts its added lines, so an added line that
// looks like a header ("+++ x") is still read as content.
func ParseAdded(diff string) []AddedLine {
	var out []AddedLine
	path, next, left := "", 0, 0
	sc := bufio.NewScanner(strings.NewReader(diff))
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		l := sc.Text()
		if left > 0 && strings.HasPrefix(l, "+") {
			if path != "" {
				out = append(out, AddedLine{Path: path, Line: next, Text: l[1:]})
			}
			next++
			left--
			continue
		}
		switch {
		case strings.HasPrefix(l, "diff --git "):
			path, next, left = "", 0, 0
		case strings.HasPrefix(l, "+++ "):
			path = strings.TrimPrefix(strings.TrimPrefix(l, "+++ "), "b/")
			if path == "/dev/null" {
				path = ""
			}
		case strings.HasPrefix(l, "@@ "):
			if m := hunk.FindStringSubmatch(l); m != nil {
				next, _ = strconv.Atoi(m[1])
				left = 1
				if m[2] != "" {
					left, _ = strconv.Atoi(m[2])
				}
			}
		}
	}
	return out
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "core.quotePath=false"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil && stderr.Len() > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return string(b), err
}

// Change reads a change from the repository at dir: the staged one, or what
// commit added against its first parent when commit is set. It answers the
// paths added, copied, modified or renamed and the diff of their added lines,
// binary files read as text (--text) and every file as its stored bytes
// (--no-textconv): a textconv driver from .gitattributes (Git for Windows'
// astextplain on *.pdf, *.docx) is never run, so a missing converter cannot
// fail the scan and a converter cannot hide a key. It asks git for the cached
// diff, so it honours GIT_INDEX_FILE.
func Change(dir, commit string) (paths []string, diff string, err error) {
	base := []string{"diff", "--cached", "--no-color", "--no-ext-diff", "--no-textconv", "--diff-filter=ACMR"}
	if commit != "" {
		base = []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--diff-filter=ACMR", commit + "^", commit}
	}
	names, err := git(dir, append(append([]string{}, base...), "--name-only", "-z")...)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read the staged paths: %w", err)
	}
	diff, err = git(dir, append(append([]string{}, base...), "--text", "-U0")...)
	if err != nil {
		return nil, "", fmt.Errorf("cannot read the staged change: %w", err)
	}
	for _, p := range strings.Split(names, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, diff, nil
}
