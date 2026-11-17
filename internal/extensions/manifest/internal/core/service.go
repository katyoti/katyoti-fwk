// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// ResourceServiceImpl is the concrete ResourceService impl. Wraps
// a Builder for GetManifest/List, walks providers for Get, OpenLayer,
// Read, Types and GetDataType (the 7-method contract).
//
// Shared between the in-process service (published per-binary via
// `appcore.PublishSimpleService`) and any future MessageBus-side
// adapter that exposes the same surface to remote callers.
type ResourceServiceImpl struct {
	builder *Builder
	// providers finds the producers of (provider, type). A seam so OpenLayer
	// can be tested without an Application; production uses FindProviders.
	providers func(ctx context.Context, provider, typ string) []contribapi.ContributionProvider

	// memo is the last build, kept while it cannot change (see manifest).
	memoMu sync.Mutex
	memo   *mbapi.Manifest
}

// NewResourceService creates a ResourceServiceImpl bound to the
// given binary identity. `provider` is the binary name; `version` is
// the build-time version string.
func NewResourceService(provider, version string) *ResourceServiceImpl {
	return &ResourceServiceImpl{builder: NewBuilder(provider, version), providers: FindProviders}
}

// manifest is the built manifest every self-reach verb reads.
//
// A build enumerates every provider and hashes the whole manifest's JSON. For
// a binary whose providers are ALL immutable the result cannot change for the
// process lifetime, so it is built once and kept; a single mutable provider
// (a content root, an overlay) makes the whole build live again, exactly as
// before. The memo belongs to the provider set it was built from: asked under
// a context that contributes a different number of providers, it is rebuilt.
// The slot surface a build adds (SlotIntrospector) is the KTSP endpoint's,
// registered at bootstrap and fixed after it, the same lifetime as the
// providers.
//
// What is handed out is a private copy: the entry slice and each entry's
// layer map are the caller's to narrow (GetManifest re-hashes a filtered
// manifest; List drops inline bytes), the memo stays whole.
func (s *ResourceServiceImpl) manifest(ctx context.Context) (*mbapi.Manifest, error) {
	s.memoMu.Lock()
	defer s.memoMu.Unlock()
	if s.memo != nil && len(application.UseExtensions[contribapi.ContributionProvider](ctx)) == len(s.memo.Providers) {
		return copyManifest(s.memo), nil
	}
	m, err := s.builder.Build(ctx)
	if err != nil {
		return nil, err
	}
	s.memo = nil
	if immutableSet(m) {
		s.memo = m
	}
	return copyManifest(m), nil
}

// immutableSet reports whether every provider that built m is immutable: the
// only set whose manifest a process may keep.
func immutableSet(m *mbapi.Manifest) bool {
	for _, p := range m.Providers {
		if p.Mutable {
			return false
		}
	}
	return true
}

// copyManifest returns m with its own entry slice and per-entry layer maps.
// Inline bytes are shared: nobody writes into a layer's bytes in place.
func copyManifest(m *mbapi.Manifest) *mbapi.Manifest {
	cp := *m
	cp.Contributions = make([]contribapi.Contribution, len(m.Contributions))
	for i, c := range m.Contributions {
		if c.Layers != nil {
			layers := make(map[string]contribapi.LayerRef, len(c.Layers))
			for k, v := range c.Layers {
				layers[k] = v
			}
			c.Layers = layers
		}
		cp.Contributions[i] = c
	}
	return &cp
}

