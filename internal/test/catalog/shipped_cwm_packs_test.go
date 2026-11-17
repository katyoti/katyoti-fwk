// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	ocapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// TestEveryShippedCWMDeclarationReadsBackFromTheCatalog: every shipped CWM
// pack (a binary's embed/cwm, an extension's contrib/embed/cwm), published by
// the pack reader as production publishes it (the generic producer, each kind
// from the folder its declaration names), is read back by CWM's own reader
// (cwmapi.DeclarationsFromManifests, the strict codec) with no finding and
// nothing dropped; the kinds an extension reads decode into their types with
// no key left over.
func TestEveryShippedCWMDeclarationReadsBackFromTheCatalog(t *testing.T) {
	root := repoRoot(t)
	packs, _ := filepath.Glob(filepath.Join(root, "app", "*", "embed", "cwm"))
	ext, _ := filepath.Glob(filepath.Join(root, "internal", "extensions", "*", "contrib", "embed", "cwm"))
	packs = append(packs, ext...)
	if len(packs) < 10 {
		t.Fatalf("found %d shipped packs, want every one", len(packs))
	}

	// One provider name per pack: each is read alone, as its own host reads it.
	exts := []application.Extension{func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", ".providers", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
	}}
	for _, p := range packs {
		name := filepath.Base(filepath.Dir(filepath.Dir(p))) // app/<name>/embed/cwm
		if name == "contrib" {                               // internal/extensions/<name>/contrib/embed/cwm
			name = filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(p))))
		}
		exts = append(exts, manifestcontrib.ContributePackPart("pack-"+name, name, "cwm", os.DirFS(p)))
	}
	app := application.New(application.Config(), exts...)
	if err := app.Run(); err != nil {
		t.Fatalf("the shipped packs do not boot: %v", err)
	}

	byProvider := map[string]*mbapi.Manifest{}
	published := map[string]int{} // kind → entries
	for _, prov := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		entries, err := prov.Enumerate(context.Background())
		if err != nil {
			t.Fatalf("%s %s: %v", prov.Provider(), prov.Type(), err)
		}
		m := byProvider[prov.Provider()]
		if m == nil {
			m = &mbapi.Manifest{Provider: prov.Provider()}
			byProvider[prov.Provider()] = m
		}
		m.Contributions = append(m.Contributions, entries...)
		published[prov.Type()] += len(entries)
		for _, c := range entries {
			if err := strictly(prov.Type(), c.Data()); err != nil {
				t.Errorf("%s:/%s/%s: %v", c.Provider, c.Type, c.ID, err)
			}
		}
	}
	var manifests []*mbapi.Manifest
	for _, m := range byProvider {
		manifests = append(manifests, m)
	}

	set := cwmapi.DeclarationsFromManifests(manifests, nil)
	for _, f := range set.Findings {
		t.Errorf("CWM's reader: %+v", f)
	}
	read := map[string]int{
		"edge-type": len(set.Edges), "object-type": len(set.Objects), "capability": len(set.Capabilities),
		"aspect": len(set.Aspects), "workflow": len(set.Workflows), "policy": len(set.Policies),
		"hook-type": len(set.HookTypes), "hook": len(set.Hooks), "event-type": len(set.EventTypes),
	}
	for kind, n := range read {
		if n != published[kind] {
			t.Errorf("%s: %d published, %d read back by CWM", kind, published[kind], n)
		}
	}
	if published["object-type"] == 0 || published["object-panel"] == 0 || published["gate"] == 0 || published["agent-context"] == 0 {
		t.Fatalf("published = %v; want every kind of the shipped packs", published)
	}
}

// strictly decodes the data of a kind an extension reads into its type,
// refusing a key no field takes; CWM's own kinds are DeclarationsFromManifests'.
func strictly(kind string, data []byte) error {
	var into any
	switch kind {
	case paneapi.ObjectPanelKind:
		into = new(paneapi.PanelDecl)
	case ocapi.AgentContextKind:
		into = new(ocapi.AgentFraming)
	case gatesapi.GateKind:
		into = new(gatesapi.Gate)
	case gatesapi.GateRequirementKind:
		into = new(gatesapi.GateRequirement)
	default:
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(into)
}
