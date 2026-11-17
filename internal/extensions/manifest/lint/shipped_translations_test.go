// SPDX-License-Identifier: Apache-2.0

package contentlint

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

// pendingPacks are packs whose translations their owning extension ships
// itself, in its own piece of work; listed so the test names what remains.
var pendingPacks = map[string]string{
	"internal/extensions/planification/contrib/embed/cwm": "planification ships its own translations",
}

// Every shipped CWM pack translates every text a person reads of its object
// types, panels, capabilities, workflows, gates, gate requirements, aspects,
// policies, edge types, hook types, agent contexts and data-types (the
// labels of their fields) into
// every supported language other than its source: no `translation-missing`,
// no error. An object type also carries a label — without one the GUI shows
// its raw name.
func TestShippedPacks_TranslatedForEveryLanguage(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	sys, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	var packs []string
	for _, pattern := range []string{"app/*/embed/cwm", "internal/extensions/*/contrib/embed/cwm"} {
		m, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		packs = append(packs, m...)
	}
	sort.Strings(packs)
	if len(packs) < 10 {
		t.Fatalf("found %d packs under %s; the walk is broken", len(packs), root)
	}
	checked := 0
	kinds := map[string]int{}
	for _, pack := range packs {
		rel, _ := filepath.Rel(root, pack)
		rel = filepath.ToSlash(rel)
		if why, ok := pendingPacks[rel]; ok {
			t.Logf("%s: skipped (%s)", rel, why)
			continue
		}
		packLang := PackSourceLangOnDisk(filepath.Join(pack, "x"))
		files, err := translatableFiles(pack)
		if err != nil {
			t.Fatal(err)
		}
		for _, tf := range files {
			paths := sys.Translatable(tf.kind)
			if len(paths) == 0 {
				t.Fatalf("kind %s declares nothing translatable", tf.kind)
			}
			data, err := os.ReadFile(tf.path)
			if err != nil {
				t.Fatal(err)
			}
			var tr []byte
			if b, err := os.ReadFile(translationsFileOf(tf.path)); err == nil {
				tr = b
			}
			name, _ := filepath.Rel(root, tf.path)
			name = filepath.ToSlash(name)
			checked++
			kinds[tf.kind]++
			for _, finding := range LintTranslationsFiles(paths, data, tr, packLang) {
				if finding.Severity == verify.SeverityError || finding.Rule == "translation-missing" {
					t.Errorf("%s: %s: %s", name, finding.Rule, finding.Message)
				}
			}
			if tf.kind == "object-type" {
				var ot struct {
					Label string `yaml:"label"`
				}
				if yaml.Unmarshal(data, &ot) == nil && ot.Label == "" {
					t.Errorf("%s: an object type without a label shows its raw name", name)
				}
			}
		}
	}
	// Every kind a pack folder holds is walked: a kind whose folder the walk
	// misses would pass by checking nothing.
	for folder, kind := range CWMPackFolderKinds {
		if kinds[kind] == 0 {
			t.Errorf("no %s checked (folder %s): the walk misses it", kind, folder)
		}
	}
	if checked < 50 {
		t.Fatalf("checked only %d declarations; the walk is broken", checked)
	}
}
