// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
)

// TypeFactory produces typed instances from config using the tuple/closure pattern.
//
//	cfg, create := factory.NewTypeConfig()
//	config.DecodeMap(data, cfg)         // fills config from YAML
//	instance, err := create(ctx)        // deps resolved via UseService[T](ctx)
//
// The closure captures the config pointer internally, so the caller
// does not need to know the concrete config type.
//
// T is the produced type (e.g., PluginFactory, MachineFactory).
type TypeFactory[T any] interface {
	NewTypeConfig() (configPtr any, createFn func(ctx context.Context) (T, error))
}

// TypeFactoryLookup finds a TypeFactory by type name.
// Returns nil if the type name is not found.
type TypeFactoryLookup[T any] interface {
	Get(typeName string) TypeFactory[T]
}

// KeyedSource is any keyed collection with Get and Has (e.g., em.IndexedPoint).
type KeyedSource[T any] interface {
	Get(key string) T
	Has(key string) bool
}

// AsTypeFactoryLookup wraps a KeyedSource into a TypeFactoryLookup using an
// extract function that pulls a TypeFactory from each element.
// Returns nil from Get when the key is not found or the extracted factory is nil.
func AsTypeFactoryLookup[TElem any, T any](
	source KeyedSource[TElem],
	extract func(TElem) TypeFactory[T],
) TypeFactoryLookup[T] {
	return &keyedLookup[TElem, T]{source: source, extract: extract}
}

type keyedLookup[TElem any, T any] struct {
	source  KeyedSource[TElem]
	extract func(TElem) TypeFactory[T]
}

func (l *keyedLookup[TElem, T]) Get(name string) TypeFactory[T] {
	if !l.source.Has(name) {
		return nil
	}
	return l.extract(l.source.Get(name))
}

// CreateFromMapResult holds the output of CreateFromMap.
type CreateFromMapResult[T any] struct {
	// Value is the created instance.
	Value T
	// TypeName is the resolved type discriminator from the config map.
	TypeName string
	// Factory is the TypeFactory that produced the value.
	Factory TypeFactory[T]
}

// CreateFromMap extracts a type discriminator from a config map, looks up the
// matching TypeFactory, decodes the map into its config via config.DecodeMap
// (env var expansion + ProcessConfig), and calls the create closure with ctx.
//
// Dependencies are resolved by the strategy inside createFn via UseService[T](ctx).
//
// typeFieldName is the map key holding the discriminator (default: "type" when empty).
func CreateFromMap[T any](ctx context.Context, lookup TypeFactoryLookup[T], data map[string]any, typeFieldName string) (CreateFromMapResult[T], error) {
	var zero CreateFromMapResult[T]

	if typeFieldName == "" {
		typeFieldName = "type"
	}

	typeVal, ok := data[typeFieldName]
	if !ok {
		return zero, fmt.Errorf("type field %q not found in config map", typeFieldName)
	}
	typeName, ok := typeVal.(string)
	if !ok || typeName == "" {
		return zero, fmt.Errorf("type field %q is not a non-empty string", typeFieldName)
	}

	factory := lookup.Get(typeName)
	if factory == nil {
		return zero, fmt.Errorf("unknown type %q", typeName)
	}

	cfgPtr, createFn := factory.NewTypeConfig()

	// Fill config from the data map.
	if cfgPtr != nil {
		if err := config.DecodeMap(data, cfgPtr); err != nil {
			return zero, fmt.Errorf("unmarshal type %q: %w", typeName, err)
		}
	}

	val, err := createFn(ctx)
	if err != nil {
		return zero, err
	}

	return CreateFromMapResult[T]{
		Value:    val,
		TypeName: typeName,
		Factory:  factory,
	}, nil
}
