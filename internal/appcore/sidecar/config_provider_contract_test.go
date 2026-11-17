// SPDX-License-Identifier: Apache-2.0

package sidecar

import (
	"context"
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

type contractSvc interface {
	Ping(ctx context.Context) error
}
type contractImpl struct{}

func (contractImpl) Ping(context.Context) error { return nil }

// The four config providers (config-file, config-program, env, and the
// config shapes as data-types) hold the provider contract over a real
// publication set: every carried layer opens through Open with bytes that
// hash to its address, and the error contract holds.
func TestConfigProviders_HoldTheProviderContract(t *testing.T) {
	app := application.New(
		config.NewConfigService(loader.NewMapLoader(nil)),
		publish.PublishSimpleService[contractSvc](
			func(context.Context) (contractSvc, error) { return contractImpl{}, nil },
			publish.WithConfigFiles("WeatherService.yaml"),
			publish.WithConfigShape(json.RawMessage(`{"type":"object","properties":{"units":{"type":"string"}}}`)),
			publish.WithTools(publish.ToolDef{Name: "pandoc", Commands: []string{"pandoc"}}),
			publish.WithAPIKeys(publish.APIKeyDef{EnvName: "WEATHER_API_KEY", Label: "Weather key", Required: true}),
		),
	)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	efs := fstest.MapFS{"embed/config/WeatherService.yaml": {Data: []byte("units: metric\n")}}
	ctx := app.Context()
	for _, p := range ConfigResourceProviders("ktw-test", efs) {
		entries, err := p.Enumerate(ctx)
		if err != nil || len(entries) == 0 {
			t.Fatalf("%s: Enumerate = %d entries, err %v — the contract would be checked over nothing", p.Type(), len(entries), err)
		}
		providertest.RunContext(t, ctx, p)
	}
}
