// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WhatsNew is the text of the guide « Quoi de neuf »: the
// entries recorded after the one the person last saw, by version, the newest
// version first, each opening on its release's « En un coup d'œil ». Entry ids
// grow with each entry recorded, so "after" is an id; a person who saw none
// is shown the latest version only.
func (c *Changes) WhatsNew(ctx context.Context, in api.WhatsNewInput) (api.WhatsNewOutput, error) {
	out := api.WhatsNewOutput{Versions: []string{}}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return out, err
	}
	// An installation has no change entries: its solution's letter, placed
	// in its project, is what is new.
	if l, ok, err := installedLetter(ctx, cwm); err != nil {
		return out, err
	} else if ok {
		if in.After == l.Key {
			return out, nil
		}
		return l, nil
	}
	all, err := c.entries(ctx, cwm)
	if err != nil {
		return out, err
	}
	after := entrySeq(in.After)
	var fresh []cwmapi.Object
	for _, o := range all {
		if entrySeq(o.ID) > after {
			fresh = append(fresh, o)
		}
	}
	if len(fresh) == 0 {
		return out, nil
	}
	sort.SliceStable(fresh, func(i, j int) bool { return entrySeq(fresh[i].ID) > entrySeq(fresh[j].ID) })
	out.Key = fresh[0].ID
	// By release, the release of the newest entry first.
	var order []string
	byRelease := map[string][]cwmapi.Object{}
	for _, o := range fresh {
		r := o.ParentID()
		if _, ok := byRelease[r]; !ok {
			order = append(order, r)
		}
		byRelease[r] = append(byRelease[r], o)
	}
	if in.After == "" {
		order = order[:1] // nothing seen yet: the latest version only
	}
	var b strings.Builder
	for _, r := range order {
		rel, err := loadRelease(ctx, cwm, r)
		if err != nil {
			continue
		}
		f := releaseFields(rel)
		out.Versions = append(out.Versions, f.Version)
		fmt.Fprintf(&b, "## %s\n\n", api.ReleaseLabel(f.Version, rel.Name))
		if glance := glanceOf(f.ChangeLogHead); glance != "" {
			b.WriteString(glance)
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "### Les changements\n\n")
		for _, o := range byRelease[r] {
			fmt.Fprintf(&b, "- **%s**", o.Name)
			if gain := gainOf(o.Body); gain != "" {
				fmt.Fprintf(&b, " — %s", gain)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	out.Markdown = strings.TrimSpace(b.String())
	return out, nil
}

// letterSlug is how a solution's « Quoi de neuf » is named once installed:
// the document <module>.quoi-de-neuf-<lang> (marketplace soldocs).
const letterSlug = ".quoi-de-neuf-"

// installedLetter is the project's installed letter, French first: its text
// up to the line `---` that opens the full change log below it, the document
// it is (opened whole by its id), its title line as its version, and a key
// that changes with each version's letter.
func installedLetter(ctx context.Context, cwm cwmapi.CWMService) (api.WhatsNewOutput, bool, error) {
	docs, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: "document"})
	if err != nil {
		return api.WhatsNewOutput{}, false, err
	}
	var ids []string
	for _, d := range docs.Objects {
		if strings.Contains(d.ID, letterSlug) {
			ids = append(ids, d.ID)
		}
	}
	if len(ids) == 0 {
		return api.WhatsNewOutput{}, false, nil
	}
	sort.SliceStable(ids, func(i, j int) bool {
		return strings.HasSuffix(ids[i], letterSlug+"fr") && !strings.HasSuffix(ids[j], letterSlug+"fr")
	})
	id := ids[0]
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return api.WhatsNewOutput{}, false, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return api.WhatsNewOutput{}, false, err
	}
	body := strings.ReplaceAll(got.Object.Body, "\r\n", "\n")
	sum := sha256.Sum256([]byte(body))
	out := api.WhatsNewOutput{Versions: []string{}, Document: id, Key: fmt.Sprintf("letter:%s@%x", id, sum[:6])}
	var letter []string
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "---" {
			break
		}
		if title, ok := strings.CutPrefix(line, "# "); ok && len(out.Versions) == 0 {
			out.Versions = append(out.Versions, strings.TrimSpace(title))
		}
		letter = append(letter, line)
	}
	out.Markdown = strings.TrimSpace(strings.Join(letter, "\n"))
	return out, true, nil
}

// entrySeq is an entry id's sequence (→ 540); 0 for
// anything else.
func entrySeq(id string) int {
	i := strings.LastIndex(id, ".CH-")
	if i < 0 {
		return 0
	}
	digits := id[i+len(".CH-"):]
	if j := strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }); j >= 0 {
		digits = digits[:j]
	}
	n, _ := strconv.Atoi(digits)
	return n
}

// glanceOf is a page head's « En un coup d'œil » section, its heading one
// level down so it sits under its version.
func glanceOf(head string) string {
	head = strings.ReplaceAll(head, "\r\n", "\n")
	const h = "## En un coup d'œil"
	i := strings.Index(head, h)
	if i < 0 {
		return ""
	}
	rest := head[i+len(h):]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		rest = rest[:j]
	}
	rest = strings.TrimSpace(strings.ReplaceAll(rest, markCount, ""))
	if rest == "" {
		return ""
	}
	return "### En un coup d'œil\n\n" + rest
}

// gainOf is what an entry brings in one paragraph: its « Ce que ça vous
// apporte » line without its label, else its first paragraph.
func gainOf(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	const label = "**Ce que ça vous apporte :**"
	if i := strings.Index(text, label); i >= 0 {
		text = text[i+len(label):]
	}
	para := strings.TrimSpace(text)
	if j := strings.Index(para, "\n\n"); j >= 0 {
		para = para[:j]
	}
	return strings.Join(strings.Fields(para), " ")
}
