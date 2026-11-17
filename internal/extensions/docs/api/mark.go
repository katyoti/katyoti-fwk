// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// The production mark (modules/doc-engine/, feature F-009): every document
// ArchiFactory produces says it was produced by ArchiFactory, with its
// version, the date, an optional classification and — for a deliverable not
// yet validated — where it stands. It is defined here once; each renderer
// (the doc renderers, slidedeck, drawio, confluence) asks MarkOf for it and
// only places its line in its own format. Beside the line a reader sees, the
// mark carries a Provenance a machine reads (the generator, the object the
// document was produced from, its version and the date), which no setting
// switches off (feature F-013).

// DefaultMarkText is the mark's text when the universe sets none.
const DefaultMarkText = DefaultMarkPrefix + " " + DefaultGenerator

// DefaultMarkPrefix opens the mark's default text, the generator after it.
const DefaultMarkPrefix = "Une production"

// DefaultGenerator is the product a provenance names when the universe sets
// none.
const DefaultGenerator = "ArchiFactory"

// ProductionMarkConfig is the universe's setting (ProductionMark.yaml): the
// mark is on unless Enabled is false; Text and Classification replace the
// defaults; Generator names the product in the provenance (ArchiFactory when
// empty). In a universe installed from a solution, the solution names the
// product and its version (SolutionOf), and Generator is not read.
type ProductionMarkConfig struct {
	Enabled        *bool  `yaml:"enabled"        json:"enabled,omitempty"`
	Text           string `yaml:"text"           json:"text,omitempty"`
	Classification string `yaml:"classification" json:"classification,omitempty"`
	Generator      string `yaml:"generator"      json:"generator,omitempty"`
}

// DocMark is what one document says of its mark, where its format allows
// (a markdown frontmatter, a render's input): Mark is on, off or a text of
// its own; Classification replaces the universe's; State is a deliverable's
// state (draft, in-review, validated, published); Origin and OriginVersion
// are the object the document was produced from (a study) and its version.
type DocMark struct {
	Mark           string `json:"mark,omitempty"           yaml:"mark,omitempty"           jsonschema:"description=on or off or a text of its own; on when omitted"`
	Classification string `json:"classification,omitempty" yaml:"classification,omitempty" jsonschema:"description=The document's classification such as Diffusion restreinte"`
	State          string `json:"state,omitempty"          yaml:"state,omitempty"          jsonschema:"description=A deliverable's state: draft or in-review or validated or published"`
	Origin         string `json:"origin,omitempty"         yaml:"origin,omitempty"         jsonschema:"description=The id of the object the document was produced from such as a study"`
	OriginVersion  string `json:"origin_version,omitempty" yaml:"origin_version,omitempty" jsonschema:"description=The version of the object the document was produced from"`
}

// Provenance is the part of the mark a machine reads: who generated the
// document, from which object and version, and when. It is answered whether
// or not the visible line is on.
type Provenance struct {
	Generator     string `json:"generator"`
	Origin        string `json:"origin,omitempty"`
	OriginVersion string `json:"origin_version,omitempty"`
	Date          string `json:"date"`
}

// ProductionMark is the mark one document carries.
type ProductionMark struct {
	Text           string     `json:"text"`
	Version        string     `json:"version,omitempty"`
	Date           string     `json:"date"`
	Classification string     `json:"classification,omitempty"`
	State          string     `json:"state,omitempty"`
	Provenance     Provenance `json:"provenance"`
}

// Line is the mark as one line of text, its parts joined by " · ".
func (m ProductionMark) Line() string {
	parts := []string{m.Text}
	if m.Version != "" {
		parts = append(parts, m.Version)
	}
	parts = append(parts, m.Date)
	for _, p := range []string{m.Classification, m.State} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, " · ")
}

// stateLabels names the states a reader must see: a document not yet
// validated says so; a validated or published one says nothing more.
var stateLabels = map[string]string{"draft": "Brouillon", "in-review": "En revue"}

// ResolveMark is the mark a document carries under a universe's setting;
// false when it shows no line (the universe or the document switched it off).
// Its Provenance is set either way: the generator names the product and, when
// binary is given, the sidecar that produced the document.
func ResolveMark(cfg ProductionMarkConfig, doc DocMark, version string, now time.Time, binary ...string) (ProductionMark, bool) {
	return ResolveMarkFor(cfg, Solution{}, doc, version, now, binary...)
}

// Solution is the solution the universe was installed from, as a mark names
// it: its display name and its version. Empty for a universe made by hand.
type Solution struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

