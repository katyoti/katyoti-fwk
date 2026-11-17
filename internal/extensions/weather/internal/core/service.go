// SPDX-License-Identifier: Apache-2.0

// Package core implements WeatherService backed by Open-Meteo
// (https://open-meteo.com — free, no API key, JSON over HTTPS).
//
// Two upstream calls per request:
//  1. Geocoding (https://geocoding-api.open-meteo.com/v1/search) to
//     resolve a city name to lat/lng + ISO country code.
//  2. Forecast (https://api.open-meteo.com/v1/forecast) for the
//     current or daily-aggregated data.
//
// Both are stateless HTTP GETs. No persistent state in the service
// struct beyond the http.Client.
package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	weatherapi "github.com/katyoti/katyoti-fwk/internal/extensions/weather/api"
)

// WeatherServiceConfig is decoded from `WeatherService.yaml`. Both
// fields are advisory — the upstream Open-Meteo API returns
// Celsius / km-h / millimeters regardless; this is a documentation
// hook for future unit-conversion support.
type WeatherServiceConfig struct {
	Unit     string `yaml:"unit,omitempty"     desc:"Temperature unit hint (metric|imperial). Currently advisory — Open-Meteo returns metric."`
	Language string `yaml:"language,omitempty" desc:"Language code for geocoding name resolution (default: en)."`
}

// service is the concrete implementation.
type service struct {
	cfg    *WeatherServiceConfig
	client *http.Client
}

// New constructs a WeatherService. The 5-second HTTP timeout is
// short enough to keep CLI commands responsive but long enough for
// Open-Meteo's typical sub-second latencies plus a slow cold start.
func New(_ context.Context, cfg *WeatherServiceConfig) (weatherapi.WeatherService, error) {
	return &service{
		cfg:    cfg,
		client: &http.Client{Timeout: 5 * time.Second},
	}, nil
}

// GetCurrent fetches snapshot conditions for the named city.
func (s *service) GetCurrent(ctx context.Context, in weatherapi.GetCurrentInput) (weatherapi.CurrentWeather, error) {
	if in.City == "" {
		return weatherapi.CurrentWeather{}, errors.New("weather: city required")
	}
	geo, err := s.geocode(ctx, in.City)
	if err != nil {
		return weatherapi.CurrentWeather{}, err
	}

	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(geo.Latitude, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(geo.Longitude, 'f', -1, 64))
	q.Set("current", "temperature_2m,weather_code,wind_speed_10m,wind_direction_10m")
	q.Set("timezone", "auto")
	q.Set("wind_speed_unit", "kmh")

	endpoint := "https://api.open-meteo.com/v1/forecast?" + q.Encode()
	var resp openMeteoCurrentResponse
	if err := s.getJSON(ctx, endpoint, &resp); err != nil {
		return weatherapi.CurrentWeather{}, fmt.Errorf("weather: open-meteo current: %w", err)
	}

	return weatherapi.CurrentWeather{
		City:          geo.Name,
		Country:       geo.CountryCode,
		Temperature:   resp.Current.Temperature2M,
		WindKMH:       resp.Current.WindSpeed10M,
		WindDirection: compassFromDegrees(resp.Current.WindDirection10M),
		Condition:     wmoCodeToCondition(resp.Current.WeatherCode),
		Time:          resp.Current.Time,
	}, nil
}

// GetForecast fetches a multi-day daily-aggregated forecast.
func (s *service) GetForecast(ctx context.Context, in weatherapi.GetForecastInput) (weatherapi.Forecast, error) {
	if in.City == "" {
		return weatherapi.Forecast{}, errors.New("weather: city required")
	}
	days := in.Days
	if days <= 0 {
		days = 5
	}
	if days > 16 {
		days = 16
	}

	geo, err := s.geocode(ctx, in.City)
	if err != nil {
		return weatherapi.Forecast{}, err
	}

	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(geo.Latitude, 'f', -1, 64))
	q.Set("longitude", strconv.FormatFloat(geo.Longitude, 'f', -1, 64))
	q.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,wind_speed_10m_max")
	q.Set("forecast_days", strconv.Itoa(days))
	q.Set("timezone", "auto")
	q.Set("wind_speed_unit", "kmh")

	endpoint := "https://api.open-meteo.com/v1/forecast?" + q.Encode()
	var resp openMeteoForecastResponse
	if err := s.getJSON(ctx, endpoint, &resp); err != nil {
		return weatherapi.Forecast{}, fmt.Errorf("weather: open-meteo forecast: %w", err)
	}

	out := weatherapi.Forecast{City: geo.Name}
	for i, date := range resp.Daily.Time {
		row := weatherapi.ForecastDay{Date: date}
		if i < len(resp.Daily.WeatherCode) {
			row.Condition = wmoCodeToCondition(resp.Daily.WeatherCode[i])
		}
		if i < len(resp.Daily.Temp2MMax) {
			row.TempMax = resp.Daily.Temp2MMax[i]
		}
		if i < len(resp.Daily.Temp2MMin) {
			row.TempMin = resp.Daily.Temp2MMin[i]
		}
		if i < len(resp.Daily.PrecipitationSum) {
			row.PrecipitationMm = resp.Daily.PrecipitationSum[i]
		}
		if i < len(resp.Daily.WindSpeed10MMax) {
			row.WindKmh = resp.Daily.WindSpeed10MMax[i]
		}
		out.Days = append(out.Days, row)
	}
	return out, nil
}

