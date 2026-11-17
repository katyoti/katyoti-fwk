// SPDX-License-Identifier: Apache-2.0

package contentlint

// i18n.go — what a pack still has to translate, and the stubs to fill.
//
// The authoring side of the one translation lint (translations.go): for one
// language, the texts each entry leaves untranslated (its source text beside
// each), read from a pack folder on disk or from a compiled sidecar's
// manifest; and, on disk, the `<name>.i18n.yaml` stubs an author (a person or
// an agent, skill `translate-contributions`) fills. The judgement is
// LintTranslations'; nothing here decides differently.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

// CWMPackFolderKinds is the folder (under a pack's `embed/cwm`) of each kind
// whose texts the GUI shows on an object.
var CWMPackFolderKinds = map[string]string{
	"object-types":      "object-type",
	"object_types":      "object-type", // the former folder
	"object-panels":     "object-panel",
	"capabilities":      "capability",
	"workflows":         "workflow",
	"gates":             "gate",
	"gate-requirements": "gate-requirement",
	"review-types":      "review-type",
	"aspects":           "aspect",
	"policies":          "policy",
	"edge-types":        "edge-type",
	"edge_types":        "edge-type", // the former folder
	"hook-types":        "hook-type",
	"event-types":       "event-type",
	"agent-contexts":    "agent-context",
	"data-types":        "data-type",
}

// DataTypesFolder is the pack folder of the data-types, which — unlike every
// other kind's — may hold its entries in subfolders (`data-types/capabilities/`).
const DataTypesFolder = "data-types"

// GuideFolder is the folder (under `embed/`) of a binary's guide recipes.
const GuideFolder = "guide"

// TextGap is one text an entry leaves untranslated in a language.
type TextGap struct {
	Path   string `json:"path" desc:"The text's path in the entry (label, states.draft…)"`
	Source string `json:"source" desc:"The text in the entry's own language"`
}

// EntryTranslation is one entry's state in one language.
type EntryTranslation struct {
	Entry      string               `json:"entry" desc:"The entry: its file on disk, or its URI"`
	Kind       string               `json:"kind"`
	SourceLang string               `json:"source_lang" desc:"The language the entry is written in"`
	Missing    []TextGap            `json:"missing,omitempty" desc:"Texts not translated into the language"`
	Findings   []verify.LintFinding `json:"findings,omitempty" desc:"Errors of its translations (unknown texts, bad file)"`
}

// EntryTranslationOf is one entry in `lang`: the texts it misses there, and
// the errors LintTranslations finds in its translations. data and
// translations are the layers as JSON (translations nil: none).
func EntryTranslationOf(entry, kind string, paths []contribapi.TranslatablePath, data, translations []byte, packLang, lang string) EntryTranslation {
	e := EntryTranslation{Entry: entry, Kind: kind}
	for _, f := range LintTranslations(paths, data, translations, packLang) {
		if f.Severity == verify.SeverityError {
			e.Findings = append(e.Findings, f)
		}
	}
	var t contribapi.Translations
	if len(translations) > 0 {
		t, _ = contribapi.ParseTranslations(translations)
	}
	e.SourceLang = contribapi.EntrySourceLang(t, packLang)
	if lang == e.SourceLang {
		return e
	}
	got, _ := contribapi.FlattenTranslation(paths, t.Languages[lang])
	e.Missing = missingTexts(contribapi.SourceTexts(paths, data), got)
	return e
}

// missingTexts is every source text `got` leaves out, by path.
func missingTexts(source, got map[string]string) []TextGap {
	var out []TextGap
	for _, p := range sortedKeys(source) {
		if got[p] == "" {
			out = append(out, TextGap{Path: p, Source: source[p]})
		}
	}
	return out
}

