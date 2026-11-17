// SPDX-License-Identifier: Apache-2.0

package provider

// MultiProvider aggregates multiple [ConfigProvider] implementations into one.
// All providers are called in order; results are concatenated.
//
//	provider := provider.NewMultiProvider(embeddedProvider, userProvider)
//	items, err := provider.ProvideConfigFor("agent.plugin")
type MultiProvider struct {
	providers []ConfigProvider
}

// NewMultiProvider creates a MultiProvider from the given providers.
func NewMultiProvider(providers ...ConfigProvider) *MultiProvider {
	return &MultiProvider{providers: providers}
}

func (m *MultiProvider) ProvideConfigFor(domain string) ([]ConfigItem, error) {
	var all []ConfigItem
	for _, p := range m.providers {
		items, err := p.ProvideConfigFor(domain)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
	}
	return all, nil
}
