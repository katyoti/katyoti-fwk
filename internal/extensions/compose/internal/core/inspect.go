// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// inspect.go — `ktw-compose inspect`: one entry of ANY kind, as the model holds
// it. The generic minimum every kind gets (ms-contribution-ext e22): the entry,
// the kind that governs it, its layers with hash and band, its producer's
// identity, its provenance and its verdict.
//
// There is no kind name in this file and no branch on one. Every question about
// the kind goes to contribapi.Kinds, built from the kind declarations the
// source itself carries — a compiled sidecar's manifest (its system provider
// and its own packs), or the universe's registry (mbapi.KindsAt). A kind a pack
// declares is therefore inspected exactly like a kind the framework ships.

// inspector is the api.Inspector over a Service. A separate type on purpose:
// every exported method of *Service is published as a KTSP method and a flat
// CLI verb (reflection over the concrete type), and the verb's YAML rendering
// is the hand-written `inspect` command's (compose/local), not a generated one.
type inspector struct{ s *Service }

// NewInspector returns the Inspector the `inspect` command runs. It reads
// compiled sidecars as a process and the universe from the binary's catalog.
func NewInspector() api.Inspector { return inspector{s: &Service{}} }

// Inspect implements api.Inspector.
func (i inspector) Inspect(ctx context.Context, in api.InspectInput) (api.InspectOutput, error) {
	return i.s.inspect(ctx, in)
}

func (s *Service) inspect(ctx context.Context, in api.InspectInput) (api.InspectOutput, error) {
	if in.URI == "" {
		return api.InspectOutput{}, fmt.Errorf("inspect: --uri is required")
	}
	parsed, err := contribapi.ParseResourceURI(in.URI)
	if err != nil {
		return api.InspectOutput{}, fmt.Errorf("inspect: %q: %w", in.URI, err)
	}
	switch {
	case in.Exe != "" && in.Universe:
		return api.InspectOutput{}, fmt.Errorf("inspect: give --exe or --universe, not both")
	case in.Exe != "":
		return s.inspectExe(ctx, in.Exe, parsed)
	case in.Universe:
		return s.inspectUniverse(ctx, parsed)
	}
	return api.InspectOutput{}, fmt.Errorf("inspect: say where to read %s: --exe <sidecar> or --universe", in.URI)
}

// inspectExe reads a compiled sidecar through its own manifest verb — the read
// the lint verbs use (ExeReader), in a throwaway workspace. The manifest carries
// the kind declarations (system and the binary's own) and the provider rows.
func (s *Service) inspectExe(ctx context.Context, exe string, uri contribapi.ResourceURI) (api.InspectOutput, error) {
	m, err := s.exeReader().Manifest(ctx, exe)
	if err != nil {
		return api.InspectOutput{}, err
	}
	want := uri.String()
	for _, c := range m.Contributions {
		if c.ResolvedURI() != want {
			continue
		}
		out := describe(c, contribapi.KindsFrom(m.Contributions), "exe:"+exe)
		out.Provider = providerRows(c, m)
		if out.Verdict.Valid == nil {
			out.Verdict.Note = "a verdict is recorded when an entry enters a universe view; a single binary's manifest carries none — inspect with --universe to read it"
		}
		return out, nil
	}
	return api.InspectOutput{}, fmt.Errorf("inspect: %s does not publish %s", exe, want)
}

// inspectUniverse reads the aggregate at universe reach: the envelope through
// mbapi.Get, the kinds through mbapi.KindsAt, the provider rows from the
// provider's manifest as the aggregate holds it.
func (s *Service) inspectUniverse(ctx context.Context, uri contribapi.ResourceURI) (api.InspectOutput, error) {
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: s.universeOverride}
	if q.Catalog == nil {
		cat, err := mbapiCatalog(ctx)
		if err != nil {
			return api.InspectOutput{}, err
		}
		q.Catalog = cat
	}
	c, err := mbapi.Get(ctx, q, uri.String())
	if err != nil {
		return api.InspectOutput{}, fmt.Errorf("inspect: %w", err)
	}
	kinds, err := mbapi.KindsAt(ctx, q)
	if err != nil {
		return api.InspectOutput{}, fmt.Errorf("inspect: kind registry: %w", err)
	}
	out := describe(c, kinds, "universe")
	switch m, merr := providerManifest(ctx, q.Catalog, c.Provider); {
	case c.Provider == contribapi.SystemProvider:
		out.Provider = api.InspectProvider{Name: c.Provider,
			Note: "the meta-model every binary embeds: no manifest of its own, immutable by construction"}
	case merr != nil:
		out.Provider = api.InspectProvider{Name: c.Provider, Note: "provider manifest unavailable: " + merr.Error()}
	default:
		out.Provider = providerRows(c, m)
	}
	return out, nil
}

// mbapiCatalog resolves the binary's universe view — the aggregate
// manifestuniverse.UniverseCatalogExtension publishes in ktw-compose.
func mbapiCatalog(ctx context.Context) (mbapi.CatalogReader, error) {
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil {
		return nil, fmt.Errorf("inspect: --universe needs this binary's universe catalog: %w", err)
	}
	return cat, nil
}