// SolutionOf is the installed solution a mark names: the
// universe's Univers.yaml `solution` (`<name>@<version>`), its display name
// the installation's name when katyoti.yml was installed from the same
// solution, else the solution's name as written.
func SolutionOf(universeSolution string, inst *cenv.Installation) Solution {
	name, version := splitSolution(universeSolution)
	if name == "" {
		return Solution{}
	}
	sol := Solution{Name: name, Version: version}
	if inst != nil && strings.TrimSpace(inst.Name) != "" {
		if instName, _ := splitSolution(inst.Solution); strings.EqualFold(instName, name) {
			sol.Name = strings.TrimSpace(inst.Name)
		}
	}
	return sol
}

func splitSolution(ref string) (string, string) {
	ref = strings.TrimSpace(ref)
	if i := strings.LastIndex(ref, "@"); i >= 0 {
		return strings.TrimSpace(ref[:i]), strings.TrimSpace(ref[i+1:])
	}
	return ref, ""
}

// ResolveMarkFor is ResolveMark in a universe installed from a solution: the
// solution names the generator and its version is the one shown; with no
// solution, the universe's setting and the binary's version apply.
func ResolveMarkFor(cfg ProductionMarkConfig, sol Solution, doc DocMark, version string, now time.Time, binary ...string) (ProductionMark, bool) {
	if n := strings.TrimSpace(sol.Name); n != "" {
		cfg.Generator = n
		if cfg.Text == "" {
			cfg.Text = DefaultMarkPrefix + " " + n
		}
		if v := strings.TrimSpace(sol.Version); v != "" {
			version = v
		}
	}
	prov := Provenance{
		Generator:     DefaultGenerator,
		Origin:        strings.TrimSpace(doc.Origin),
		OriginVersion: strings.TrimSpace(doc.OriginVersion),
		Date:          now.UTC().Format(time.RFC3339),
	}
	if g := strings.TrimSpace(cfg.Generator); g != "" {
		prov.Generator = g
	}
	if len(binary) > 0 && strings.TrimSpace(binary[0]) != "" {
		prov.Generator += " (" + strings.TrimSpace(binary[0]) + ")"
	}
	if cfg.Enabled != nil && !*cfg.Enabled && doc.Mark != "on" {
		return ProductionMark{Provenance: prov}, false
	}
	m := ProductionMark{Text: DefaultMarkText, Date: now.Format("02/01/2006"), Classification: cfg.Classification, Provenance: prov}
	if cfg.Text != "" {
		m.Text = cfg.Text
	}
	switch strings.TrimSpace(doc.Mark) {
	case "off":
		return ProductionMark{Provenance: prov}, false
	case "", "on":
	default:
		m.Text = strings.TrimSpace(doc.Mark)
	}
	if doc.Classification != "" {
		m.Classification = doc.Classification
	}
	if v := strings.TrimSpace(version); v != "" && v != "dev" {
		m.Version = v
	}
	m.State = stateLabels[doc.State]
	return m, true
}

// MarkOf is the mark a document carries in this application: the universe's
// ProductionMark.yaml, this binary's name and version, today. Its Provenance
// is set even when it answers false.
func MarkOf(ctx context.Context, doc DocMark) (ProductionMark, bool) {
	cfg, err := application.UseConfig[ProductionMarkConfig](ctx)
	if err != nil {
		cfg = ProductionMarkConfig{}
	}
	name, version := binaryNameVersion(ctx)
	return ResolveMarkFor(cfg, installedSolution(application.Env()), doc, version, time.Now(), name)
}

// universeEnv is where the universe keeps its Univers.yaml: its own config
// root, which is not the project's home when the project is attached to a
// central universe (an installed solution's).
type universeEnv interface{ UniverseDir() string }

// installedSolution is the solution this universe was installed from, if any.
func installedSolution(env universeEnv) Solution {
	m, err := cenv.LoadUniverseManifest(env.UniverseDir())
	if err != nil || m == nil {
		return Solution{}
	}
	return SolutionOf(m.Solution, cenv.CurrentInstallation())
}

// DocMarkOf reads a document's mark settings from its metadata (a markdown
// frontmatter): mark, classification, state, origin, origin_version.
func DocMarkOf(meta map[string]any) DocMark {
	str := func(k string) string {
		switch v := meta[k].(type) {
		case string:
			return v
		case bool:
			if v {
				return "on"
			}
			return "off"
		case nil:
			return ""
		default:
			return fmt.Sprint(v)
		}
	}
	return DocMark{Mark: str("mark"), Classification: str("classification"), State: str("state"), Origin: str("origin"), OriginVersion: str("origin_version")}
}

// binaryNameVersion is this binary's name and version, off its cobra root
// (the source the catalog and the universe read too).
func binaryNameVersion(ctx context.Context) (string, string) {
	cr, err := application.UseNamedService[appcore.CobraCommand](ctx, "root")
	if err != nil || cr.Cmd == nil {
		return "", ""
	}
	return cr.Cmd.Name(), cr.Cmd.Version
}