// OpenLayer implements ResourceService.OpenLayer through the provider
// primitive: the kind's default layer is resolved here, from this binary's own
// kinds, and the named layer is opened by the provider (ContributionProvider.
// Open). The bulk band and a layer the producer addresses but does not carry
// are answered as a refusal that still names the layer's address.
func (s *ResourceServiceImpl) OpenLayer(ctx context.Context, in mbapi.OpenLayerInput) (mbapi.OpenLayerOutput, error) {
	uri, err := contribapi.ParseResourceURI(in.URI)
	if err != nil {
		return mbapi.OpenLayerOutput{}, err
	}
	providers := s.find(ctx, uri.Provider, uri.Type)
	if len(providers) == 0 {
		return mbapi.OpenLayerOutput{}, mbapi.ErrContributionNotFound{URI: in.URI}
	}
	layer := in.Layer
	if layer == "" {
		var derr error
		if layer, derr = s.defaultLayerOf(ctx, uri.Type, in.URI); derr != nil {
			return mbapi.OpenLayerOutput{}, derr
		}
	}
	for _, p := range providers {
		content, ref, oerr := readAll(p.Open(ctx, uri.ID, layer))
		switch {
		case oerr == nil:
			return mbapi.OpenLayerOutput{Ref: ref, Content: content}, nil
		case errors.Is(oerr, contribapi.ErrContributionNotFound{}):
			continue // this provider does not enumerate the id — try the next.
		}
		// The bulk band and a layer addressed but not carried travel as a
		// refusal that still names the layer's address; the text is the
		// typed error's own (mbapi.Refusal), so the wire and the reaches
		// say one thing.
		if text, ok := mbapi.Refusal(oerr); ok {
			return mbapi.OpenLayerOutput{Ref: ref, Refused: text}, nil
		}
		return mbapi.OpenLayerOutput{}, oerr
	}
	return mbapi.OpenLayerOutput{}, mbapi.ErrContributionNotFound{URI: in.URI}
}

// find is the provider lookup, through the test seam when one is set.
func (s *ResourceServiceImpl) find(ctx context.Context, provider, typ string) []contribapi.ContributionProvider {
	if s.providers != nil {
		return s.providers(ctx, provider, typ)
	}
	return FindProviders(ctx, provider, typ)
}

// readAll drains an Open result into bytes, closing the reader. The wire
// carries whole layers up to the bulk band, so the service materialises
// what the provider streamed.
func readAll(rc io.ReadCloser, ref contribapi.LayerRef, err error) ([]byte, contribapi.LayerRef, error) {
	if err != nil {
		return nil, ref, err
	}
	defer rc.Close()
	b, rerr := io.ReadAll(rc)
	if rerr != nil {
		return nil, ref, rerr
	}
	return b, ref, nil
}

// openReadLayer opens the layer Read answers with: the one mbapi.ReadLayer
// names for the entry this provider enumerates under id, through the
// provider primitive.
func openReadLayer(ctx context.Context, p contribapi.ContributionProvider, id string) ([]byte, contribapi.LayerRef, error) {
	items, err := p.Enumerate(ctx)
	if err != nil {
		return nil, contribapi.LayerRef{}, err
	}
	for _, c := range items {
		if c.ID == id {
			return readAll(p.Open(ctx, id, mbapi.ReadLayer(c)))
		}
	}
	return nil, contribapi.LayerRef{}, contribapi.ErrContributionNotFound{URI: p.Provider() + ":/" + p.Type() + "/" + id}
}

// defaultLayerOf reads the kind's default layer from this binary's own
// kind declarations; `source` when the kind is unknown here (the convention
// KindDeclaration.DefaultLayer applies to an undeclared default).
// defaultLayerOf answers the kind's declared default layer from the
// declarations THIS producer carries: the system set and its own. A kind
// declared by another pack has no default here: the producer refuses with
// ErrLayerNotDeclared naming the missing declaration rather than guessing
// `source` (re-audit N13, 2026-09-26); the universe view, which sees every
// declaration, resolves it.
func (s *ResourceServiceImpl) defaultLayerOf(ctx context.Context, kind, uri string) (string, error) {
	var decls []contribapi.Contribution
	for _, p := range s.find(ctx, contribapi.SystemProvider, contribapi.ResourceTypeContributionType) {
		if items, err := p.Enumerate(ctx); err == nil {
			decls = append(decls, items...)
		}
	}
	for _, p := range s.find(ctx, s.builder.provider, contribapi.ResourceTypeContributionType) {
		if items, err := p.Enumerate(ctx); err == nil {
			decls = append(decls, items...)
		}
	}
	if layer := contribapi.KindsFrom(decls).DefaultLayer(kind); layer != "" {
		return layer, nil
	}
	return "", fmt.Errorf("open %s: this producer carries no declaration of kind %q, so it has no default layer (name the layer, or open at universe reach): %w",
		uri, kind, contribapi.ErrLayerNotDeclared{URI: uri, Layer: ""})
}

