// SPDX-License-Identifier: Apache-2.0

package sidecar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// ConfigResourceProvider emits descriptor entries for one of the three
// config-related resource types (config-file / config-program / env). The
// data source is the union of all PublishedService contributions read from
// the EM at Enumerate-time.
//
// Three providers — one per type — sharing the same logic. Use
// ConfigResourceProviders(name, efs) to build the standard group.
//
// Replaces the typed-report path of ConfigService.Describe (per todo
// bus-split-config-as-resources): the descriptor is the static declaration
// of needs; ConfigService.Check remains the runtime probe combining static
// needs with host state.
type ConfigResourceProvider struct {
	ProviderName string // binary name (typically Cobra root.Use)
	ResourceType string // mbapi.ResourceTypeConfigFile / ConfigProgram / Env
	EmbeddedFS   fs.FS  // optional — used to set ConfigFileBody.HasDefault

	// epoch is stamped by every Enumerate: the enumeration reads the
	// application's extension points, which Identity has no context to reach.
	epoch contribapi.ImmutableEpoch
}

// ConfigResourceProviders returns the providers that, together, cover every
// sidecar's configuration surface: what it needs, and — for a typed config —
// the SHAPE of each file it needs. Wire each via
// `manifestlocal.ContributeResources(p)` in your bootstrap.
func ConfigResourceProviders(providerName string, efs fs.FS) []contribapi.ContributionProvider {
	return []contribapi.ContributionProvider{
		&ConfigResourceProvider{ProviderName: providerName, ResourceType: mbapi.ResourceTypeConfigFile, EmbeddedFS: efs},
		&ConfigResourceProvider{ProviderName: providerName, ResourceType: mbapi.ResourceTypeConfigProgram},
		&ConfigResourceProvider{ProviderName: providerName, ResourceType: mbapi.ResourceTypeEnv},
		// Shapes ride the normal contribution path as ordinary data-types, so
		// they are discoverable, content-addressed, cached and refreshed like
		// anything else — no bespoke schema verb to keep in step.
		&ConfigShapeProvider{ProviderName: providerName},
	}
}

func (p *ConfigResourceProvider) Type() string     { return p.ResourceType }
func (p *ConfigResourceProvider) Provider() string { return p.ProviderName }

// Enumerate reads PublishedServicesPoint from the EM, aggregates the
// relevant slice for this provider's type, and emits one Contribution per
// item. Items are de-duplicated by ID — if two PublishedServices declare
// the same config file or program, only the first wins.
func (p *ConfigResourceProvider) Enumerate(ctx context.Context) ([]contribapi.Contribution, error) {
	pubs := application.UseExtensions[publish.PublishedService](ctx)
	var out []contribapi.Contribution
	var err error
	switch p.ResourceType {
	case mbapi.ResourceTypeConfigFile:
		out, err = p.enumerateConfigFiles(pubs)
	case mbapi.ResourceTypeConfigProgram:
		out, err = p.enumerateConfigPrograms(pubs)
	case mbapi.ResourceTypeEnv:
		out, err = p.enumerateEnv(pubs)
	default:
		return nil, fmt.Errorf("ConfigResourceProvider: unknown ResourceType %q", p.ResourceType)
	}
	if err != nil {
		return nil, err
	}
	return p.epoch.Record(out), nil
}

// Open implements ContributionProvider.Open from this provider's own
// enumeration — one answer, the entry Enumerate already carries.
func (p *ConfigResourceProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

func (p *ConfigResourceProvider) enumerateConfigFiles(pubs []publish.PublishedService) ([]contribapi.Contribution, error) {
	seen := map[string]struct{}{}
	out := make([]contribapi.Contribution, 0)
	for _, ps := range pubs {
		for _, name := range ps.ConfigFiles {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}

			body := mbapi.ConfigFileBody{}
			if p.EmbeddedFS != nil {
				if _, err := fs.Stat(p.EmbeddedFS, "embed/config/"+name); err == nil {
					body.HasDefault = true
				}
			}
			// Point at this file's own shape, where one was derived. The
			// data-type it names is published by configShapeProvider below —
			// an ordinary contribution, discoverable, content-addressed and
			// refreshed like any other, rather than a bespoke schema verb.
			if _, ok := shapeFor(pubs, name); ok {
				body.Schema = contribapi.Contribution{
					Provider: p.ProviderName,
					Type:     contribapi.ResourceTypeDataType,
					ID:       configShapeID(name),
				}.ResolvedURI()
			}
			raw, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshal config-file body for %s: %w", name, err)
			}
			c := contribapi.Contribution{
				Provider: p.ProviderName,
				Type:     p.ResourceType,
				ID:       name,
			}
			c.SetData(raw)
			out = append(out, c)
		}
	}
	return out, nil
}

func (p *ConfigResourceProvider) enumerateConfigPrograms(pubs []publish.PublishedService) ([]contribapi.Contribution, error) {
	seen := map[string]struct{}{}
	out := make([]contribapi.Contribution, 0)
	for _, ps := range pubs {
		for _, t := range ps.Tools {
			if _, ok := seen[t.Name]; ok {
				continue
			}
			seen[t.Name] = struct{}{}

			body := mbapi.ConfigProgramBody{
				Commands:     t.Commands,
				ConfigKey:    t.ConfigKey,
				InstallSkill: t.InstallSkill,
				VersionFlag:  t.VersionFlag,
			}
			raw, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshal config-program body for %s: %w", t.Name, err)
			}
			c := contribapi.Contribution{
				Provider: p.ProviderName,
				Type:     p.ResourceType,
				ID:       t.Name,
			}
			c.SetData(raw)
			out = append(out, c)
		}
	}
	return out, nil
}

