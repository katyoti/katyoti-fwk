// SPDX-License-Identifier: Apache-2.0

package contribapi

// translations.go — what a kind lets a person read in their language, and how
// an entry's `translations` layer gives it.
//
// A kind declares its translatable texts once (`translatable` on the kind,
// KindDeclaration.Translatable), as paths into its data layer — segments
// joined by `.`:
//
//	label                     a field holding a text
//	hashtags                  a field holding a list of texts (translated as a whole)
//	steps[id].say             the `say` field of each item of the list `steps`, the
//	                          item named by its `id` field (a list is keyed by
//	                          identity, never by position: a reordered list keeps
//	                          its translations)
//	pane.badges[field].label  a field inside an object field, then a list
//	types.*.fields.*.label    `*` is every key of a map (a data-type's types, a
//	                          type's fields): the item is named by its key
//
// The last segment names a text; `*` is never first or last.
//
// An entry's translations are the file `<name>.i18n.yaml` beside it, published
// as the `translations` layer (canonical JSON): language code → the same shape
// as the entry, holding only the translatable texts, a list becoming a map by
// identity:
//
//	source_lang: fr        # optional: the language the entry is written in
//	en:
//	  label: Architecture file
//	  steps:
//	    intro: {say: Welcome}
//	    # or, when the list declares one translatable field, its text alone:
//	    outro: Bye
//
// Every text is addressed by a FLAT path — `label`, `hashtags`,
// `steps.intro.say` — which is the key of the hub's translation bundle
// (`<uri>#<path>`) and of the GUI's `ct(uri, path, fallback)`. A text a
// language leaves out falls back to the entry's own.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// TranslationsSourceLangKey is the reserved top-level key of a translations
// layer naming the language the entry itself is written in. Every other
// top-level key is a language code.
const TranslationsSourceLangKey = "source_lang"

// DefaultSourceLang is the language an entry is written in when neither it
// nor its pack says otherwise.
const DefaultSourceLang = "en"