// ----------------------------------------------------------------------
// HTTP helpers
// ----------------------------------------------------------------------

type geocodeResult struct {
	Name        string
	Latitude    float64
	Longitude   float64
	CountryCode string
}

// geocode resolves a city name to lat/lng via Open-Meteo's free
// geocoding endpoint. Returns the first match (Open-Meteo orders by
// relevance + population).
func (s *service) geocode(ctx context.Context, city string) (geocodeResult, error) {
	q := url.Values{}
	q.Set("name", city)
	q.Set("count", "1")
	q.Set("language", s.language())
	q.Set("format", "json")
	endpoint := "https://geocoding-api.open-meteo.com/v1/search?" + q.Encode()

	var resp openMeteoGeocodeResponse
	if err := s.getJSON(ctx, endpoint, &resp); err != nil {
		return geocodeResult{}, fmt.Errorf("weather: geocode %q: %w", city, err)
	}
	if len(resp.Results) == 0 {
		return geocodeResult{}, fmt.Errorf("weather: no geocoding match for %q", city)
	}
	top := resp.Results[0]
	return geocodeResult{
		Name:        top.Name,
		Latitude:    top.Latitude,
		Longitude:   top.Longitude,
		CountryCode: top.CountryCode,
	}, nil
}

func (s *service) language() string {
	if s.cfg != nil && s.cfg.Language != "" {
		return s.cfg.Language
	}
	return "en"
}

// getJSON issues an HTTP GET and decodes a JSON body into out.
func (s *service) getJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ktw-weather/1.0 (https://github.com/katyoti/katyoti-fwk)")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// ----------------------------------------------------------------------
// Open-Meteo response shapes — only the fields we actually use.
// ----------------------------------------------------------------------

type openMeteoGeocodeResponse struct {
	Results []struct {
		Name        string  `json:"name"`
		Latitude    float64 `json:"latitude"`
		Longitude   float64 `json:"longitude"`
		CountryCode string  `json:"country_code"`
	} `json:"results"`
}

type openMeteoCurrentResponse struct {
	Current struct {
		Time             string  `json:"time"`
		Temperature2M    float64 `json:"temperature_2m"`
		WindSpeed10M     float64 `json:"wind_speed_10m"`
		WindDirection10M float64 `json:"wind_direction_10m"`
		WeatherCode      int     `json:"weather_code"`
	} `json:"current"`
}

type openMeteoForecastResponse struct {
	Daily struct {
		Time             []string  `json:"time"`
		WeatherCode      []int     `json:"weather_code"`
		Temp2MMax        []float64 `json:"temperature_2m_max"`
		Temp2MMin        []float64 `json:"temperature_2m_min"`
		PrecipitationSum []float64 `json:"precipitation_sum"`
		WindSpeed10MMax  []float64 `json:"wind_speed_10m_max"`
	} `json:"daily"`
}

// ----------------------------------------------------------------------
// WMO weather code → human label.
// ----------------------------------------------------------------------

// wmoCodeToCondition maps Open-Meteo's WMO weather code to a short
// human-readable label. See
// https://open-meteo.com/en/docs#weather-code for the canonical
// table; we collapse the 28-value set into 9 buckets to keep CLI
// output compact.
func wmoCodeToCondition(code int) string {
	switch {
	case code == 0:
		return "clear"
	case code <= 3:
		return "partly cloudy"
	case code == 45 || code == 48:
		return "fog"
	case code >= 51 && code <= 57:
		return "drizzle"
	case code >= 61 && code <= 67:
		return "rain"
	case code >= 71 && code <= 77:
		return "snow"
	case code >= 80 && code <= 82:
		return "showers"
	case code >= 95 && code <= 99:
		return "thunderstorm"
	default:
		return fmt.Sprintf("wmo:%d", code)
	}
}

// compassFromDegrees converts a wind-direction-degrees value (0..360,
// north = 0, east = 90) into an 8-point compass abbreviation.
func compassFromDegrees(deg float64) string {
	const fullCircle = 360.0
	d := deg
	for d < 0 {
		d += fullCircle
	}
	for d >= fullCircle {
		d -= fullCircle
	}
	switch idx := int((d+22.5)/45.0) % 8; idx {
	case 0:
		return "N"
	case 1:
		return "NE"
	case 2:
		return "E"
	case 3:
		return "SE"
	case 4:
		return "S"
	case 5:
		return "SW"
	case 6:
		return "W"
	case 7:
		return "NW"
	default:
		return ""
	}
}