func (p *ConfigResourceProvider) enumerateEnv(pubs []publish.PublishedService) ([]contribapi.Contribution, error) {
	seen := map[string]struct{}{}
	out := make([]contribapi.Contribution, 0)
	for _, ps := range pubs {
		for _, k := range ps.APIKeys {
			if _, ok := seen[k.EnvName]; ok {
				continue
			}
			seen[k.EnvName] = struct{}{}

			body := mbapi.EnvBody{
				Label:       k.Label,
				Required:    k.Required,
				Description: k.Description,
			}
			raw, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshal env body for %s: %w", k.EnvName, err)
			}
			c := contribapi.Contribution{
				Provider: p.ProviderName,
				Type:     p.ResourceType,
				ID:       k.EnvName,
			}
			c.SetData(raw)
			out = append(out, c)
		}
	}
	return out, nil
}

// configShapeID is the data-type name carrying a config file's shape.
//
// Prefixed so it cannot collide with a domain data-type of the same name: a
// sidecar with a `Weather.yaml` config and a `weather` data-type describing its
// forecasts would otherwise publish two different shapes at one URI, and the
// later one would silently win.
func configShapeID(fileName string) string {
	return "config-" + strings.TrimSuffix(fileName, ".yaml")
}

// shapeFor finds the derived schema for a config file.
//
// A service's shape describes the file it declares — but only when it declares
// exactly ONE. A service naming several files gives no way to tell which its
// config type describes, so nothing is claimed rather than something guessed: a
// form built from the wrong file's shape is worse than no form, because it
// looks right.
//
// This is why the shape is not keyed by the config TYPE's name. A config file
// is named after its service (`GocodeService.yaml`) and the type it decodes
// into may be called anything (`core.Config`), so deriving the file name from
// the type linked nothing for exactly those services.
func shapeFor(pubs []publish.PublishedService, fileName string) (json.RawMessage, bool) {
	for _, ps := range pubs {
		if len(ps.ConfigShape) == 0 || len(ps.ConfigFiles) != 1 {
			continue
		}
		if ps.ConfigFiles[0] == fileName {
			return ps.ConfigShape, true
		}
	}
	return nil, false
}

// ConfigShapeProvider publishes each config file's shape as an ordinary
// data-type contribution.
//
// A separate provider rather than a branch inside ConfigResourceProvider,
// because these are contributions of a DIFFERENT kind: `data-type`, not
// `config-file`. A provider serves one kind — that is the contract the whole
// catalog is built on — and bending it here would make the exception the thing
// everyone copies.
type ConfigShapeProvider struct {
	ProviderName string

	epoch contribapi.ImmutableEpoch // stamped by Enumerate, as ConfigResourceProvider's
}

func (p *ConfigShapeProvider) Type() string     { return contribapi.ResourceTypeDataType }
func (p *ConfigShapeProvider) Provider() string { return p.ProviderName }

// Enumerate emits one data-type per config file whose shape could be derived.
//
// A config with no derivable shape emits nothing, which is what makes the
// degradation graceful: the config-file contribution simply carries no schema
// reference, and a consumer falls back to the health report.
func (p *ConfigShapeProvider) Enumerate(ctx context.Context) ([]contribapi.Contribution, error) {
	out, err := p.enumerateFrom(application.UseExtensions[publish.PublishedService](ctx))
	if err != nil {
		return nil, err
	}
	return p.epoch.Record(out), nil
}

// enumerateFrom is Enumerate over an explicit publication set — the same code
// the runtime takes, reachable without an application.
func (p *ConfigShapeProvider) enumerateFrom(pubs []publish.PublishedService) ([]contribapi.Contribution, error) {
	seen := map[string]struct{}{}
	out := make([]contribapi.Contribution, 0)
	for _, ps := range pubs {
		for _, name := range ps.ConfigFiles {
			schema, ok := shapeFor(pubs, name)
			if !ok {
				continue
			}
			id := configShapeID(name)
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			c := contribapi.Contribution{
				Provider: p.ProviderName,
				Type:     contribapi.ResourceTypeDataType,
				ID:       id,
				Metadata: &contribapi.ContributionMeta{
					Name:        name,
					Description: "Shape of " + name + ", derived from the Go type it decodes into",
					MediaType:   "application/json",
				},
			}
			// Derived from a Go type, so there is no file — the producer
			// states the schema directly as this contribution's content.
			c.SetSource(schema, "application/json")
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Open implements ContributionProvider.Open from this provider's own
// enumeration — one answer, the entry Enumerate already carries.
func (p *ConfigShapeProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

// Identity: config declarations come from the services the binary publishes
// (its extension points, fixed once bootstrap has run) and the templates
// compiled into it — immutable, named by the digest of the last enumeration
// (stamped by Enumerate, which the builder calls first).
func (p *ConfigResourceProvider) Identity() contribapi.Identity { return p.epoch.Identity(p) }

// Identity: same source as ConfigResourceProvider.
func (p *ConfigShapeProvider) Identity() contribapi.Identity { return p.epoch.Identity(p) }
