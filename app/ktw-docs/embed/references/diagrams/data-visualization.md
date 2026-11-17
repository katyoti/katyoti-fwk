# Data Visualization

Syntax reference for Vega and Vega-Lite charts.

---

## Vega-Lite (Recommended for Most Charts)

Vega-Lite is a high-level grammar for interactive data visualization. Prefer Vega-Lite over Vega for standard charts — it is much more concise.

### Bar Chart

```vegalite
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 400,
  "height": 300,
  "data": {
    "values": [
      {"category": "A", "value": 28},
      {"category": "B", "value": 55},
      {"category": "C", "value": 43},
      {"category": "D", "value": 91}
    ]
  },
  "mark": "bar",
  "encoding": {
    "x": {"field": "category", "type": "nominal", "title": "Category"},
    "y": {"field": "value", "type": "quantitative", "title": "Value"},
    "color": {"field": "category", "type": "nominal", "legend": null}
  }
}
```

### Line Chart (Time Series)

```vegalite
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 500,
  "height": 300,
  "data": {
    "values": [
      {"date": "2025-01", "value": 100},
      {"date": "2025-02", "value": 130},
      {"date": "2025-03", "value": 120},
      {"date": "2025-04", "value": 170},
      {"date": "2025-05", "value": 160},
      {"date": "2025-06", "value": 200}
    ]
  },
  "mark": {"type": "line", "point": true},
  "encoding": {
    "x": {"field": "date", "type": "temporal", "title": "Month"},
    "y": {"field": "value", "type": "quantitative", "title": "Revenue ($K)"}
  }
}
```

### Scatter Plot

```vegalite
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 400,
  "height": 400,
  "data": {
    "values": [
      {"x": 1, "y": 2, "group": "A"},
      {"x": 3, "y": 5, "group": "A"},
      {"x": 5, "y": 3, "group": "B"},
      {"x": 7, "y": 8, "group": "B"},
      {"x": 9, "y": 6, "group": "B"}
    ]
  },
  "mark": "point",
  "encoding": {
    "x": {"field": "x", "type": "quantitative"},
    "y": {"field": "y", "type": "quantitative"},
    "color": {"field": "group", "type": "nominal"},
    "size": {"value": 100}
  }
}
```

### Stacked Area Chart

```vegalite
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 500,
  "height": 300,
  "data": {
    "values": [
      {"date": "2025-01", "category": "Web", "value": 50},
      {"date": "2025-01", "category": "Mobile", "value": 30},
      {"date": "2025-02", "category": "Web", "value": 60},
      {"date": "2025-02", "category": "Mobile", "value": 45},
      {"date": "2025-03", "category": "Web", "value": 70},
      {"date": "2025-03", "category": "Mobile", "value": 55}
    ]
  },
  "mark": "area",
  "encoding": {
    "x": {"field": "date", "type": "temporal"},
    "y": {"field": "value", "type": "quantitative", "stack": true},
    "color": {"field": "category", "type": "nominal"}
  }
}
```

### Heatmap

```vegalite
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 400,
  "height": 300,
  "data": {
    "values": [
      {"day": "Mon", "hour": "9AM", "value": 10},
      {"day": "Mon", "hour": "12PM", "value": 30},
      {"day": "Mon", "hour": "3PM", "value": 20},
      {"day": "Tue", "hour": "9AM", "value": 15},
      {"day": "Tue", "hour": "12PM", "value": 40},
      {"day": "Tue", "hour": "3PM", "value": 25}
    ]
  },
  "mark": "rect",
  "encoding": {
    "x": {"field": "hour", "type": "nominal"},
    "y": {"field": "day", "type": "nominal"},
    "color": {"field": "value", "type": "quantitative", "scale": {"scheme": "blues"}}
  }
}
```

### Horizontal Bar Chart (Ranked)

```vegalite
{
  "$schema": "https://vega.github.io/schema/vega-lite/v5.json",
  "width": 400,
  "height": 200,
  "data": {
    "values": [
      {"item": "Feature A", "votes": 120},
      {"item": "Feature B", "votes": 98},
      {"item": "Feature C", "votes": 75},
      {"item": "Feature D", "votes": 60}
    ]
  },
  "mark": "bar",
  "encoding": {
    "y": {"field": "item", "type": "nominal", "sort": "-x"},
    "x": {"field": "votes", "type": "quantitative"}
  }
}
```

### Key Concepts

**Mark types**: `bar`, `line`, `point`, `area`, `rect`, `circle`, `square`, `tick`, `rule`, `text`, `arc`

**Encoding channels**: `x`, `y`, `color`, `size`, `shape`, `opacity`, `row`, `column`, `facet`, `detail`, `tooltip`

**Data types**: `quantitative` (numbers), `temporal` (dates), `nominal` (categories), `ordinal` (ordered categories)

**Aggregations**: `count`, `sum`, `mean`, `median`, `min`, `max`, `stdev`, `variance`

**Transforms**: `filter`, `calculate`, `aggregate`, `bin`, `timeUnit`, `sort`, `window`, `fold`, `flatten`

---

## Vega (Full Grammar)

Use Vega when you need features beyond Vega-Lite: custom interactions, complex layouts, or fine-grained control. Vega specs are verbose — only use when Vega-Lite cannot express the visualization.

### Minimal Example

```vega
{
  "$schema": "https://vega.github.io/schema/vega/v5.json",
  "width": 400,
  "height": 200,
  "data": [
    {
      "name": "table",
      "values": [
        {"x": "A", "y": 28},
        {"x": "B", "y": 55},
        {"x": "C", "y": 43}
      ]
    }
  ],
  "scales": [
    {"name": "xscale", "type": "band", "domain": {"data": "table", "field": "x"}, "range": "width", "padding": 0.1},
    {"name": "yscale", "type": "linear", "domain": {"data": "table", "field": "y"}, "range": "height", "nice": true}
  ],
  "axes": [
    {"orient": "bottom", "scale": "xscale"},
    {"orient": "left", "scale": "yscale"}
  ],
  "marks": [
    {
      "type": "rect",
      "from": {"data": "table"},
      "encode": {
        "enter": {
          "x": {"scale": "xscale", "field": "x"},
          "width": {"scale": "xscale", "band": 1},
          "y": {"scale": "yscale", "field": "y"},
          "y2": {"scale": "yscale", "value": 0},
          "fill": {"value": "steelblue"}
        }
      }
    }
  ]
}
```

### When to Use Vega vs Vega-Lite

| Need | Use |
|---|---|
| Standard charts (bar, line, scatter, area, heatmap) | Vega-Lite |
| Multi-view compositions, layered charts | Vega-Lite (`layer`, `concat`, `facet`) |
| Custom animations, interactions | Vega |
| Non-standard mark types or layouts | Vega |
| Complex data transformations | Vega |