// PackTranslations walks a folder on disk — a pack (`embed/cwm`), a binary's
// `embed/`, or any folder above them — and answers each translatable entry in
// `lang`: every YAML entry of a CWM pack folder (CWMPackFolderKinds) and of a
// guide folder, judged with the `<name>.i18n.yaml` beside it.
func PackTranslations(root, lang string) ([]EntryTranslation, error) {
	files, err := translatableFiles(root)
	if err != nil {
		return nil, err
	}
	sys, err := builtin.Kinds()
	if err != nil {
		return nil, err
	}
	var out []EntryTranslation
	for _, f := range files {
		paths := sys.Translatable(f.kind)
		if len(paths) == 0 {
			continue
		}
		raw, err := os.ReadFile(f.path)
		if err != nil {
			return nil, err
		}
		data, err := yamlToJSON(raw)
		if err != nil {
			out = append(out, EntryTranslation{Entry: f.path, Kind: f.kind,
				Findings: []verify.LintFinding{errf("entry-invalid", "%v", err)}})
			continue
		}
		var tr []byte
		if b, err := os.ReadFile(translationsFileOf(f.path)); err == nil {
			if tr, err = yamlToJSON(b); err != nil {
				out = append(out, EntryTranslation{Entry: f.path, Kind: f.kind,
					Findings: []verify.LintFinding{errf("translations-invalid", "%v", err)}})
				continue
			}
		}
		out = append(out, EntryTranslationOf(f.path, f.kind, paths, data, tr, PackSourceLangOnDisk(f.path), lang))
	}
	return out, nil
}

// ManifestTranslations answers each translatable entry of a compiled
// sidecar's manifest in `lang`, from the translations layers it carries
// inline (a layer the manifest does not carry inline reads as none).
func ManifestTranslations(m *mbapi.Manifest, lang string) []EntryTranslation {
	kinds := contribapi.KindsFrom(m.Contributions)
	sys, _ := builtin.Kinds()
	packLangs := PackSourceLangs(m)
	var out []EntryTranslation
	for _, c := range m.Contributions {
		paths := kinds.Translatable(c.Type)
		if _, declared := kinds.Get(c.Type); !declared {
			paths = sys.Translatable(c.Type)
		}
		if len(paths) == 0 {
			continue
		}
		var tr []byte
		if ref, ok := c.Layers[contribapi.LayerTranslations]; ok {
			tr = ref.Inline
		}
		data := TranslatableData(c)
		e := EntryTranslationOf(c.ResolvedURI(), c.Type, paths, data, tr, packLangs[c.Provider], lang)
		if len(contribapi.SourceTexts(paths, data)) == 0 && len(e.Findings) == 0 {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Entry < out[j].Entry })
	return out
}

// TranslatableData is the structure an entry's translatable texts are read
// from: its data layer, else — for a kind whose content is its source, a
// data-type's DSL — its source layer carried inline, read as YAML.
func TranslatableData(c contribapi.Contribution) []byte {
	if d := c.Data(); len(d) > 0 {
		return d
	}
	if ref, ok := c.Layers[contribapi.LayerSource]; ok && len(ref.Inline) > 0 {
		if j, err := yamlToJSON(ref.Inline); err == nil {
			return j
		}
	}
	return nil
}

