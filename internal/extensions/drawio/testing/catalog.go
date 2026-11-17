// SPDX-License-Identifier: Apache-2.0

package testing

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Catalog is an in-memory mbapi.Catalog holding the contributions a test
// puts in it, by provider.
type Catalog struct {
	manifests map[string]*mbapi.Manifest
}

// NewCatalog builds a Catalog from contributions grouped by provider.
func NewCatalog(contribs map[string][]contribapi.Contribution) *Catalog {
	c := &Catalog{manifests: make(map[string]*mbapi.Manifest)}
	for provider, list := range contribs {
		c.manifests[provider] = &mbapi.Manifest{Provider: provider, Contributions: list}
	}
	return c
}

// Add puts one contribution under its provider.
func (f *Catalog) Add(c contribapi.Contribution) {
	m, ok := f.manifests[c.Provider]
	if !ok {
		m = &mbapi.Manifest{Provider: c.Provider}
		f.manifests[c.Provider] = m
	}
	m.Contributions = append(m.Contributions, c)
}

func (f *Catalog) Get(_ context.Context, name string) (*mbapi.Manifest, error) {
	if m, ok := f.manifests[name]; ok {
		return m, nil
	}
	return nil, mbapi.ErrSidecarNotInCatalog
}

func (f *Catalog) List(_ context.Context) ([]*mbapi.Manifest, error) {
	out := make([]*mbapi.Manifest, 0, len(f.manifests))
	for _, m := range f.manifests {
		out = append(out, m)
	}
	return out, nil
}

func (f *Catalog) Names(_ context.Context) ([]string, error) {
	out := make([]string, 0, len(f.manifests))
	for name := range f.manifests {
		out = append(out, name)
	}
	return out, nil
}

func (f *Catalog) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, m := range f.manifests {
		for _, c := range m.Contributions {
			if c.Type == t {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

func (f *Catalog) GetBody(_ context.Context, uri string) ([]byte, error) {
	pu, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return nil, err
	}
	for _, m := range f.manifests {
		for _, c := range m.Contributions {
			prov := c.Provider
			if prov == "" {
				prov = m.Provider
			}
			if c.Type == pu.Type && c.ID == pu.ID && prov == pu.Provider {
				return c.Data(), nil
			}
		}
	}
	return nil, mbapi.ErrSidecarNotInCatalog
}

func (f *Catalog) SetSchema(_ mbapi.CatalogSchemaService) error { return nil }
func (f *Catalog) Reindex(_ context.Context, _ []string) error  { return nil }
func (f *Catalog) MarkReady(_ context.Context) error            { return nil }
func (f *Catalog) Ready() bool                                  { return true }

// Contribution wraps a body as an inline contribution of kind typ.
func Contribution(t testing.TB, provider, typ, id string, body any) contribapi.Contribution {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal %s %s: %v", typ, id, err)
	}
	return contribapi.Contribution{
		Provider: provider,
		Type:     typ,
		ID:       id,
		URI:      provider + ":/" + typ + "/" + id,
		Layers:   contribapi.InlineData(raw),
	}
}

// WithCatalog returns a context whose application serves cat as the
// universe catalog.
func WithCatalog(t testing.TB, cat *Catalog) context.Context {
	t.Helper()
	app := application.New(application.Config())
	application.PutService[mbapi.Catalog](app, "default", cat)
	return application.WithApplication(context.Background(), app)
}
