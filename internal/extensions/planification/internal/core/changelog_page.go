// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A release's change-log page is its head (the frontmatter, the info box, En un
// coup d'œil), one `## <domain>` section per domain holding its entries
// (`### N. title`, the badge line, the text, `---`), and its tail (Prochaine
// release, Réserves). The entries are `change` objects; the head and the tail
// are the release's (ChangeLogHead, ChangeLogTail). Two markers in the head are
// filled from the entries: the Dernière mise à jour line and the count line.

// The markers the renderer fills in a page's head.
const (
	// markUpdates stands for the dated update lines of the Dernière mise à jour
	// line: what follows it on the line is kept as written.
	markUpdates = "{{changes:updates}}"
	// markCount stands for the count line (N changements : x majeurs · …).
	markCount = "{{changes:count}}"
)

const updatesLabel = "**Dernière mise à jour :** "

// pageEntry is one entry as a page holds it.
type pageEntry struct {
	Number   int
	Label    string // its number as the page wrote it when it is not one (1.001-9)
	Title    string
	Domain   string
	Badge    string // the badge line as written, when it does not parse
	Value    string
	Maturity string
	ForWhom  string
	Text     string
	Date     string // YYYY-MM-DD, from the Dernière mise à jour line
	Update   string // its line there
}

// parsedPage is a page cut into its parts.
type parsedPage struct {
	Head, Tail string
	Entries    []pageEntry
}

var (
	// entryHeading is an entry's heading: its number, or a change of an
	// earlier release listed again (1.001-9: change 9 of 1.001), and its title.
	entryHeading = regexp.MustCompile(`^### (\d+|\d+\.\d+-\d+)\. (.+?)\s*$`)
	badgeLine    = regexp.MustCompile(`^\*\*Valeur :\*\* (.+?) · \*\*Maturité :\*\* (.+?) · \*\*Pour qui :\*\* (.+?)\s*$`)
	updateDated  = regexp.MustCompile(`^(\d{2})/(\d{2})/(\d{4}) — (changement \d+ : .*)$`)
	updateOne    = regexp.MustCompile(`^changement (\d+) : (.*)$`)
)

// valueOf reads a badge's value label (★★ Notable) as a value; "" when it is none.
func valueOf(label string) string {
	for v, l := range api.ValueLabels {
		if strings.TrimSpace(label) == l {
			return v
		}
	}
	return ""
}

