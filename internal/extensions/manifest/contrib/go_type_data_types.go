// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// GoType is one data-type whose schema is the shape of a Go type: the pack
// that decodes its entries into T publishes T's shape as the kind's schema,
// so the two cannot drift. Built with GoTypeOf.
type GoType struct {
	// ID is the data-type id: published at `<provider>:/data-type/<ID>`.
	ID string
	// Version is the schema's own version, bumped when the type changes shape.
	Version string
	// Root is the DSL root type name.
	Root string
	// Description is the document-level description.
	Description string
	// AST renders the root type (reflect.TypeAST of the Go type).
	AST func() (*schemaapi.Type, error)
}

// SchemaOverride is implemented by a Go type whose published shape differs
// from what its fields alone say: GoTypeOf hands it the rendered root to
// adjust (a field an embedded struct requires that this type leaves
// optional). It changes the schema only, never the JSON form.
type SchemaOverride interface {
	OverrideSchema(root *schemaapi.Type)
}

// GoTypeOf names the data-type rendered from T, adjusted by T's
// SchemaOverride when it has one.
func GoTypeOf[T any](id, version, root, description string) GoType {
	return GoType{ID: id, Version: version, Root: root, Description: description,
		AST: func() (*schemaapi.Type, error) {
			ast, err := ktwreflect.TypeAST[T]()
			if err != nil {
				return nil, err
			}
			if o, ok := any(new(T)).(SchemaOverride); ok {
				o.OverrideSchema(ast)
			}
			return ast, nil
		}}
}

// ContributeGoTypeDataTypes registers a provider publishing each type as a
// data-type of <provider> ("" = the host). The contribute key names the
// provider: a binary registers it once per provider.
func ContributeGoTypeDataTypes(provider string, types ...GoType) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("manifest", ".go-type-data-types."+provider, func(app *application.Application) error {
			resolved := provider
			if resolved == "" {
				resolved = app.Provider()
			}
			return application.Contribute(app, contribapi.ContributionProvider(&GoTypeDataTypeProvider{ProviderName: resolved, Types: types}))
		})
	}
}

// GoTypeDataTypeProvider publishes data-types rendered from Go types at run
// time: the DSL document (in its JSON form) is the entry's source layer. A
// generic producer, beside the file-backed data-type provider, for a pack
// whose kind's schema IS a Go type.
type GoTypeDataTypeProvider struct {
	ProviderName string
	Types        []GoType

	epoch contribapi.ImmutableEpoch // stamped by Enumerate
}

func (p *GoTypeDataTypeProvider) Type() string     { return contribapi.ResourceTypeDataType }
func (p *GoTypeDataTypeProvider) Provider() string { return p.ProviderName }

// Enumerate renders one data-type per Go type, sorted by id. A type that does
// not render is an error: the binary would publish a kind with no schema.
func (p *GoTypeDataTypeProvider) Enumerate(_ context.Context) ([]contribapi.Contribution, error) {
	out := make([]contribapi.Contribution, 0, len(p.Types))
	for _, t := range p.Types {
		raw, err := t.Document()
		if err != nil {
			return nil, err
		}
		c := contribapi.Contribution{
			Provider: p.ProviderName,
			Type:     contribapi.ResourceTypeDataType,
			ID:       t.ID,
			Metadata: &contribapi.ContributionMeta{Name: t.ID, MediaType: "application/json"}, // description: derived from the source (the data-type kind's meta.from)
		}
		c.SetSource(raw, "application/json")
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return p.epoch.Record(out), nil
}

// Document is the type's DSL document, in its JSON form.
func (t GoType) Document() ([]byte, error) {
	root, err := t.AST()
	if err != nil {
		return nil, fmt.Errorf("data-type %s: %w", t.ID, err)
	}
	doc := schemaapi.NewSchema(t.ID)
	doc.Version, doc.Root, doc.Description = t.Version, t.Root, t.Description
	doc.Types[t.Root] = root
	raw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("data-type %s: marshal: %w", t.ID, err)
	}
	return raw, nil
}

// Open serves a layer from the provider's own enumeration.
func (p *GoTypeDataTypeProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

// Identity: the types are compiled into the binary — immutable, named by the
// digest of the last enumeration.
func (p *GoTypeDataTypeProvider) Identity() contribapi.Identity { return p.epoch.Identity(p) }
