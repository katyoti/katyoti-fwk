---
name: weather-usage
description: When and how to fetch weather data for a city via ktw-weather's WeatherService.
hashtags: [weather, forecast, planning]
category: information
---

# weather-usage

Use ktw-weather when the user asks about **current conditions** or a
**multi-day forecast** for a named place. Two methods are available:

| Method | Tool URI | When to use |
|---|---|---|
| `GetCurrent` | `ktw-weather:weather-service/get-current` | "What's the weather right now in X?" / "Is it raining in X?" |
| `GetForecast` | `ktw-weather:weather-service/get-forecast` | "What's the weather looking like next week in X?" / "Will I need a jacket in X tomorrow?" |

## Inputs

Both methods take a `city` argument — the human-readable name. The
service resolves it to lat/long via Open-Meteo's geocoding endpoint and
returns the first match (ordered by population/relevance). You don't
need to disambiguate the country.

`GetForecast` also takes a `days` argument (1..16, default 5). Open-Meteo
caps the free tier at 16-day horizons.

## Outputs

`GetCurrent` returns a single `CurrentWeather` record with
temperature, wind, condition, and the local-time timestamp.

`GetForecast` returns a `Forecast` envelope: the resolved city +
an array of `ForecastDay` rows (date / condition / max temp / min temp /
precipitation / wind). The CLI auto-renders this as a table; an LLM
consumer parses the JSON directly.

## Rendering the forecast as a UI page (graphs)

When the user wants the forecast **as a page / dashboard / with graphs**,
don't just print the table — render it. The `forecast-day` data-type
(`ktw-weather:/data-type/forecast-day`) renders the rows as a schema-bound
**table** or **view**, and for a richer page (KPI cards + a temperature
**line chart** + a precipitation **bar chart**) follow the generic
**`render-data-as-ui`** skill (ktw-methods): call `get-forecast`, then emit a
`display-ui-component` page DSL binding the `days[]` rows to chart nodes. The
worked forecast-page example lives in that skill.

## CLI examples (standalone, no hub required)

```bash
# Snapshot of current conditions
ktw-weather get-current --city "Paris"

# 7-day forecast as a column-aligned table
ktw-weather get-forecast --city "Tokyo" --days 7

# Same call, JSON output for piping into jq
ktw-weather get-forecast --city "Tokyo" --days 7 --out json | jq '.days[0]'

# What slot methods does this binary expose?
ktw-weather ktsp describe --json | jq '.ktsp.slot_types[] | select(.type=="WeatherService")'
```

## Units

The service returns metric units by default:
- Temperature in degrees Celsius
- Wind speed in km/h
- Precipitation in millimeters

The `WeatherService.yaml` config has a `unit` field for future
conversion support; today it's advisory.

## Failure modes

| Symptom | Cause | What to tell the user |
|---|---|---|
| `no geocoding match for "<city>"` | City name didn't resolve | Try a more specific spelling or include the country (e.g. "Paris, TX" not "Paris") |
| `upstream returned status 5xx` | Open-Meteo transient outage | Suggest retrying in a moment |
| HTTP timeout (5s) | Network or upstream slowness | Same — retry |

## Don't use ktw-weather for

- Historical weather (Open-Meteo has an archive endpoint not exposed yet)
- Severe-weather alerts (out of scope for v1)
- Marine / aviation specialized forecasts (different APIs)