// parsePage cuts a page into its head, its entries by domain and its tail. A
// `##` section holding an entry is a domain; the head is what comes before the
// first one, the tail what comes after the last.
func parsePage(md string) parsedPage {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	lines := strings.Split(md, "\n")
	type section struct {
		start, end int
		entries    bool
	}
	var secs []section
	for i, l := range lines {
		if strings.HasPrefix(l, "## ") {
			if len(secs) > 0 {
				secs[len(secs)-1].end = i
			}
			secs = append(secs, section{start: i})
		} else if len(secs) > 0 && entryHeading.MatchString(l) {
			secs[len(secs)-1].entries = true
		}
	}
	if len(secs) > 0 {
		secs[len(secs)-1].end = len(lines)
	}
	first, last := -1, -1
	for i, s := range secs {
		if s.entries {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return parsedPage{Head: md}
	}
	var p parsedPage
	p.Head = strings.Join(lines[:secs[first].start], "\n")
	if last+1 < len(secs) {
		p.Tail = strings.Join(lines[secs[last+1].start:], "\n")
	}
	for _, s := range secs[first : last+1] {
		domain := strings.TrimSpace(strings.TrimPrefix(lines[s.start], "## "))
		var cur *pageEntry
		var body []string
		flush := func() {
			if cur == nil {
				return
			}
			cur.Text = strings.TrimSpace(strings.Join(body, "\n"))
			p.Entries = append(p.Entries, *cur)
			cur, body = nil, nil
		}
		for _, l := range lines[s.start+1 : s.end] {
			if m := entryHeading.FindStringSubmatch(l); m != nil {
				flush()
				cur = &pageEntry{Title: m[2], Domain: domain}
				if n, err := strconv.Atoi(m[1]); err == nil {
					cur.Number = n
				} else {
					cur.Label = m[1]
				}
				continue
			}
			if cur == nil {
				continue
			}
			if strings.TrimSpace(l) == "---" {
				continue
			}
			if cur.Badge == "" && cur.Value == "" && len(strings.TrimSpace(strings.Join(body, ""))) == 0 && strings.HasPrefix(l, "**Valeur :**") {
				if m := badgeLine.FindStringSubmatch(l); m != nil && valueOf(m[1]) != "" {
					cur.Value, cur.Maturity, cur.ForWhom = valueOf(m[1]), m[2], m[3]
				} else {
					cur.Badge = l
				}
				continue
			}
			body = append(body, l)
		}
		flush()
	}
	p.Head = takeUpdates(p.Head, p.Entries)
	return p
}

// takeUpdates reads the head's Dernière mise à jour line: each `changement N :`
// gives entry N its date and its line, and the line becomes the marker
// followed by what is not an entry's (the release's opening note).
func takeUpdates(head string, entries []pageEntry) string {
	lines := strings.Split(head, "\n")
	for i, l := range lines {
		at := strings.Index(l, updatesLabel)
		if at < 0 {
			continue
		}
		var rest []string
		date := ""
		for _, seg := range strings.Split(l[at+len(updatesLabel):], " ; ") {
			seg = strings.TrimSpace(seg)
			if m := updateDated.FindStringSubmatch(seg); m != nil {
				date = m[3] + "-" + m[2] + "-" + m[1]
				seg = m[4]
			}
			if m := updateOne.FindStringSubmatch(seg); m != nil && date != "" {
				n, _ := strconv.Atoi(m[1])
				for j := range entries {
					if entries[j].Number == n && entries[j].Date == "" {
						entries[j].Date, entries[j].Update = date, m[2]
						break
					}
				}
				continue
			}
			rest = append(rest, seg)
		}
		line := l[:at+len(updatesLabel)] + markUpdates
		if len(rest) > 0 {
			line += " ; " + strings.Join(rest, " ; ")
		}
		lines[i] = line
		break
	}
	return strings.Join(lines, "\n")
}

// badgeOf is an entry's badge line: as written when it was imported so,
// else from its fields.
func badgeOf(e pageEntry) string {
	if e.Badge != "" {
		return e.Badge
	}
	maturity, who := e.Maturity, e.ForWhom
	if maturity == "" {
		maturity = "Construit et testé"
	}
	if who == "" {
		who = "tous"
	}
	return fmt.Sprintf("**Valeur :** %s · **Maturité :** %s · **Pour qui :** %s", api.ValueLabels[e.Value], maturity, who)
}

// renderPage writes a page from its head, its entries and its tail. Domains
// come in value order (the most majors first, then notables…), entries in a
// domain newest first (date, then number).
func renderPage(head, tail string, entries []pageEntry) string {
	byDomain := map[string][]pageEntry{}
	var domains []string
	for _, e := range entries {
		if _, ok := byDomain[e.Domain]; !ok {
			domains = append(domains, e.Domain)
		}
		byDomain[e.Domain] = append(byDomain[e.Domain], e)
	}
	weight := func(d string) []int {
		w := make([]int, len(api.ValueOrder))
		for _, e := range byDomain[d] {
			for i, v := range api.ValueOrder {
				if e.Value == v {
					w[i]++
				}
			}
		}
		return w
	}
	sort.SliceStable(domains, func(i, j int) bool {
		a, b := weight(domains[i]), weight(domains[j])
		for k := range a {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return domains[i] < domains[j]
	})
	var b strings.Builder
	b.WriteString(strings.TrimRight(fillHead(head, entries), "\n"))
	b.WriteString("\n\n")
	for _, d := range domains {
		es := byDomain[d]
		sort.SliceStable(es, func(i, j int) bool {
			if es[i].Date != es[j].Date {
				return es[i].Date > es[j].Date
			}
			return es[i].Number > es[j].Number
		})
		fmt.Fprintf(&b, "## %s\n\n", d)
		for _, e := range es {
			fmt.Fprintf(&b, "### %s. %s\n\n%s\n\n", e.numbered(), e.Title, badgeOf(e))
			if t := strings.TrimSpace(e.Text); t != "" {
				b.WriteString(t)
				b.WriteString("\n\n")
			}
			b.WriteString("---\n\n")
		}
	}
	if t := strings.TrimSpace(tail); t != "" {
		b.WriteString(t)
		b.WriteString("\n")
	}
	return b.String()
}

// fillHead fills the head's markers from the entries.
func fillHead(head string, entries []pageEntry) string {
	if strings.Contains(head, markUpdates) {
		dated := make([]pageEntry, 0, len(entries))
		for _, e := range entries {
			if e.Date != "" {
				dated = append(dated, e)
			}
		}
		sort.SliceStable(dated, func(i, j int) bool {
			if dated[i].Date != dated[j].Date {
				return dated[i].Date > dated[j].Date
			}
			return dated[i].Number > dated[j].Number
		})
		var segs []string
		day := ""
		for _, e := range dated {
			line := e.Update
			if line == "" {
				line = lowerFirst(e.Title)
			}
			// " ; " separates the updates: one inside would cut this one
			// in two when the line is read again.
			line = strings.ReplaceAll(line, " ; ", ", ")
			seg := fmt.Sprintf("changement %d : %s", e.Number, line)
			if e.Date != day {
				day = e.Date
				seg = frDate(e.Date) + " — " + seg
			}
			segs = append(segs, seg)
		}
		filled := strings.Join(segs, " ; ")
		if filled == "" {
			head = strings.Replace(head, markUpdates+" ; ", "", 1)
		}
		head = strings.Replace(head, markUpdates, filled, 1)
	}
	if strings.Contains(head, markCount) {
		head = strings.Replace(head, markCount, countLine(entries), 1)
	}
	return head
}

// countLine is `**N changements** : x majeurs · y notables · z améliorations · w corrections`.
func countLine(entries []pageEntry) string {
	n := map[string]int{}
	for _, e := range entries {
		n[e.Value]++
	}
	plural := func(k int, one, many string) string {
		if k > 1 {
			return fmt.Sprintf("%d %s", k, many)
		}
		return fmt.Sprintf("%d %s", k, one)
	}
	return fmt.Sprintf("**%s** : %s · %s · %s · %s",
		plural(len(entries), "changement", "changements"),
		plural(n[api.ValueMajor], "majeur", "majeurs"), plural(n[api.ValueNotable], "notable", "notables"),
		plural(n[api.ValueImprovement], "amélioration", "améliorations"), plural(n[api.ValueFix], "correction", "corrections"))
}

// frDate writes YYYY-MM-DD as DD/MM/YYYY.
func frDate(d string) string {
	if len(d) != 10 {
		return d
	}
	return d[8:10] + "/" + d[5:7] + "/" + d[0:4]
}

func lowerFirst(s string) string {
	for i, r := range s {
		return strings.ToLower(string(r)) + s[i+len(string(r)):]
	}
	return s
}

var (
	bracketIDs = regexp.MustCompile(`\[([a-z0-9-]+\.[A-Z]+-\d+(?:\s*,\s*[a-z0-9-]+\.[A-Z]+-\d+)*)\]`)
	objectRef  = regexp.MustCompile(`^[a-z0-9-]+\.[A-Z]+-\d+$`)
)

// numbered is an entry's number as its heading writes it.
func (e pageEntry) numbered() string {
	if e.Label != "" {
		return e.Label
	}
	return strconv.Itoa(e.Number)
}
