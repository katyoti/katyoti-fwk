---
name: llm-catalog
description: >-
  Build a unified CSV catalog of LLM models from Artificial Analysis and
  OpenRouter APIs. Includes an embedded TypeScript script (bun) for data
  extraction, filtering, and merging — no LLM needed for the transformation.
iconName: database
icon: "📊"
hashtags: [data, llm, catalog, api]
category: data
provides: [task]
---

# LLM Catalog Builder

## Scripts

### `scripts/build-catalog.ts`

A TypeScript script (runs with bun) that reads both API JSON files, filters to
major providers, merges by provider+model_id, and outputs a sorted CSV.

**Providers kept:** anthropic, openai, google, mistralai, cohere

**Usage via skills-run-script:**
```
skills-run-script(
  skill_name="llm-catalog",
  script_path="scripts/build-catalog.ts",
  args=["artificialanalysis-models.json", "openrouter-models.json", "llm-catalog.csv"]
)
```

**Output columns:**
`provider, model_id, name, context_window, max_output, cost_input_per_mtok, cost_output_per_mtok, speed_tok_s, latency_s, intelligence_score, coding_score, math_score, source`

Sorted by provider, then model_id.

## Process

1. `rest-api-fetch(endpoint="artificialanalysis-models")` → downloads JSON
2. `rest-api-fetch(endpoint="openrouter-models")` → downloads JSON
3. `skills-run-script(skill_name="llm-catalog", script_path="scripts/build-catalog.ts", args=[...])` → produces CSV
4. `file-read` the CSV and present summary
