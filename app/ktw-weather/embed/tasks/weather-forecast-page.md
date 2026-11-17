---
name: weather-forecast-page
description: Ask for a city and render its 7-day weather forecast as a UI page with charts — temperature, rain, wind, clouds.
type: task
iconName: cloud-sun
icon: "🌦️"
hashtags: [weather, forecast, ui, charts, dashboard, demo]
category: weather
config:
  capability: fast
  # chat = interactive: ask-user routes inline (direct), follow-ups loop back.
  mode: chat
  # Lean tool surface — only what this task needs: ask the city, call the
  # weather methods, mount the page.
  plugins:
    - ask-user
    - display-ui-component
  # The knowledge lives in the skill (reusable by any tool); this task just
  # loads it. render-data-as-ui is the page-DSL/chart dependency it builds on.
  skills:
    - "ktw-weather:/skill/forecast-page"
    - "ktw-methods:/skill/render-data-as-ui"
  tools:
    - "ktw-weather:weather-service/get-current"
    - "ktw-weather:weather-service/get-forecast"
---

# Weather forecast page

Load **`ktw-weather:/skill/forecast-page`** and follow it. Use the `city` input
if present; otherwise the skill asks for it.
