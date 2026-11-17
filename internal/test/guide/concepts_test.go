// SPDX-License-Identifier: Apache-2.0

package guide

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// internalRef is what a document written for users never names: a decision,
// rule, request or feature id, a source file, a module path or a repository
// page link.
var internalRef = regexp.MustCompile(`\b(OD|RQ|R|F|AC|TD|IN|RL)-\d{3}\b|\.go\b|internal/|\.cwm\.md|\]\(\.\./`)

// TestConceptDocumentsShipForUsers: KaTyoti ships its first concept documents
// from the hub - projects, repositories and universes; universes, solutions
// and sidecar versions - marked concept, offered in every universe
// (katyoti-usage), in English and French, with no internal reference.
func TestConceptDocumentsShipForUsers(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "app", "ktw-hub", "embed", "guide")
	for _, id := range []string{"concept-projects-repositories-and-universes", "concept-universes-solutions-and-sidecar-versions", "concept-project-and-module-types"} {
		raw, err := os.ReadFile(filepath.Join(dir, id+".yaml"))
		if err != nil {
			t.Fatalf("%s: not shipped: %v", id, err)
		}
		var g struct {
			Concept  bool     `yaml:"concept"`
			Hashtags []string `yaml:"hashtags"`
			Body     string   `yaml:"body"`
		}
		if err := config.DecodeBytes(raw, marshal.YAML, &g); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if !g.Concept {
			t.Errorf("%s: not marked concept", id)
		}
		usage := false
		for _, h := range g.Hashtags {
			usage = usage || h == "katyoti-usage"
		}
		if !usage {
			t.Errorf("%s: not offered in every universe (no katyoti-usage)", id)
		}
		tr, err := os.ReadFile(filepath.Join(dir, id+".i18n.yaml"))
		if err != nil {
			t.Fatalf("%s: no translations: %v", id, err)
		}
		var langs map[string]struct {
			Body string `yaml:"body"`
		}
		if err := config.DecodeBytes(tr, marshal.YAML, &langs); err != nil {
			t.Fatalf("%s: translations: %v", id, err)
		}
		for lang, body := range map[string]string{"en": g.Body, "fr": langs["fr"].Body} {
			if strings.Count(body, "\n## ") < 3 {
				t.Errorf("%s (%s): the document is missing or short", id, lang)
			}
			if m := internalRef.FindAllString(body, -1); len(m) > 0 {
				t.Errorf("%s (%s): names internal references %v", id, lang, m)
			}
		}
	}
}

// TestConceptGuidesSayWhatIsDelivered: the concept guides describe KaTyoti as
// delivered, never a model it is moving to (no « Today and the target »
// section), and the guide on projects, repositories and universes leads to the
// guide on project types - in English and French.
func TestConceptGuidesSayWhatIsDelivered(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "app", "ktw-hub", "embed", "guide")
	read := func(id string) map[string]string {
		raw, err := os.ReadFile(filepath.Join(dir, id+".yaml"))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		var g struct {
			Body string `yaml:"body"`
		}
		if err := config.DecodeBytes(raw, marshal.YAML, &g); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		tr, err := os.ReadFile(filepath.Join(dir, id+".i18n.yaml"))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		var langs map[string]struct {
			Body string `yaml:"body"`
		}
		if err := config.DecodeBytes(tr, marshal.YAML, &langs); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		return map[string]string{"en": g.Body, "fr": langs["fr"].Body}
	}
	target := map[string][]string{
		"en": {"Today and the target", "The model KaTyoti is moving to"},
		"fr": {"Aujourd'hui et la cible", "Le modèle vers lequel KaTyoti évolue"},
	}
	for _, id := range []string{"concept-projects-repositories-and-universes", "concept-universes-solutions-and-sidecar-versions", "concept-project-and-module-types"} {
		for lang, body := range read(id) {
			for _, s := range target[lang] {
				if strings.Contains(body, s) {
					t.Errorf("%s (%s): describes a target model (%q), not what is delivered", id, lang, s)
				}
			}
		}
	}
	leads := map[string][]string{
		"en": {"« Project and module types »"},
		"fr": {"« Types de projets et de modules »"},
	}
	for lang, body := range read("concept-projects-repositories-and-universes") {
		for _, s := range leads[lang] {
			if !strings.Contains(body, s) {
				t.Errorf("concept-projects-repositories-and-universes (%s): does not lead to %s", lang, s)
			}
		}
	}
}

