// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	skillslocal "github.com/katyoti/katyoti-fwk/internal/extensions/skills/local"
	weatherlocal "github.com/katyoti/katyoti-fwk/internal/extensions/weather/local"
)

// buildExtensions declares all ApplicationExtensions for the ktw-weather
// sidecar. SN-11 migrated this from the ~14-line hand-written list to
// `sidecarkit.NewSidecar(...)` — same extensions, less boilerplate.
//
// The hand-written form is still valid (and required for multi-domain
// sidecars like ktw-agents / ktw-docs / ktw-hub). The builder is the
// canonical form for single-service sidecars per the ms-sidecar-norm
// milestone.
func buildExtensions(efs embed.FS) []application.Extension {
	return sidecarkit.NewSidecar(appcore.AppConfig{
		AppName: "ktw-weather",
		Version: version,
		Short:   "ktw-weather — Weather forecast sidecar (Open-Meteo backed)",
		Long: `ktw-weather is the canonical minimum-viable sidecar for the ktw-agents
ecosystem. It demonstrates the full unified-contribution-system pattern:
one published service (WeatherService) with two methods, one skill, one
config file, ~80 LOC of domain logic.

Backend: https://open-meteo.com (free, no API key required).

LLM: run 'ktw-weather skills get-instructions ktw-weather:/skill/weather-usage' for guidance.`,
		EmbeddedFS: efs,
	},
		sidecarkit.WithDomain(weatherlocal.Extension()),
		sidecarkit.WithKTSPDescription("Weather forecast service"),
		sidecarkit.WithFlatCLI(),
		// Its embed/ pack is the kit's: the forecast-day data-type (renders
		// as a schema-bound table/view, ktw-weather:/data-type/forecast-day;
		// pairs with the render-data-as-ui chart page of ktw-methods) and
		// the weather-forecast-page task (ask a city, render the forecast as a
		// page with charts), run by whatever host runs the agent loop
		// (ktw-agents) when ktw-weather is a universe member.
		// The skills runtime (SkillManager, `sc skills`): this binary's own wiring, never the kit's.
		sidecarkit.WithDomain(skillslocal.Extension()),
	)
}