// GetManifest implements ResourceService.GetManifest. Returns this
// binary's full content-addressed Manifest. Used by ktw-hub's catalog
// refresh to cache a complete descriptor in one round-trip.
//
// IncludeSystem=false strips the embedded `system` meta-model entries
// (identical in every binary) and re-hashes — the hub requests this for
// every sidecar except its one canonical system carrier so the universe
// catalog holds the 28 system entries exactly once. IncludeLocal=false
// strips `Provider == "local"` binary-private entries the same way.
func (s *ResourceServiceImpl) GetManifest(ctx context.Context, in mbapi.ContributionGetManifestInput) (mbapi.ContributionGetManifestOutput, error) {
	m, err := s.manifest(ctx)
	if err != nil {
		return mbapi.ContributionGetManifestOutput{}, err
	}
	// Same visibility rules as List, through the same implementation. Here
	// both default to INCLUDE — a manifest fetch with nothing asked for is
	// the whole manifest — so only an explicit false drops anything.
	vis := mbapi.Filter{IncludeSystem: mbapi.IncludeYes, IncludeLocal: mbapi.IncludeYes}
	if in.IncludeSystem != nil && !*in.IncludeSystem {
		vis.IncludeSystem = mbapi.IncludeNo
	}
	if in.IncludeLocal != nil && !*in.IncludeLocal {
		vis.IncludeLocal = mbapi.IncludeNo
	}
	if kept := vis.Apply(m.Contributions, mbapi.ReachSelf); len(kept) != len(m.Contributions) {
		m.Contributions = kept
		ReHash(m)
	}
	return mbapi.ContributionGetManifestOutput{Manifest: m}, nil
}

// List implements ResourceService.List. Returns Contributions
// matching the filter; bodies are stripped unless in.WithBody is true.
// One method answers both the headers-only and the with-body listing
// (the command-surface decision).
func (s *ResourceServiceImpl) List(ctx context.Context, in mbapi.ContributionListInput) (mbapi.ContributionListOutput, error) {
	m, err := s.manifest(ctx)
	if err != nil {
		return mbapi.ContributionListOutput{}, err
	}
	// Visibility is decided by mbapi.Filter, the same rules the Go query
	// surface applies — one implementation, so the wire and in-process views
	// cannot drift on what "system" and "local" mean.
	//
	// Both defaults are stated explicitly here rather than left to the
	// reach: over the wire, IncludeLocal defaults FALSE so remote callers
	// never see binary-private artifacts, and the local CLI passes TRUE to
	// see everything the binary publishes.
	vis := mbapi.Filter{
		IncludeSystem: mbapi.IncludeYes,
		IncludeLocal:  mbapi.IncludeNo,
	}
	if in.IncludeSystem != nil && !*in.IncludeSystem {
		vis.IncludeSystem = mbapi.IncludeNo
	}
	if in.IncludeLocal {
		vis.IncludeLocal = mbapi.IncludeYes
	}

	matched := make([]contribapi.Contribution, 0, len(m.Contributions))
	for _, c := range m.Contributions {
		if !contributionMatchesFilter(c, in.Filter) {
			continue
		}
		if !in.WithBody {
			c.DropInline()
		}
		matched = append(matched, c)
	}
	return mbapi.ContributionListOutput{Contributions: vis.Apply(matched, mbapi.ReachSelf)}, nil
}