// TestProjectsGuideExplainsRepositoriesAndMembers: the concept guide on
// projects, repositories and universes explains, in English and French, that a
// repository is one project and a project never holds another, that a universe
// attaches several repositories as its members and serves their projects at
// once, beside other members only a repository created on this PC - no guide
// shows a repository holding several projects - and the
// concept guides name no project group, the kind that grouped projects before
func TestProjectsGuideExplainsRepositoriesAndMembers(t *testing.T) {
	dir := filepath.Join(repoRoot(t), "app", "ktw-hub", "embed", "guide")
	read := func(id string) map[string]string {
		raw, err := os.ReadFile(filepath.Join(dir, id+".yaml"))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		var g struct {
			Description string `yaml:"description"`
			Body        string `yaml:"body"`
		}
		if err := config.DecodeBytes(raw, marshal.YAML, &g); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		tr, err := os.ReadFile(filepath.Join(dir, id+".i18n.yaml"))
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		var langs map[string]struct {
			Description string `yaml:"description"`
			Body        string `yaml:"body"`
		}
		if err := config.DecodeBytes(tr, marshal.YAML, &langs); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		return map[string]string{"en": g.Description + "\n" + g.Body, "fr": langs["fr"].Description + "\n" + langs["fr"].Body}
	}
	says := map[string][]string{
		"en": {"A repository is one project", "A project never holds another project", "attaches several repositories as its members", "serves every project of its members at once", "only a repository created on this PC", "| **Repository** |"},
		"fr": {"Un dépôt est un projet", "Un projet ne contient jamais un autre projet", "rattache plusieurs dépôts comme ses membres", "sert à la fois tous les projets de ses membres", "seulement un dépôt créé sur ce poste", "| **Dépôt** |"},
	}
	// One repository is one project: no guide shows a
	// repository holding several projects.
	several := []string{"one or several projects", "holding several projects", "un ou plusieurs projets", "contient plusieurs projets", "several dossiers, each a project", "plusieurs dossiers, chacun un projet"}
	for _, id := range []string{"concept-projects-repositories-and-universes", "concept-project-and-module-types"} {
		for lang, body := range read(id) {
			for _, s := range several {
				if strings.Contains(body, s) {
					t.Errorf("%s (%s): still says %q - one repository is one project", id, lang, s)
				}
			}
		}
	}
	for lang, body := range read("concept-projects-repositories-and-universes") {
		for _, s := range says[lang] {
			if !strings.Contains(body, s) {
				t.Errorf("concept-projects-repositories-and-universes (%s): does not say %q", lang, s)
			}
		}
		// A universe serves every project of its members: no rule of one at a time.
		for _, s := range []string{"one project at a time", "un projet à la fois"} {
			if strings.Contains(body, s) {
				t.Errorf("concept-projects-repositories-and-universes (%s): still says %q", lang, s)
			}
		}
	}
	gone := []string{"project group", "Project group", "project-group", "groupe de projets", "Groupe de projets", "Groupes de projets"}
	for _, id := range []string{"concept-projects-repositories-and-universes", "concept-universes-solutions-and-sidecar-versions", "concept-project-and-module-types"} {
		for lang, body := range read(id) {
			for _, s := range gone {
				if strings.Contains(body, s) {
					t.Errorf("%s (%s): names %q, a kind KaTyoti no longer has", id, lang, s)
				}
			}
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "concept-project-groups.yaml")); err == nil {
		t.Error("the concept guide « Project groups » is still shipped")
	}
}
