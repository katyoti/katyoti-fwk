// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// FULL-TEXT SEARCH. CWM's own inverted index over every text an object holds —
// its id, every string of every capability group, its body — kept by the object
// index: built by the scan, updated by every write-through and delete, so it is
// never stale and never scheduled. No model and no embedding: words, their
// frequency, and BM25. Meaning-level search is the kb extension's, over the
// same objects.
//
// Ranking: an object matching MORE of the query's words comes first; among
// equals, the higher BM25 score. A field's words weigh by what the field is — a
// label counts more than a body. A query word of three letters or more also
// matches the words it begins ("valid" → "validity"), a little below an exact
// match. Words are lower-cased and accent-folded ("décision" = "decision").

const (
	ftK1          = 1.2
	ftB           = 0.75
	ftPrefixScore = 0.8 // a prefix match counts this much of an exact one
	ftSnippetLen  = 160
)

// ftField is one text of an object, with the weight of what it is.
type ftField struct {
	name   string
	text   string
	weight float64
}

type ftDoc struct {
	fields []ftField
	length float64 // weighted word count
}

type ftIndex struct {
	docs     map[string]*ftDoc
	postings map[string]map[string]map[int]int // word → object id → field index → count
	total    float64                           // sum of the docs' lengths
}

func newFTIndex() *ftIndex {
	return &ftIndex{docs: map[string]*ftDoc{}, postings: map[string]map[string]map[int]int{}}
}

// add indexes obj (the full object, body included), replacing what was there.
func (ft *ftIndex) add(obj api.Object) {
	ft.remove(obj.ID)
	doc := &ftDoc{fields: objectTexts(obj)}
	for fi, f := range doc.fields {
		words := ftWords(f.text)
		doc.length += f.weight * float64(len(words))
		for _, w := range words {
			byDoc := ft.postings[w]
			if byDoc == nil {
				byDoc = map[string]map[int]int{}
				ft.postings[w] = byDoc
			}
			byField := byDoc[obj.ID]
			if byField == nil {
				byField = map[int]int{}
				byDoc[obj.ID] = byField
			}
			byField[fi]++
		}
	}
	ft.docs[obj.ID] = doc
	ft.total += doc.length
}

// adopt makes ft hold src's document for id, or none when src has none: a scan
// keeps the live entry of an object written while it read (objectindex.go).
func (ft *ftIndex) adopt(src *ftIndex, id string) {
	ft.remove(id)
	doc, ok := src.docs[id]
	if !ok {
		return
	}
	for fi, f := range doc.fields {
		for _, w := range ftWords(f.text) {
			byDoc := ft.postings[w]
			if byDoc == nil {
				byDoc = map[string]map[int]int{}
				ft.postings[w] = byDoc
			}
			byField := byDoc[id]
			if byField == nil {
				byField = map[int]int{}
				byDoc[id] = byField
			}
			byField[fi]++
		}
	}
	ft.docs[id] = doc
	ft.total += doc.length
}

func (ft *ftIndex) remove(id string) {
	doc, ok := ft.docs[id]
	if !ok {
		return
	}
	for _, f := range doc.fields {
		for _, w := range ftWords(f.text) {
			if byDoc := ft.postings[w]; byDoc != nil {
				delete(byDoc, id)
				if len(byDoc) == 0 {
					delete(ft.postings, w)
				}
			}
		}
	}
	ft.total -= doc.length
	delete(ft.docs, id)
}

// ftHit is one ranked object.
type ftHit struct {
	id       string
	score    float64
	coverage int
	field    int
	matched  []string
}

// search ranks the objects keep admits against query.
func (ft *ftIndex) search(query string, keep func(id string) bool) []api.ObjectSearchHit {
	terms := uniqueWords(ftWords(query))
	if len(terms) == 0 || len(ft.docs) == 0 {
		return nil
	}
	n := float64(len(ft.docs))
	avg := ft.total / n
	if avg == 0 {
		avg = 1
	}
	type acc struct {
		score      float64
		coverage   int
		fieldScore map[int]float64
		matched    []string
	}
	byID := map[string]*acc{}
	for _, t := range terms {
		// The words this query word matches, each with how much it counts.
		matches := map[string]float64{}
		if _, ok := ft.postings[t]; ok {
			matches[t] = 1
		}
		if len([]rune(t)) >= 3 {
			for w := range ft.postings {
				if w != t && strings.HasPrefix(w, t) {
					matches[w] = ftPrefixScore
				}
			}
		}
		// Per object: the best word this query word matched there.
		best := map[string]float64{}
		bestField := map[string]map[int]float64{}
		for w, factor := range matches {
			byDoc := ft.postings[w]
			idf := math.Log(1 + (n-float64(len(byDoc))+0.5)/(float64(len(byDoc))+0.5))
			for id, byField := range byDoc {
				if !keep(id) {
					continue
				}
				doc := ft.docs[id]
				var tf float64
				for fi, c := range byField {
					tf += doc.fields[fi].weight * float64(c)
				}
				s := factor * idf * tf * (ftK1 + 1) / (tf + ftK1*(1-ftB+ftB*doc.length/avg))
				if s > best[id] {
					best[id] = s
					fs := map[int]float64{}
					for fi, c := range byField {
						fs[fi] = factor * doc.fields[fi].weight * float64(c)
					}
					bestField[id] = fs
				}
			}
		}
		for id, s := range best {
			a := byID[id]
			if a == nil {
				a = &acc{fieldScore: map[int]float64{}}
				byID[id] = a
			}
			a.score += s
			a.coverage++
			a.matched = append(a.matched, t)
			for fi, fs := range bestField[id] {
				a.fieldScore[fi] += fs
			}
		}
	}
	hits := make([]ftHit, 0, len(byID))
	for id, a := range byID {
		field, top := 0, -1.0
		for fi, s := range a.fieldScore {
			if s > top || (s == top && fi < field) {
				field, top = fi, s
			}
		}
		hits = append(hits, ftHit{id: id, score: a.score, coverage: a.coverage, field: field, matched: a.matched})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].coverage != hits[j].coverage {
			return hits[i].coverage > hits[j].coverage
		}
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].id < hits[j].id
	})
	out := make([]api.ObjectSearchHit, len(hits))
	for i, h := range hits {
		f := ft.docs[h.id].fields[h.field]
		out[i] = api.ObjectSearchHit{
			ID:      h.id,
			Score:   math.Round(h.score*1000) / 1000,
			Matched: h.matched,
			Field:   f.name,
			Snippet: ftSnippet(f.text, h.matched),
		}
	}
	return out
}

