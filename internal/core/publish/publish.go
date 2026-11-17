// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/core/services"
)

// PublishService declares a published service with config.
// Registers the service factory and contributes a PublishedService to the EM point.
func PublishService[TService any, TConfig any](
	factory func(ctx context.Context, cfg *TConfig) (TService, error),
	opts ...PublishOption,
) application.Extension {
	reg := services.ConfiguredService[TService, TConfig](factory)
	svcType := reflect.TypeOf((*TService)(nil)).Elem()
	// Derive the config file AND its shape HERE, the only place that knows
	// both the file's name and the Go type it decodes into.
	//
	// The file name is not a choice: the service manager already resolves
	// config from the SERVICE type name, so declaring it separately could only
	// ever agree or be wrong. Advertising it here means `sc config check`
	// lists it and `sc config` materialises its template without the author
	// restating what the declaration above already said.
	//
	// Prepended so an explicit WithConfigShape still overrides, and so an
	// explicit WithConfigFiles adds FURTHER files rather than replacing this
	// one (a service reading more than one config still says so).
	opts = append([]PublishOption{
		configShapeOf[TConfig](),
		configFileOf[TService, TConfig](),
	}, opts...)
	return publishExtension(reg, svcType, opts)
}

// PublishSimpleService declares a published service with no config.
func PublishSimpleService[TService any](
	factory func(ctx context.Context) (TService, error),
	opts ...PublishOption,
) application.Extension {
	reg := services.SimpleService[TService](factory)
	svcType := reflect.TypeOf((*TService)(nil)).Elem()
	return publishExtension(reg, svcType, opts)
}

// DeclareInfra contributes infrastructure needs as a PublishedService with no service factory.
func DeclareInfra(name string, opts ...PublishOption) application.Extension {
	ps := &PublishedService{Name: name}
	for _, opt := range opts {
		opt(ps)
	}
	return func(init *application.ApplicationInit) {
		ensurePublishedServicesPoint(init)
		init.AddContribute("infra", "."+name, func(app *application.Application) error {
			return application.Contribute(app, *ps)
		})
	}
}

// publishExtension is the shared implementation for all Publish* variants.
func publishExtension(reg services.ServiceRegistration, svcType reflect.Type, opts []PublishOption) application.Extension {
	ps := &PublishedService{
		Name:        svcType.Name(),
		ServiceType: svcType,
	}
	for _, opt := range opts {
		opt(ps)
	}
	// The derived file joins the advertised list only after every option has
	// run, so WithoutConfigFile works wherever it appears and a caller's own
	// WithConfigFiles cannot be shadowed by ordering.
	if ps.ownConfigFile != "" && !ps.suppressOwnConfigFile {
		WithConfigFiles(ps.ownConfigFile)(ps)
	}

	className := config.TypeNameOf(svcType)

	return func(init *application.ApplicationInit) {
		ensurePublishedServicesPoint(init)

		init.AddContribute("services", "."+className, func(app *application.Application) error {
			application.RegisterService(app, reg)
			return nil
		})

		init.AddContribute("published", "."+ps.Name, func(app *application.Application) error {
			return application.Contribute(app, *ps)
		})

		// Phase D pending: ServiceDef auto-contribution as a
		// contribapi.Contribution{Type:"service"} via a Producer in
		// core/contribution. The Phase B `contribution.InterfacePoint`
		// auto-wire here is removed pending that rewire.
	}
}

// ensurePublishedServicesPoint registers the PublishedServices EM point if not already present.
func ensurePublishedServicesPoint(init *application.ApplicationInit) {
	init.AddRegisterPoints("publish", ".points", func(app *application.Application) error {
		if app.PointInfo(em.PointID[PublishedService]()) == nil {
			return application.RegisterPoint(app, NewPublishedServicesPoint())
		}
		return nil
	})
}

// configFileOf advertises the config file this service's declaration already
// loads from.
//
// Nothing for a config type with no fields: a service whose config struct is
// empty has no settings, and advertising a file an operator cannot usefully
// edit is worse than advertising none.
func configFileOf[TService any, TConfig any]() PublishOption {
	if !hasFields[TConfig]() {
		return func(*PublishedService) {}
	}
	name := config.TypeName[TService]() + ".yaml"
	return func(ps *PublishedService) { ps.ownConfigFile = name }
}

// configShapeOf derives the schema of a config type and records it under the
// file that type decodes from.
//
// Automatic for every typed config, by construction: it runs inside
// PublishService rather than being something each sidecar remembers to add. An
// opt-in list would leave the sidecars nobody added unconfigurable — the same
// gap, moved somewhere easier to miss.
//
// The schema is the **DSL**, not JSON Schema. A config shape is published as a
// `data-type`, and a data-type is parsed by the DSL parser — so a JSON Schema
// there is a document in the wrong language: it registers nowhere, and the URI
// it was published under answers nothing.
//
// The conversion is free. `reflect.TypeAST` walks the Go type into the same AST
// the parser builds, and `schemaapi.Schema`'s JSON field names ARE the DSL's,
// so marshalling it produces a document the parser accepts — descriptions from
// `jsonschema:"description=…"` and optionality from `omitempty` intact.
//
// A type whose schema cannot be derived records nothing. That is a normal
// outcome, not an error: an absent shape means a consumer falls back to the
// health report, and failing publication over it would take down a service that
// otherwise works.
func configShapeOf[TConfig any]() PublishOption {
	// An empty config struct has no shape worth publishing: a schema with no
	// fields tells a form renderer to draw nothing, which is indistinguishable
	// from a schema that failed to load.
	if !hasFields[TConfig]() {
		return func(*PublishedService) {}
	}
	ast, err := ktwreflect.TypeAST[TConfig]()
	if err != nil || ast == nil {
		return func(*PublishedService) {}
	}
	doc := schemaapi.NewSchema(config.ConfigName[TConfig]())
	doc.Root = configRootType
	doc.Types = map[string]*schemaapi.Type{configRootType: ast}
	body, err := json.Marshal(doc)
	if err != nil {
		return func(*PublishedService) {}
	}
	return WithConfigShape(body)
}

// hasFields reports whether a config type has any settings at all.
//
// The one question both the file and the shape turn on: a service whose config
// struct is empty has nothing an operator can set, so advertising a file and
// publishing a schema for it would describe a surface that does not exist.
func hasFields[TConfig any]() bool {
	t := reflect.TypeFor[TConfig]()
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t != nil && t.Kind() == reflect.Struct && t.NumField() > 0
}

// configRootType is the name of the single type a config document declares.
// Fixed rather than derived from the Go type: the document has exactly one
// root, and naming it after the Go struct would put an implementation detail
// into a published schema.
const configRootType = "Config"