// Get implements ResourceService.Get. Fetches one Contribution by
// URI. Returns ErrContributionNotFound when no provider serves the URI.
func (s *ResourceServiceImpl) Get(ctx context.Context, in mbapi.ContributionGetInput) (mbapi.ContributionGetOutput, error) {
	uri, err := contribapi.ParseResourceURI(in.URI)
	if err != nil {
		return mbapi.ContributionGetOutput{}, err
	}
	// First check the Builder's manifest — if the Contribution was
	// enumerated with its Body inline (the typical case for skills, tasks,
	// data-types), return it directly.
	m, err := s.manifest(ctx)
	if err != nil {
		return mbapi.ContributionGetOutput{}, err
	}
	for _, c := range m.Contributions {
		if c.Provider == uri.Provider && c.Type == uri.Type && c.ID == uri.ID {
			// get is the METADATA view: identity + Metadata + Schema, with
			// the data layer dropped. The content is the Read lane (raw
			// bytes per type — markdown for skills, file bytes for
			// references, full JSON for tasks, YAML for data-types).
			c.DropInline()
			return mbapi.ContributionGetOutput{Contribution: c}, nil
		}
	}
	// Fall back to the providers to confirm the entry exists (lazy entries
	// not inlined in the manifest). Still metadata-only — no Body. Try every
	// provider for (provider, type): a virtual provider may serve a URI a
	// file-backed provider (registered first) doesn't. The entry exists when
	// its read layer opens — or is refused for a reason only an enumerated
	// entry can have (bulk band, bytes not carried, no such layer).
	providers := s.find(ctx, uri.Provider, uri.Type)
	if len(providers) == 0 {
		return mbapi.ContributionGetOutput{}, mbapi.ErrContributionNotFound{URI: in.URI}
	}
	var lastErr error = mbapi.ErrContributionNotFound{URI: in.URI}
	for _, p := range providers {
		if _, _, oerr := openReadLayer(ctx, p, uri.ID); oerr != nil && !entryExists(oerr) {
			lastErr = oerr
			continue
		}
		return mbapi.ContributionGetOutput{Contribution: contribapi.Contribution{
			Provider: uri.Provider,
			Type:     uri.Type,
			ID:       uri.ID,
			URI:      in.URI,
		}}, nil
	}
	return mbapi.ContributionGetOutput{}, lastErr
}

// entryExists reports whether an Open error could only come from an entry
// the provider enumerates: its layer is bulk, addressed but not carried, or
// simply not one it declares.
func entryExists(err error) bool {
	var bulk contribapi.ErrBulkLayer
	var notCarried contribapi.ErrLayerNotCarried
	var undeclared contribapi.ErrLayerNotDeclared
	return errors.As(err, &bulk) || errors.As(err, &notCarried) || errors.As(err, &undeclared)
}

// Read implements ResourceService.Read — the raw-source compatibility path on
// the wire: the content of the layer mbapi.ReadLayer names (source when
// declared, else data), opened through the owning provider's Open, with the
// layer's own media type. References enumerate their bytes as the source
// layer and the index drops them, so their content is reachable here.
func (s *ResourceServiceImpl) Read(ctx context.Context, in mbapi.ContributionReadInput) (mbapi.ContributionReadOutput, error) {
	uri, err := contribapi.ParseResourceURI(in.URI)
	if err != nil {
		return mbapi.ContributionReadOutput{}, err
	}
	// A (provider, type) pair can be served by more than one provider — a
	// file-backed provider AND a virtual one (e.g. ktw-agents serves
	// "data-type" from both the embed provider and the method-I/O provider).
	// Try each until one serves the URI, so a virtual contribution (a service
	// method's Input/Output schema, an in-memory persona, …) is never shadowed
	// by a file-backed provider that's merely registered first. Read MUST
	// always return content for an advertised URI — virtual or not.
	providers := s.find(ctx, uri.Provider, uri.Type)
	if len(providers) == 0 {
		return mbapi.ContributionReadOutput{}, mbapi.ErrContributionNotFound{URI: in.URI}
	}
	var lastErr error
	for _, p := range providers {
		content, ref, oerr := openReadLayer(ctx, p, uri.ID)
		if oerr != nil {
			lastErr = oerr
			continue // this provider doesn't serve the URI — try the next.
		}
		return mbapi.ContributionReadOutput{
			URI:       in.URI,
			MediaType: ref.MediaType,
			Content:   content,
		}, nil
	}
	return mbapi.ContributionReadOutput{}, fmt.Errorf("read %s: %w", in.URI, lastErr)
}