// ScaffoldTranslations adds to the `<name>.i18n.yaml` beside an entry an
// empty stub for every text it misses in `lang` — `path: ~  # <source text>`
// — keeping what the file already says. An author replaces each `~` with the
// translation; a stub left empty still reads as missing. It answers how many
// stubs it added (0: nothing written).
func ScaffoldTranslations(e EntryTranslation, lang string) (int, error) {
	if len(e.Missing) == 0 || !fileExists(e.Entry) {
		return 0, nil
	}
	sys, err := builtin.Kinds()
	if err != nil {
		return 0, err
	}
	paths := sys.Translatable(e.Kind)
	file := translationsFileOf(e.Entry)
	var doc yaml.Node
	if raw, err := os.ReadFile(file); err == nil {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return 0, fmt.Errorf("%s: %w", file, err)
		}
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
		doc.HeadComment = "Translations of the " + e.Kind + " beside this file (modules/contributions/\ncontribution-types.md, Translations)."
	}
	top := doc.Content[0]
	if top.Kind != yaml.MappingNode {
		return 0, fmt.Errorf("%s: not a mapping", file)
	}
	langNode := mappingChild(top, lang)
	added := 0
	for _, gap := range e.Missing {
		// Where the text goes in the file: its maps, then its key (an item
		// declaring one text takes the shorthand, the text alone).
		parents, leaf, ok := contribapi.TranslationFilePath(paths, gap.Path)
		if !ok {
			continue
		}
		node := langNode
		for _, k := range parents {
			node = mappingChild(node, k)
		}
		if lookup(node, leaf) != nil {
			continue // translated, or a stub already there
		}
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: leaf},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "~", LineComment: oneLine(gap.Source)})
		added++
	}
	if added == 0 {
		return 0, nil
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return 0, err
	}
	_ = enc.Close()
	return added, os.WriteFile(file, buf.Bytes(), 0o644)
}

type translatableFile struct{ path, kind string }

// translatableFiles is every entry file under root whose folder names a
// translatable kind.
func translatableFiles(root string) ([]translatableFile, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, err
	}
	var out []translatableFile
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, manifestcontrib.TranslationsSuffix) {
			return err
		}
		// A kind's folder sits right under the pack (`cwm/<folder>`) or, for
		// the guide, under `embed/` — a same-named folder deeper (a data
		// type's `data-types/capabilities/`) is not one.
		dir, parent := filepath.Base(filepath.Dir(p)), filepath.Base(filepath.Dir(filepath.Dir(p)))
		kind, ok := CWMPackFolderKinds[dir]
		ok = ok && parent == "cwm" && dir != DataTypesFolder
		if !ok && underPackDataTypes(p) {
			kind, ok = "data-type", true
		}
		if dir == GuideFolder && parent == "embed" {
			kind, ok = "guide-recipe", true
		}
		if ok {
			out = append(out, translatableFile{path: p, kind: kind})
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, err
}

// underPackDataTypes reports whether a file sits under a pack's
// `cwm/data-types/`, at any depth.
func underPackDataTypes(p string) bool {
	for dir := filepath.Dir(p); ; {
		parent := filepath.Dir(dir)
		if filepath.Base(dir) == DataTypesFolder && filepath.Base(parent) == "cwm" {
			return true
		}
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// PackSourceLangOnDisk is the `source_lang` of the `embed/pack.yaml` of the
// binary an entry file belongs to ("" = English, also for an extension's
// pack, which has none).
func PackSourceLangOnDisk(entryPath string) string {
	for dir := filepath.Dir(entryPath); ; dir = filepath.Dir(dir) {
		if filepath.Base(dir) == "embed" {
			raw, err := os.ReadFile(filepath.Join(dir, "pack.yaml"))
			if err != nil {
				return ""
			}
			var p struct {
				SourceLang string `yaml:"source_lang"`
			}
			_ = yaml.Unmarshal(raw, &p)
			return p.SourceLang
		}
		if parent := filepath.Dir(dir); parent == dir {
			return ""
		}
	}
}

func translationsFileOf(entry string) string {
	return strings.TrimSuffix(entry, filepath.Ext(entry)) + manifestcontrib.TranslationsSuffix
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// mappingChild is the mapping under key in m, created when absent (or null).
func mappingChild(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			v := m.Content[i+1]
			if v.Kind != yaml.MappingNode {
				*v = yaml.Node{Kind: yaml.MappingNode}
			}
			return v
		}
	}
	v := &yaml.Node{Kind: yaml.MappingNode}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
	return v
}

func lookup(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func oneLine(s string) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > 100 {
		r = append(r[:97], '…')
	}
	return string(r)
}