// describe is the one body both sources share: everything about the entry that
// the envelope and the kind registry answer.
func describe(c contribapi.Contribution, kinds contribapi.Kinds, source string) api.InspectOutput {
	out := api.InspectOutput{URI: c.ResolvedURI(), Source: source}
	out.Kind = kindOf(kinds, c.Type)
	if c.Metadata != nil {
		out.Meta = asMap(c.Metadata)
	}
	decl, _ := kinds.Get(c.Type)
	out.Layers = layersOf(c, decl, out.Kind.DefaultLayer)
	if c.Provenance != nil {
		out.Provenance = &api.InspectProv{
			Signed:         c.Signed(),
			IdentityDigest: c.IdentityDigest(),
			KeyID:          c.Provenance.KeyID,
			Publisher:      c.Provenance.Publisher,
			Signature:      c.Provenance.Signature,
		}
	}
	out.Verdict = api.InspectVerdict{State: "unchecked", Valid: c.Valid, Errors: c.ValidationErrors}
	if c.Valid != nil {
		out.Verdict.State = "invalid"
		if *c.Valid {
			out.Verdict.State = "valid"
		}
	}
	return out
}

// kindOf renders the registry's answer about one kind.
func kindOf(kinds contribapi.Kinds, kind string) api.InspectKind {
	k := api.InspectKind{
		Name:         kind,
		Schema:       kinds.SchemaFor(kind),
		DefaultLayer: kinds.DefaultLayer(kind),
		Budget:       kinds.Budget(kind),
		Execution:    string(kinds.Execution(kind)),
		DeclaredBy:   kinds.DeclaredBy(kind),
	}
	decl, ok := kinds.Get(kind)
	if !ok {
		if !contribapi.IsMetaKind(kind) {
			k.Findings = []string{"no declaration in this view"}
		}
		return k
	}
	k.Declared = true
	k.Validate = decl.Severity()
	for name, l := range decl.Layers {
		k.Layers = append(k.Layers, api.InspectKindLayer{
			Name: name, MediaType: l.MediaType, Required: l.Required, Default: l.Default, Resident: l.Resident,
		})
	}
	sort.Slice(k.Layers, func(i, j int) bool { return k.Layers[i].Name < k.Layers[j].Name })
	for _, f := range kinds.Check() {
		if f.Kind == kind {
			k.Findings = append(k.Findings, f.Message)
		}
	}
	return k
}

// layersOf joins what the kind declares with what the entry carries, default
// layer first, then by name.
func layersOf(c contribapi.Contribution, decl contribapi.KindDeclaration, def string) []api.InspectLayer {
	names := map[string]bool{}
	for n := range decl.Layers {
		names[n] = true
	}
	for n := range c.Layers {
		names[n] = true
	}
	out := make([]api.InspectLayer, 0, len(names))
	for n := range names {
		l := api.InspectLayer{Name: n}
		if ld, ok := decl.Layers[n]; ok {
			l.Declared = true
			l.MediaType = ld.MediaType
		}
		if ref, ok := c.Layers[n]; ok {
			l.Carried = true
			l.Hash, l.Size, l.Band = ref.Hash, ref.Size, string(ref.Band())
			l.Inline = len(ref.Inline) > 0
			if ref.MediaType != "" {
				l.MediaType = ref.MediaType
			}
		}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Name == def) != (out[j].Name == def) {
			return out[i].Name == def
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// providerRows picks the entry's producer identity out of a manifest: the rows
// recorded for this provider and this kind.
func providerRows(c contribapi.Contribution, m *mbapi.Manifest) api.InspectProvider {
	p := api.InspectProvider{Name: c.Provider}
	if m == nil {
		p.Note = "no manifest"
		return p
	}
	for _, r := range m.Providers {
		if r.Name == c.Provider && r.Type == c.Type {
			p.Identity = append(p.Identity, api.InspectIdentity{Type: r.Type, Digest: r.Digest, Mutable: r.Mutable})
		}
	}
	if len(p.Identity) == 0 {
		p.Note = "the manifest records no identity for this provider and kind"
	}
	return p
}

// providerManifest asks the view for one provider's manifest. A remote reader
// names it Manifest, an in-process catalog Get; a reader that has neither says
// so rather than guessing.
func providerManifest(ctx context.Context, r mbapi.CatalogReader, provider string) (*mbapi.Manifest, error) {
	switch v := r.(type) {
	case interface {
		Manifest(context.Context, string) (*mbapi.Manifest, error)
	}:
		return v.Manifest(ctx, provider)
	case interface {
		Get(context.Context, string) (*mbapi.Manifest, error)
	}:
		return v.Get(ctx, provider)
	}
	return nil, fmt.Errorf("this view cannot name a provider's manifest")
}

// asMap renders a value through its JSON form — the metadata as published,
// under its wire names, with no field list of its own to drift.
func asMap(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}