// Types implements ResourceService.Types. Returns the distinct
// Type discriminators this binary contributes.
func (s *ResourceServiceImpl) Types(ctx context.Context) ([]string, error) {
	m, err := s.manifest(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, c := range m.Contributions {
		if _, ok := seen[c.Type]; ok {
			continue
		}
		seen[c.Type] = struct{}{}
		out = append(out, c.Type)
	}
	return out, nil
}

// GetDataType implements ResourceService.GetDataType. Returns the
// canonical DSL Schema body of a data-type the sidecar OWNS by name.
// Empty Body + nil error means "this sidecar doesn't own that data-type".
func (s *ResourceServiceImpl) GetDataType(ctx context.Context, in mbapi.ContributionGetDataTypeInput) (mbapi.ContributionGetDataTypeOutput, error) {
	if in.Name == "" {
		return mbapi.ContributionGetDataTypeOutput{}, nil
	}
	m, err := s.manifest(ctx)
	if err != nil {
		return mbapi.ContributionGetDataTypeOutput{}, err
	}
	for _, c := range m.Contributions {
		// The meta-model's own kind (contribapi.IsMetaKind): this verb IS the
		// data-type lookup, the base case, not a per-kind branch.
		if c.Type != contribapi.ResourceTypeDataType {
			continue
		}
		if c.ID == in.Name {
			// Source(), not Data(): since c24 a data-type carries its DSL in
			// the SOURCE layer so the index does not hold it twice. Answering
			// from the data layer returned nothing to every remote consumer
			// that resolves a schema by name.
			return mbapi.ContributionGetDataTypeOutput{Body: c.Source()}, nil
		}
	}
	// Not owned by this sidecar — empty + nil per contract.
	return mbapi.ContributionGetDataTypeOutput{}, nil
}

// contributionMatchesFilter applies the server-side filter to one
// Contribution. Empty filter matches everything.
func contributionMatchesFilter(c contribapi.Contribution, f mbapi.ContributionListFilter) bool {
	// URI allowlist short-circuits other criteria.
	if len(f.URIs) > 0 {
		uri := c.ResolvedURI()
		for _, u := range f.URIs {
			if u == uri {
				return true
			}
		}
		return false
	}
	if len(f.Types) > 0 {
		hit := false
		for _, t := range f.Types {
			if c.Type == t {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if f.IDGlob != "" {
		if !globMatch(f.IDGlob, c.ID) {
			return false
		}
	}
	return true
}

// globMatch is a thin wrapper over path.Match with predictable
// semantics for IDs containing '/'.
func globMatch(pattern, id string) bool {
	ok, err := path.Match(pattern, id)
	if err == nil && ok {
		return true
	}
	// Fallback: simple suffix match for "*-something" / "something-*"
	// style globs against IDs with slashes (path.Match doesn't cross /).
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(id, pattern[1:]) {
		return true
	}
	if strings.HasSuffix(pattern, "*") && strings.HasPrefix(id, pattern[:len(pattern)-1]) {
		return true
	}
	return false
}

// Compile-time check.
var _ mbapi.ResourceService = (*ResourceServiceImpl)(nil)
