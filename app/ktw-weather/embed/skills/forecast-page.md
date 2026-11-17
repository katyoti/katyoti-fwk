---
name: forecast-page
description: >-
  Render a ktw-weather forecast as a UI page with charts — temperature,
  rain, wind, clouds — for a city. Use when the user asks to SEE /
  visualize / "show me a page for" a city's forecast, not just read the
  numbers. Reusable by any agent that has the ktw-weather methods +
  display-ui-component (the weather-forecast-page task is just a thin
  launcher over this).
hashtags: [weather, forecast, ui, charts, dashboard]
category: lifecycle
advertised: true
---

# forecast-page

Visualize a city's weather forecast as a UI **page** (not a raw table). This is
the weather-specific entry; it builds on the generic
**`render-data-as-ui`** skill (ktw-methods) for the page-DSL grammar + chart
palette. This skill owns the full how-to — anything that needs to render a
weather forecast loads it.

**Render the whole page in French** — heading, KPI labels, chart titles, axis
and series labels, the refinement question. Use French weather vocabulary
(Température, Précipitations, Vent, Nuages, Max/Min, Pluie).

## Procedure

1. **City.** Use the city you were given (task input / the conversation). If
   none, ask the user ONE question via **ask-user** (in French): « Pour quelle
   ville voulez-vous les prévisions ? »
2. **Fetch.** Call `ktw-weather:weather-service/get-forecast` with
   `{ "city": <city>, "days": 7 }`. Keep the `days[]` rows (date / tempMax /
   tempMin / precipitationMm / windKmh / cloud cover / condition). Never
   fabricate data; if the call fails, tell the user. (`get-current` is there for
   a current-conditions snapshot if you want one.)
3. **Add a French day label per row.** The charts must show the **day**, not the
   raw ISO date, on the x-axis. For each `days[]` row, derive a `jour` field from
   its `date` — the French weekday abbreviation + day-of-month, e.g. `"Ven 5"`
   (Lun / Mar / Mer / Jeu / Ven / Sam / Dim + the day number). Bind the charts'
   `dataKey` to `"jour"`, not `"date"`.
4. **Build the page.** Follow `ktw-methods:/skill/render-data-as-ui` — its
   worked example IS this page: a `vbox` with a `heading` (« <ville> —
   Prévisions 7 jours »), a KPI `grid` (max du jour, min du jour, pluie totale
   sur 7 jours — all labelled in French), a `line` chart titled « Température
   (°C) » binding `series:[tempMax,tempMin]` (labels « Max » / « Min ») over
   `dataKey:"jour"`, and a `bar` chart titled « Précipitations (mm) » for
   `precipitationMm` (« Pluie »). Add Vent / Nuages series where they fit. Pass
   the `days[]` rows (now carrying `jour`) straight into each chart's `data`.
5. **Emit** via `display-ui-component`
   `{ kind: "page", title: "<ville> — Prévisions 7 jours", dslTree, data: { city } }`.
6. **Offer one refinement** (in French — autre période, ajouter une mesure,
   changer un type de graphe…). For the **raw rows** instead of charts, render
   the `forecast-day` data-type (`ktw-weather:/data-type/forecast-day`) as a
   `table` / `view` — no page DSL needed.

## Don't
- Don't fabricate days to fill a chart — pass the real `get-forecast` rows or
  show the engine's empty state.
- Don't hand-roll custom TSX — the line/bar/KPI nodes already exist in the DSL.