var (
	reTranslatableSeg = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)(?:\[([a-zA-Z_][a-zA-Z0-9_]*)\])?$`)
	reLanguage        = regexp.MustCompile(`^[a-z]{2}$`)
	reTemplateOnly    = regexp.MustCompile(`^\$\{[^}]*\}$`)
)

// IsLanguageCode reports whether s is a language code a translations layer
// may name: two lowercase letters.
func IsLanguageCode(s string) bool { return reLanguage.MatchString(s) }

// PathSegment is one step of a translatable path.
type PathSegment struct {
	// Field is the field stepped into ("" for `*`).
	Field string
	// Key, when set, makes Field a list whose items are named by their Key
	// field: the step goes into each item.
	Key string
	// Any is `*`: every key of the map at this point, the item named by its key.
	Any bool
}

// TranslatablePath is one declared translatable text of a kind.
type TranslatablePath struct {
	Segments []PathSegment
}

// ParseTranslatablePath reads one entry of a kind's `translatable` list.
func ParseTranslatablePath(s string) (TranslatablePath, error) {
	bad := fmt.Errorf("translatable path %q is not `field`, `list[key].field`, `a.b`, or `map.*.field` (the last segment a plain field, `*` never first)", s)
	if s == "" {
		return TranslatablePath{}, bad
	}
	parts := strings.Split(s, ".")
	var p TranslatablePath
	for i, part := range parts {
		if part == "*" {
			if i == 0 || i == len(parts)-1 {
				return TranslatablePath{}, bad
			}
			p.Segments = append(p.Segments, PathSegment{Any: true})
			continue
		}
		m := reTranslatableSeg.FindStringSubmatch(part)
		if m == nil || (m[2] != "" && i == len(parts)-1) {
			return TranslatablePath{}, bad
		}
		p.Segments = append(p.Segments, PathSegment{Field: m[1], Key: m[2]})
	}
	return p, nil
}

// String is the path as a kind declares it.
func (p TranslatablePath) String() string {
	parts := make([]string, len(p.Segments))
	for i, s := range p.Segments {
		switch {
		case s.Any:
			parts[i] = "*"
		case s.Key != "":
			parts[i] = s.Field + "[" + s.Key + "]"
		default:
			parts[i] = s.Field
		}
	}
	return strings.Join(parts, ".")
}

// ParseTranslatablePaths reads a kind's whole `translatable` list; the
// paths that do not parse, or that contradict an earlier one (a field read
// as a text by one path and stepped into by another), are returned as
// errors and left out.
func ParseTranslatablePaths(decl []string) ([]TranslatablePath, []error) {
	var out []TranslatablePath
	var errs []error
	root := newTrObject()
	for _, s := range decl {
		p, err := ParseTranslatablePath(s)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		probe := root.clone()
		if err := probe.add(p); err != nil {
			errs = append(errs, fmt.Errorf("translatable path %q: %w", s, err))
			continue
		}
		root = probe
		out = append(out, p)
	}
	return out, errs
}

// TranslatablePaths returns the parsed translatable paths of this kind.
func (d KindDeclaration) TranslatablePaths() []TranslatablePath {
	out, _ := ParseTranslatablePaths(d.Translatable)
	return out
}

// trNode is one position of a kind's translatable texts, the paths merged
// into a tree: a text, an object (fields), or a collection — a list keyed by
// an item field, or a map — whose items share one node.
type trNode struct {
	kind   trKind
	fields map[string]*trNode // trObject
	key    string             // trList: the item field naming an item
	item   *trNode            // trList, trMap
}

type trKind int

const (
	trUnset trKind = iota
	trText
	trObject
	trList
	trMap
)

func newTrObject() *trNode { return &trNode{kind: trObject, fields: map[string]*trNode{}} }

func (n *trNode) clone() *trNode {
	if n == nil {
		return nil
	}
	c := &trNode{kind: n.kind, key: n.key, item: n.item.clone()}
	if n.fields != nil {
		c.fields = make(map[string]*trNode, len(n.fields))
		for k, v := range n.fields {
			c.fields[k] = v.clone()
		}
	}
	return c
}

func (n *trNode) become(k trKind) error {
	if n.kind == trUnset {
		n.kind = k
		if k == trObject {
			n.fields = map[string]*trNode{}
		}
		return nil
	}
	if n.kind != k {
		return fmt.Errorf("one position is read both as a text and as a structure, or as a list and a map")
	}
	return nil
}

func (n *trNode) add(p TranslatablePath) error {
	cur := n
	for i, s := range p.Segments {
		last := i == len(p.Segments)-1
		if s.Any {
			if err := cur.become(trMap); err != nil {
				return err
			}
			if cur.item == nil {
				cur.item = &trNode{}
			}
			cur = cur.item
			continue
		}
		if err := cur.become(trObject); err != nil {
			return err
		}
		child := cur.fields[s.Field]
		if child == nil {
			child = &trNode{}
			cur.fields[s.Field] = child
		}
		cur = child
		if s.Key != "" {
			if err := cur.become(trList); err != nil {
				return err
			}
			if cur.key != "" && cur.key != s.Key {
				return fmt.Errorf("list %q keyed by both %q and %q", s.Field, cur.key, s.Key)
			}
			cur.key = s.Key
			if cur.item == nil {
				cur.item = &trNode{}
			}
			cur = cur.item
		}
		if last {
			if err := cur.become(trText); err != nil {
				return err
			}
		}
	}
	return nil
}

// trTree merges paths into one tree; a path contradicting an earlier one is
// left out (ParseTranslatablePaths reports it).
func trTree(paths []TranslatablePath) *trNode {
	root := newTrObject()
	for _, p := range paths {
		probe := root.clone()
		if probe.add(p) == nil {
			root = probe
		}
	}
	return root
}

// shorthand is the one text field of a collection item that declares
// exactly one ("" otherwise): the translation may give that text alone.
func (n *trNode) shorthand() string {
	if n.kind != trObject || len(n.fields) != 1 {
		return ""
	}
	for f, c := range n.fields {
		if c.kind == trText {
			return f
		}
	}
	return ""
}

func joinPath(prefix, k string) string {
	if prefix == "" {
		return k
	}
	return prefix + "." + k
}

// Translations is an entry's `translations` layer, decoded.
type Translations struct {
	// SourceLang is the language the entry is written in, when the
	// translations say so ("" otherwise: the pack's, else English).
	SourceLang string
	// Languages maps a language code to its texts, in the entry's shape.
	Languages map[string]map[string]any
}

// ParseTranslations decodes a translations layer (canonical JSON).
func ParseTranslations(raw []byte) (Translations, error) {
	var top map[string]any
	if err := json.Unmarshal(raw, &top); err != nil {
		return Translations{}, fmt.Errorf("translations: %w", err)
	}
	t := Translations{Languages: map[string]map[string]any{}}
	for k, v := range top {
		if k == TranslationsSourceLangKey {
			s, ok := v.(string)
			if !ok {
				return Translations{}, fmt.Errorf("translations: %s must be a language code", TranslationsSourceLangKey)
			}
			t.SourceLang = s
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			if v == nil {
				m = map[string]any{}
			} else {
				return Translations{}, fmt.Errorf("translations: %q must map texts, got %T", k, v)
			}
		}
		t.Languages[k] = m
	}
	return t, nil
}

// SourceTexts returns every translatable text an entry's data layer holds,
// by flat path. An empty text is left out: there is nothing to translate; so
// is a text that is only one `${...}` template (its value comes from the data,
// already in the reader's language) — a text mixing words and `${...}` stays.
func SourceTexts(paths []TranslatablePath, data json.RawMessage) map[string]string {
	out := map[string]string{}
	var tree map[string]any
	if len(data) == 0 || json.Unmarshal(data, &tree) != nil {
		return out
	}
	sourceWalk(trTree(paths), tree, "", out)
	return out
}

func sourceWalk(n *trNode, v any, prefix string, out map[string]string) {
	switch n.kind {
	case trText:
		if s, ok := textOf(v); ok && !reTemplateOnly.MatchString(strings.TrimSpace(s)) {
			out[prefix] = s
		}
	case trObject:
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		for f, c := range n.fields {
			sourceWalk(c, m[f], joinPath(prefix, f), out)
		}
	case trList:
		items, _ := v.([]any)
		for _, it := range items {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			key, ok := m[n.key].(string)
			if !ok || key == "" {
				continue
			}
			sourceWalk(n.item, m, joinPath(prefix, key), out)
		}
	case trMap:
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		for k, it := range m {
			sourceWalk(n.item, it, joinPath(prefix, k), out)
		}
	}
}

// FlattenTranslation flattens one language's texts to flat paths, against
// the kind's declared paths. `unknown` names every key the kind does not
// declare as translatable, and every value of the wrong shape.
//
// A translation has the entry's shape with every list a map by identity; an
// item of a list or map whose declaration names one text may give that text
// alone (`intro: Welcome` for `steps[id].say`).
func FlattenTranslation(paths []TranslatablePath, tree map[string]any) (texts map[string]string, unknown []string) {
	texts = map[string]string{}
	flattenWalk(trTree(paths), tree, "", false, texts, &unknown)
	sort.Strings(unknown)
	return texts, unknown
}

func flattenWalk(n *trNode, v any, prefix string, isItem bool, texts map[string]string, unknown *[]string) {
	if v == nil {
		return
	}
	switch n.kind {
	case trText:
		if s, ok := textOf(v); ok {
			texts[prefix] = s
		} else {
			*unknown = append(*unknown, prefix)
		}
	case trObject:
		if s, ok := v.(string); ok && isItem {
			if f := n.shorthand(); f != "" {
				if s != "" {
					texts[joinPath(prefix, f)] = s
				}
				return
			}
		}
		m, ok := v.(map[string]any)
		if !ok {
			*unknown = append(*unknown, prefix)
			return
		}
		for k, sub := range m {
			c, declared := n.fields[k]
			if !declared {
				*unknown = append(*unknown, joinPath(prefix, k))
				continue
			}
			flattenWalk(c, sub, joinPath(prefix, k), false, texts, unknown)
		}
	case trList, trMap:
		m, ok := v.(map[string]any)
		if !ok {
			*unknown = append(*unknown, prefix)
			return
		}
		for id, sub := range m {
			flattenWalk(n.item, sub, joinPath(prefix, id), true, texts, unknown)
		}
	}
}

// TranslationFilePath is where the text of flat path `flat` is written in a
// translation tree: the keys of the maps leading to it and its own key —
// `steps.intro.say` is `steps` → `intro` → `say`, or `steps` → `intro` alone
// when the item declares one text (the shorthand). An item's key may hold
// dots (a badge named by its field `fields.severity`): the split that reaches
// a text wins. ok is false for a path the declaration does not reach.
func TranslationFilePath(paths []TranslatablePath, flat string) (parents []string, leaf string, ok bool) {
	keys, ok := filePath(trTree(paths), strings.Split(flat, "."), false)
	if !ok || len(keys) == 0 {
		return nil, "", false
	}
	return keys[:len(keys)-1], keys[len(keys)-1], true
}

// filePath resolves parts against node n; isItem says n is a collection's
// item (where the shorthand applies).
func filePath(n *trNode, parts []string, isItem bool) ([]string, bool) {
	if len(parts) == 0 {
		return nil, n.kind == trText
	}
	switch n.kind {
	case trObject:
		c, declared := n.fields[parts[0]]
		if !declared {
			return nil, false
		}
		if len(parts) == 1 && isItem && n.shorthand() == parts[0] {
			return []string{}, true // the item itself holds the text
		}
		rest, ok := filePath(c, parts[1:], false)
		if !ok {
			return nil, false
		}
		return append([]string{parts[0]}, rest...), true
	case trList, trMap:
		for i := 1; i <= len(parts); i++ {
			key := strings.Join(parts[:i], ".")
			if rest, ok := filePath(n.item, parts[i:], true); ok {
				return append([]string{key}, rest...), true
			}
		}
	}
	return nil, false
}

// EntrySourceLang is the language an entry is written in: its translations'
// `source_lang`, else its pack's, else English.
func EntrySourceLang(t Translations, packLang string) string {
	switch {
	case t.SourceLang != "":
		return t.SourceLang
	case packLang != "":
		return packLang
	}
	return DefaultSourceLang
}

// TranslationKey is the key of one text in a translation bundle.
func TranslationKey(uri, path string) string { return uri + "#" + path }

// BundleTexts returns an entry's texts in `lang`, keyed for the translation
// bundle (TranslationKey), from its translations layer. Nothing when the
// entry is written in `lang` or has no translation into it: the reader shows
// the entry's own text.
func BundleTexts(uri string, paths []TranslatablePath, translations []byte, lang string) map[string]string {
	if len(paths) == 0 || len(translations) == 0 {
		return nil
	}
	t, err := ParseTranslations(translations)
	if err != nil || t.SourceLang == lang {
		return nil
	}
	tree, ok := t.Languages[lang]
	if !ok {
		return nil
	}
	texts, _ := FlattenTranslation(paths, tree)
	if len(texts) == 0 {
		return nil
	}
	out := make(map[string]string, len(texts))
	for p, s := range texts {
		out[TranslationKey(uri, p)] = s
	}
	return out
}

// textOf reads a text: a string, or a list of strings joined by ", " (a
// list of texts is translated as a whole).
func textOf(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, x != ""
	case []any:
		var parts []string
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return "", false
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, ", "), len(parts) > 0
	}
	return "", false
}
