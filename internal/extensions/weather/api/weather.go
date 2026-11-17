// SPDX-License-Identifier: Apache-2.0

// Package api defines the WeatherService interface, its method I/O types,
// and the ServiceDef that AutoKTSP consumes to publish the service over
// KTSP / HTTP / Cobra CLI. Open-Meteo is the backend (free, no API
// key); see `internal/core/service.go` for the concrete implementation.
//
// ktw-weather is the canonical "minimum viable sidecar" template. The types here demonstrate the full RC1 contribution-system
// pattern in ~80 LOC: typed inputs/outputs with `desc:` for help
// generation, `ui-cli:` for terminal rendering, and one skill that
// teaches the LLM when to call us.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// WeatherService is the published interface. The two methods cover
// the canonical weather use case: a snapshot of right-now conditions
// and a multi-day forecast.
type WeatherService interface {
	GetCurrent(ctx context.Context, in GetCurrentInput) (CurrentWeather, error)
	GetForecast(ctx context.Context, in GetForecastInput) (Forecast, error)
}

// WeatherServiceDef drives AutoKTSP's slot publication + CLI generation.
// The kebab name `weather-service` becomes the slot token in URIs
// (e.g. `ktw-weather:weather-service/get-forecast`).
var WeatherServiceDef = meta.ServiceDef{
	Name:    "weather-service",
	Short:   "Weather forecast service (Open-Meteo backed, no API key required)",
	Long:    "Two methods: GetCurrent for a snapshot of current conditions and GetForecast for a multi-day forecast. Resolves city names via Open-Meteo's geocoding endpoint; returns Celsius + millimeters by default.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"GetCurrent":  {Short: "Current weather for a city", TimeoutS: 30},
		"GetForecast": {Short: "Multi-day weather forecast for a city", TimeoutS: 30},
	},
}

// ----------------------------------------------------------------------
// GetCurrent
// ----------------------------------------------------------------------

// GetCurrentInput is the single-field input — the city name. Resolved
// via Open-Meteo geocoding to lat/long internally.
type GetCurrentInput struct {
	City string `json:"city" cli:"arg" desc:"City name (e.g., \"Paris\", \"Tokyo\", \"São Paulo\")" required:"true"`
}

// CurrentWeather is the snapshot returned by GetCurrent. Renders as
// `key: value` text by default (object kind has no array sibling for
// the ui-cli renderer to descend into).
type CurrentWeather struct {
	City          string  `json:"city"          desc:"Resolved city name (after geocoding lookup)"`
	Country       string  `json:"country"       desc:"ISO country code of the resolved city"`
	Temperature   float64 `json:"temperature"   desc:"Current temperature in degrees Celsius"`
	WindKMH       float64 `json:"windKmh"       desc:"Wind speed in km/h"`
	WindDirection string  `json:"windDirection" desc:"Wind direction (compass abbreviation)"`
	Condition     string  `json:"condition"     desc:"Human-readable weather condition derived from WMO weather code"`
	Time          string  `json:"time"          desc:"Observation timestamp in ISO 8601 (location's local time)"`
}

// ----------------------------------------------------------------------
// GetForecast
// ----------------------------------------------------------------------

// GetForecastInput selects the city and a number of forecast days
// (1..16 per Open-Meteo's free tier; defaults to 5).
type GetForecastInput struct {
	City string `json:"city" cli:"arg" desc:"City name (e.g., \"Paris\", \"Tokyo\")"   required:"true"`
	Days int    `json:"days" cli:"days,d"   desc:"Forecast horizon (1..16, default 5)" default:"5"`
}

// Forecast is the multi-day forecast envelope. The single-array-field
// (Days) + scalar metadata (City) pattern matches the ui-cli
// renderer's pagination-envelope heuristic: it descends into Days
// and uses ForecastDay's columns for the rendered table.
type Forecast struct {
	City string        `json:"city"           desc:"Resolved city name (after geocoding lookup)"`
	Days []ForecastDay `json:"days" desc:"Forecast entries, one per day" ui-cli:"mode=table,columns=date;condition;tempMax;tempMin;precipitationMm;windKmh,primary=date,sort=date"`
}

// ForecastDay is one row in the Days table. Field order in the
// struct doubles as the default column order (overridable via the
// `ui-cli:"columns=..."` on the parent array field).
type ForecastDay struct {
	Date            string  `json:"date"            desc:"Forecast date (YYYY-MM-DD)"`
	Condition       string  `json:"condition"       desc:"Human-readable summary (sunny / cloudy / rainy / …)"`
	TempMax         float64 `json:"tempMax"         desc:"Max temperature in degrees Celsius"`
	TempMin         float64 `json:"tempMin"         desc:"Min temperature in degrees Celsius"`
	PrecipitationMm float64 `json:"precipitationMm" desc:"Total precipitation in millimeters"`
	WindKmh         float64 `json:"windKmh"         desc:"Max wind speed in km/h"`
}
