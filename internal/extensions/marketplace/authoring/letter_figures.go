// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// A WRITTEN LETTER'S FIGURES MATCH ITS CHANGE LOGS (« totals
// and values »): a solution's « Quoi de neuf » written for its users states,
// in its « En chiffres » section, how many changes it covers, how many
// requests, the changes by value and a table by theme. At build time those
// figures are counted again from the change logs the letter covers - its
// version's and the KaTyoti version it ships - and the build fails naming
// each figure that differs, so the letter never drifts from them.

// LetterFigures are the figures of a letter, or counted from change logs. A
// figure the letter does not state is -1 and is not checked.
type LetterFigures struct {
	Changes, Requests                   int
	Major, Notable, Improvements, Fixes int
	Unclassed                           int
	ThemeTotal, ThemeSum                int
}

var (
	figChanges  = regexp.MustCompile(`\*\*(\d+)\*\*\s+changements?\b`)
	figRequests = regexp.MustCompile(`\*\*(\d+)\*\*\s+demandes?\b`)
	figValues   = regexp.MustCompile(`\*\*(\d+)\*\*\s+changements? majeurs?,\s*\*\*(\d+)\*\*\s+notables?,\s*\*\*(\d+)\*\*\s+améliorations?,\s*\*\*(\d+)\*\*\s+corrections?(?:\s*\((\d+)\s+non class[ée]s?\))?`)
	figRow      = regexp.MustCompile(`^\|\s*(.+?)\s*\|\s*(\**)(\d+)(\**)\s*\|\s*$`)
	changeEntry = regexp.MustCompile(`^### \d+\.`)
)

// ParseLetterFigures reads a letter's « En chiffres » section; ok is false
// when the letter has none (nothing is then checked).
func ParseLetterFigures(md string) (LetterFigures, bool) {
	f := LetterFigures{Changes: -1, Requests: -1, Major: -1, Notable: -1, Improvements: -1, Fixes: -1, Unclassed: -1, ThemeTotal: -1, ThemeSum: -1}
	in, found := false, false
	sum, rows := 0, 0
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "## ") {
			in = strings.TrimSpace(strings.TrimPrefix(line, "## ")) == "En chiffres"
			found = found || in
			continue
		}
		if !in {
			continue
		}
		if m := figValues.FindStringSubmatch(line); m != nil {
			f.Major, f.Notable, f.Improvements, f.Fixes = atoi(m[1]), atoi(m[2]), atoi(m[3]), atoi(m[4])
			f.Unclassed = 0
			if m[5] != "" {
				f.Unclassed = atoi(m[5])
			}
			continue
		}
		if m := figChanges.FindStringSubmatch(line); m != nil && f.Changes < 0 {
			f.Changes = atoi(m[1])
		}
		if m := figRequests.FindStringSubmatch(line); m != nil {
			f.Requests = atoi(m[1])
		}
		if m := figRow.FindStringSubmatch(line); m != nil {
			label := strings.Trim(m[1], "* ")
			n := atoi(m[3])
			if strings.EqualFold(label, "Total") {
				f.ThemeTotal = n
			} else {
				sum += n
				rows++
			}
		}
	}
	if rows > 0 {
		f.ThemeSum = sum
	}
	return f, found
}

// CountChangelogFigures counts the change logs' entries and their values:
// ★★★ majeur, ★★ notable, ★ amélioration, 🛠 correction, any other (or none)
// non classé.
func CountChangelogFigures(paths []string) (LetterFigures, error) {
	f := LetterFigures{Requests: -1, ThemeTotal: -1, ThemeSum: -1}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return f, fmt.Errorf("the change log %s: %w", p, err)
		}
		inEntry, valued := false, false
		closeEntry := func() {
			if inEntry && !valued {
				f.Unclassed++
			}
		}
		for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
			switch {
			case changeEntry.MatchString(line):
				closeEntry()
				f.Changes++
				inEntry, valued = true, false
			case strings.HasPrefix(line, "## "):
				closeEntry()
				inEntry = false
			case inEntry && !valued && strings.HasPrefix(line, "**Valeur :**"):
				valued = true
				v := strings.TrimSpace(strings.TrimPrefix(line, "**Valeur :**"))
				switch {
				case strings.HasPrefix(v, "★★★"):
					f.Major++
				case strings.HasPrefix(v, "★★"):
					f.Notable++
				case strings.HasPrefix(v, "★"):
					f.Improvements++
				case strings.HasPrefix(v, "🛠"):
					f.Fixes++
				default:
					f.Unclassed++
				}
			}
		}
		closeEntry()
	}
	return f, nil
}

// CheckLetterFigures compares a letter's figures with those counted from the
// change logs it covers (and requests, the requests their changes name, when
// known: -1 skips it); it answers each figure that differs, or nil.
func CheckLetterFigures(letter string, covers []string, requests int) error {
	got, ok := ParseLetterFigures(letter)
	if !ok || len(covers) == 0 {
		return nil
	}
	want, err := CountChangelogFigures(covers)
	if err != nil {
		return err
	}
	want.Requests = requests
	var diffs []string
	cmp := func(name string, letterN, countN int) {
		if letterN >= 0 && countN >= 0 && letterN != countN {
			diffs = append(diffs, fmt.Sprintf("%s: the letter says %d, the change logs %d", name, letterN, countN))
		}
	}
	cmp("changes", got.Changes, want.Changes)
	cmp("requests", got.Requests, want.Requests)
	cmp("majeurs", got.Major, want.Major)
	cmp("notables", got.Notable, want.Notable)
	cmp("améliorations", got.Improvements, want.Improvements)
	cmp("corrections", got.Fixes, want.Fixes)
	cmp("non classés", got.Unclassed, want.Unclassed)
	cmp("theme total", got.ThemeTotal, want.Changes)
	if got.ThemeSum >= 0 && got.ThemeTotal >= 0 && got.ThemeSum != got.ThemeTotal {
		diffs = append(diffs, fmt.Sprintf("the theme table: its rows sum to %d, its total says %d", got.ThemeSum, got.ThemeTotal))
	}
	if len(diffs) > 0 {
		return fmt.Errorf("the written « Quoi de neuf » does not match its change logs (%s):\n  %s",
			strings.Join(covers, ", "), strings.Join(diffs, "\n  "))
	}
	return nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