// objectTexts lists every text of obj with its weight: the id, every string of
// every capability group (by path, `name`, `want.title`), the body, the
// type's fields.
func objectTexts(obj api.Object) []ftField {
	fields := []ftField{{name: "id", text: strings.NewReplacer(".", " ", "-", " ", "_", " ").Replace(obj.ID), weight: 2}}
	for _, f := range []ftField{
		{name: "name", text: obj.Name, weight: 3},
		{name: "slug", text: obj.Slug, weight: 2},
		{name: "description", text: obj.Description, weight: 2},
		{name: "tags", text: strings.Join(obj.Tags, " "), weight: 2},
	} {
		if strings.TrimSpace(f.text) != "" {
			fields = append(fields, f)
		}
	}
	collectTexts("body", obj.Body, &fields)
	if len(obj.Fields) > 0 {
		collectTexts("fields", obj.Fields, &fields)
	}
	caps := make([]string, 0, len(obj.Capabilities))
	for c := range obj.Capabilities {
		caps = append(caps, c)
	}
	sort.Strings(caps)
	for _, c := range caps {
		collectTexts(c, obj.Capabilities[c], &fields)
	}
	return fields
}

func collectTexts(path string, v any, out *[]ftField) {
	switch t := v.(type) {
	case string:
		if strings.TrimSpace(t) != "" {
			*out = append(*out, ftField{name: path, text: t, weight: fieldWeight(path)})
		}
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			collectTexts(path+"."+k, t[k], out)
		}
	case []any:
		for _, e := range t {
			collectTexts(path, e, out)
		}
	case []string:
		for _, e := range t {
			collectTexts(path, e, out)
		}
	case json.RawMessage:
		var d any
		if json.Unmarshal(t, &d) == nil {
			collectTexts(path, d, out)
		}
	case nil, bool, int, int64, float64:
	default:
		// Any other Go value (a typed slice, a struct): through JSON, which only
		// yields the kinds handled above.
		if b, err := json.Marshal(t); err == nil {
			var d any
			if json.Unmarshal(b, &d) == nil {
				collectTexts(path, d, out)
			}
		}
	}
}

// fieldWeight: what a field is decides what its words weigh.
func fieldWeight(path string) float64 {
	last := path[strings.LastIndex(path, ".")+1:]
	switch last {
	case "name", "title":
		return 3
	case "description", "tags":
		return 2
	}
	return 1
}

// ftWords splits text into lower-cased, accent-folded words.
func ftWords(text string) []string {
	var words []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			words = append(words, b.String())
			b.Reset()
		}
	}
	for _, r := range norm.NFD.String(text) {
		switch {
		case unicode.Is(unicode.Mn, r):
			// a combining accent: folded away
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		default:
			flush()
		}
	}
	flush()
	return words
}

func uniqueWords(ws []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range ws {
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// ftSnippet returns a window of text around the first matched word.
func ftSnippet(text string, matched []string) string {
	flat := strings.Join(strings.Fields(text), " ")
	runes := []rune(flat)
	if len(runes) <= ftSnippetLen {
		return flat
	}
	// Find the first matched word, comparing folded text rune by rune.
	folded := []rune(foldRunes(flat))
	at := -1
	for _, m := range matched {
		if i := indexRunes(folded, []rune(m)); i >= 0 && (at < 0 || i < at) {
			at = i
		}
	}
	start := 0
	if at > ftSnippetLen/3 {
		start = at - ftSnippetLen/3
	}
	end := start + ftSnippetLen
	if end > len(runes) {
		end, start = len(runes), max(0, len(runes)-ftSnippetLen)
	}
	s := string(runes[start:end])
	if start > 0 {
		s = "…" + s
	}
	if end < len(runes) {
		s += "…"
	}
	return s
}

// foldRunes lower-cases and strips accents rune for rune, keeping positions.
func foldRunes(s string) string {
	var b strings.Builder
	for _, r := range s {
		d := []rune(norm.NFD.String(string(r)))
		b.WriteRune(unicode.ToLower(d[0]))
	}
	return b.String()
}

func indexRunes(hay, needle []rune) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		ok := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}
