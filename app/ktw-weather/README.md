# ktw-weather

Weather forecast sidecar for the ktw-agents ecosystem. Backed by
[Open-Meteo](https://open-meteo.com) (free, no API key, JSON over HTTPS).

**Why this exists.** ktw-weather is the **canonical minimum-viable
sidecar template** for the unified contribution system (see
[modules/contributions/concepts.md](../../modules/contributions/concepts.md)).
It exists primarily to:

1. **Validate** the unified-contribution-system architecture end-to-end
   — every other ms-rc1 piece flows through this binary at least once.
2. **Anchor** future sidecar authors — `cp -r app/ktw-weather app/ktw-foo`
   is the fastest path to a new sidecar.
3. **Measure** binary weight — a sidecar with one service, one skill,
   one config, and ~80 LOC of domain code should ship in **< 18 MB
   stripped** (RC1 gate per 5Q).

## Surface

One published service, two methods:

| Method | Slot URI | Input | Output |
|---|---|---|---|
| `GetCurrent`  | `slot://ktw-weather/weather-service/default/get-current`  | `{city}`              | `CurrentWeather` (snapshot) |
| `GetForecast` | `slot://ktw-weather/weather-service/default/get-forecast` | `{city, days (1..16)}` | `Forecast` (table of `ForecastDay` rows) |

One skill:

- `ktw-weather:/skill/weather-usage` — when/how an LLM should call us

One config file:

- `WeatherService.yaml` — defaults (unit=metric, language=en)

## CLI examples

```bash
# Current conditions
ktw-weather get-current Paris

# 5-day forecast — auto-renders as a column-aligned table thanks to
# the `ui-cli:"mode=table,..."` tag on Forecast.Days
ktw-weather get-forecast Tokyo --days 5

# Force JSON regardless of TTY
ktw-weather get-forecast Tokyo --days 5 --out json

# What does the binary expose?
ktw-weather ktsp describe --json
ktw-weather skills list
ktw-weather config check
```

## Architecture

The binary is the canonical 9-extension sidecar pattern:

```
buildExtensions(efs) = [
  appcore.NewAppExt(...)                                // 1: app identity
  appcore.SidecarExt(efs)                               // 2: ConfigService + config CLI
  manifestlocal.Extension()                             // 3: ContributionService slot + CLI
  manifestlocal.ContributeConfigResources(efs)          // 4: config-* Contributions
  weatherlocal.Extension()                              // 5: domain — WeatherService
  skillslocal.Extension()                               // 6: SkillManager
  skillslocal.ContributeSkillResources(...)             // 7: embedded skills
  scriptslocal.Extension()                              // 8: ScriptService (zero scripts shipped today)
  ktslocal.AutoKTSPContainer(..., WithFlatCLI())        // 9: KTSP wire-up
  ktslocal.AutoHTTPServer(...)                          // 10: HTTP wire-up
]
```

The only domain code lives in `internal/extensions/weather/` — three
files (~430 LOC total):

- `api/weather.go` (~95 LOC) — interface, ServiceDef, typed Input/Output
  with `desc:` + `ui-cli:` tags
- `internal/core/service.go` (~300 LOC) — Open-Meteo HTTP client, two
  upstream calls per request (geocode → forecast), WMO weather-code
  bucketing, compass-direction conversion
- `local/services.go` (~30 LOC) — one `PublishService` call

The `ui-cli:"mode=table,columns=date;condition;tempMax;..."` tag on
`Forecast.Days` is what makes `get-forecast` render as a column-aligned
table on TTY and as plain JSON when piped. Zero per-binary CLI code.

## Open-Meteo specifics

- **Free tier**: no API key, no rate limit for typical use.
- **Geocoding**: `https://geocoding-api.open-meteo.com/v1/search?name=<city>`
  returns lat/long + country code. We take the first match.
- **Forecast**: `https://api.open-meteo.com/v1/forecast?latitude=...&longitude=...&daily=...&forecast_days=N`
  with `forecast_days` capped at 16 by Open-Meteo.
- **HTTP timeout**: 5 seconds per request (short enough to keep CLI
  responsive, long enough for cold paths).

## Failure modes

| Symptom | Cause | Behaviour |
|---|---|---|
| `no geocoding match for "<city>"` | City name didn't resolve | Error returned; no upstream forecast call |
| `upstream returned status N` | Open-Meteo non-2xx | Error returned with the status code |
| HTTP timeout | Network slow / Open-Meteo down | Error after 5s; user should retry |

## RC1 validation gate (5Q)

This sidecar validates:

- The unified contribution envelope flows correctly through a new sidecar
- The `ui-cli` schema extension (3L) drives terminal rendering end-to-end
  for table-shaped outputs
- The NATS-package split (5O) + multi-runner relocation keep sidecars
  small — measured **13.4 MB stripped** for ktw-weather (well under the
  18 MB gate)
- Authoring time is <1 hour for a new sidecar (this one took ~30 min
  end-to-end including this README)
