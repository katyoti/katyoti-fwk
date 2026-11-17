// SPDX-License-Identifier: Apache-2.0

package provider

// ConfigProvider is a domain-agnostic source of configuration items.
// Different implementations can load from YAML files, embedded defaults,
// remote services, etc.
type ConfigProvider interface {
	// ProvideConfigFor returns configuration items for the given domain.
	// Returns nil, nil if the provider has nothing for this domain.
	ProvideConfigFor(domain string) ([]ConfigItem, error)
}

// ConfigItem describes a single factory to be created from a strategy + config.
type ConfigItem struct {
	// Data holds the parsed configuration as a generic map.
	// Consumers extract "name", "type", and other fields via FindValue.
	// The map is decoded into a typed config struct via marshal.DecodeMap.
	Data map[string]any

	// Source identifies where this config came from (e.g., "plugins.yaml", "embedded").
	Source string
}

// FindValue returns the string value of a key in this item's Data map.
// Returns "" if the key is absent or not a string.
func (item *ConfigItem) FindValue(key string) string {
	v, ok := item.Data[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// FuncConfigProvider wraps a function as a ConfigProvider.
type FuncConfigProvider struct {
	Fn func(domain string) ([]ConfigItem, error)
}

func (f *FuncConfigProvider) ProvideConfigFor(domain string) ([]ConfigItem, error) {
	return f.Fn(domain)
}
