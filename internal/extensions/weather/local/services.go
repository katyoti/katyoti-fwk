// SPDX-License-Identifier: Apache-2.0

// Package local registers WeatherService as a PublishedService. One
// import in `app/ktw-weather/cmd/bootstrap.go` is enough to gain the
// full KTSP / HTTP / Cobra CLI surface (per the standard sidecar
// pattern documented in CLAUDE.md "Service Publishing").
package local

import (
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	weatherapi "github.com/katyoti/katyoti-fwk/internal/extensions/weather/api"
	weathercore "github.com/katyoti/katyoti-fwk/internal/extensions/weather/internal/core"
)

// Extension publishes WeatherService over KTSP + HTTP + Cobra CLI.
// `WithFlatCLI`-equivalent behavior is enabled at the AutoKTSPContainer
// level in the binary's bootstrap (single-service sidecars opt into
// flat CLI so users type `ktw-weather get-forecast` rather than
// `ktw-weather weather-service get-forecast`).
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishService[weatherapi.WeatherService, weathercore.WeatherServiceConfig](
			weathercore.New,
			appcore.WithServiceDef(weatherapi.WeatherServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithHTTPServer(8892),
		))
	}
}
