// SPDX-License-Identifier: Apache-2.0

package builtin

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// ContributeLeafKinds publishes the leaf kinds this binary carries: for each,
// `<provider>:/contribution-type/<kind>` (its schema named under provider) and
// `<provider>:/data-type/<schema id>` (the Go type's shape). A binary that
// does not call it declares none of the leaf kinds.
//
// Once per binary, whatever the number of calls: a binary that wires a pack
// per root or layer calls it each time; the calls add their leaves to one
// publication, under one provider ("" = the host). A second provider is an
// error — one binary declares a kind once. A call with no leaf publishes
// nothing.
func ContributeLeafKinds(provider string, leaves ...leaf.Declaration) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("contribution", ".builtin-kinds", func(app *application.Application) error {
			if len(leaves) == 0 {
				return nil // nothing to publish
			}
			resolved := provider
			if resolved == "" {
				resolved = app.Provider()
			}
			p, first, err := publicationOf(app, resolved)
			if err != nil {
				return err
			}
			p.add(leaves)
			if !first {
				return nil
			}
			if err := application.Contribute(app, contribapi.ContributionProvider(p.kinds)); err != nil {
				return fmt.Errorf("contribute builtin kinds: %w", err)
			}
			if err := application.Contribute(app, contribapi.ContributionProvider(p.types)); err != nil {
				return fmt.Errorf("contribute builtin kind data-types: %w", err)
			}
			return nil
		})
	}
}

// publication is a binary's one publication of its leaf kinds, app-scoped.
// Every call adds to it during the contribute phase; its providers enumerate
// at bootstrap, when every call has run.
type publication struct {
	provider string
	kinds    *kindProvider
	types    *manifestcontrib.GoTypeDataTypeProvider
}

var publicationMu sync.Mutex

// publicationOf returns this application's publication, creating it on the
// first call (first = true).
func publicationOf(app *application.Application, provider string) (*publication, bool, error) {
	publicationMu.Lock()
	defer publicationMu.Unlock()
	if p, err := application.AppUseService[*publication](app); err == nil && p != nil {
		if p.provider != provider {
			return nil, false, fmt.Errorf("builtin kinds: published under %q, then asked under %q", p.provider, provider)
		}
		return p, false, nil
	}
	p := &publication{provider: provider, kinds: &kindProvider{provider: provider},
		types: &manifestcontrib.GoTypeDataTypeProvider{ProviderName: provider}}
	application.PutService[*publication](app, "default", p)
	return p, true, nil
}

// add records leaves not yet published.
func (p *publication) add(ls []leaf.Declaration) {
	publicationMu.Lock()
	defer publicationMu.Unlock()
	for _, l := range ls {
		if p.kinds.has(l.Kind) {
			continue
		}
		p.kinds.leaves = append(p.kinds.leaves, l)
		p.types.Types = append(p.types.Types, l.Schema)
	}
}

// kindProvider publishes the contribution-type of each leaf kind.
type kindProvider struct {
	provider string
	leaves   []leaf.Declaration
	epoch    contribapi.ImmutableEpoch
}

func (k *kindProvider) has(kind string) bool {
	for _, l := range k.leaves {
		if l.Kind == kind {
			return true
		}
	}
	return false
}

func (k *kindProvider) Type() string     { return contribapi.ResourceTypeContributionType }
func (k *kindProvider) Provider() string { return k.provider }

// Enumerate builds one declaration per leaf, sorted by kind.
func (k *kindProvider) Enumerate(_ context.Context) ([]contribapi.Contribution, error) {
	out := make([]contribapi.Contribution, 0, len(k.leaves))
	for _, l := range k.leaves {
		e, err := entryOf(k.provider, l)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return k.epoch.Record(out), nil
}

// Open serves a layer from the provider's own enumeration.
func (k *kindProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, k, id, layer)
}

// Identity: the leaves are compiled into the binary — immutable.
func (k *kindProvider) Identity() contribapi.Identity { return k.epoch.Identity(k) }

// Providers are the providers ContributeLeafKinds publishes for leaves under
// provider — the contribution-types and their data-types — outside an
// application: what a census judging another binary's entries reads as that
// binary's leaf declarations.
func Providers(provider string, leaves ...leaf.Declaration) []contribapi.ContributionProvider {
	p := &publication{provider: provider, kinds: &kindProvider{provider: provider},
		types: &manifestcontrib.GoTypeDataTypeProvider{ProviderName: provider}}
	p.add(leaves)
	return []contribapi.ContributionProvider{p.kinds, p.types}
}
