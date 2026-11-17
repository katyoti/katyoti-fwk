// SPDX-License-Identifier: Apache-2.0

package pptx

import (
	"fmt"
	"path/filepath"

	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// Import reads a template, proposes its theme with the decisions applied,
// and — when out is set — writes the theme into the pack rooted there
// (<out>/deck-themes/<name>.yaml and <out>/deck-assets/<name>/; the report is
// returned, never written beside the theme: a pack's deck folders hold
// declarations and asset folders only). The CLI and the
// DeckService both run it. source names the template in the report.
func Import(data []byte, o Options, source, out string) (deckapi.ImportThemeOutput, error) {
	p, res, err := Prepare(data, o, source)
	if err != nil {
		return res, err
	}
	if out == "" {
		return res, nil
	}
	if err := p.Write(out); err != nil {
		return res, err
	}
	decl, assets := p.Paths(out)
	res.Written = append(res.Written, decl)
	for _, f := range sortedKeys(p.Files) {
		res.Written = append(res.Written, filepath.Join(assets, filepath.FromSlash(f)))
	}
	return res, nil
}

// Prepare reads a template and proposes its theme with the decisions
// applied: the proposal (its declaration and files) and what a dry run
// reports. A host that stores the theme elsewhere (a project object) writes
// the proposal's files itself.
func Prepare(data []byte, o Options, source string) (*Proposal, deckapi.ImportThemeOutput, error) {
	var res deckapi.ImportThemeOutput
	inv, err := Read(data)
	if err != nil {
		return nil, res, fmt.Errorf("%s: %w", source, err)
	}
	p, err := Propose(inv, o)
	if err != nil {
		return nil, res, err
	}
	y, err := p.YAML()
	if err != nil {
		return nil, res, err
	}
	res.Report, res.Mapping, res.Notes, res.Declaration = p.Report(inv, source), p.Mapping, p.Notes, string(y)
	return p, res, nil
}
