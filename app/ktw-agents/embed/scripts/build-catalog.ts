#!/usr/bin/env bun
// SPDX-License-Identifier: Apache-2.0
/**
 * build-catalog.ts — Merge LLM model data from Artificial Analysis + OpenRouter into CSV
 *
 * Usage: bun run build-catalog.ts <aa-input.json> <or-input.json> <output.csv>
 *
 * Filters to major providers: Google, OpenAI, Anthropic, Mistral, Cohere
 * Merges by provider + model_id. Sorted by provider, model_id.
 */

const MAJOR_PROVIDERS: Record<string, string> = {
  // Artificial Analysis creator names → normalized provider
  "OpenAI": "openai",
  "Anthropic": "anthropic",
  "Google": "google",
  "Google DeepMind": "google",
  "Mistral": "mistralai",
  "Mistral AI": "mistralai",
  "Cohere": "cohere",
};

const OR_PROVIDER_PREFIXES = ["openai", "anthropic", "google", "mistralai", "cohere"];

interface AAModel {
  name: string;
  slug: string;
  model_creator: { name: string; slug: string };
  evaluations?: {
    artificial_analysis_intelligence_index?: number;
    coding_index?: number;
    math_index?: number;
  };
  pricing?: {
    input_cost_per_1m_tokens?: number;
    output_cost_per_1m_tokens?: number;
  };
  median_output_tokens_per_second?: number;
  median_time_to_first_token_seconds?: number;
}

interface ORModel {
  id: string;
  name: string;
  context_length?: number;
  top_provider?: { max_completion_tokens?: number };
  pricing?: { prompt?: string; completion?: string };
}

interface CatalogRow {
  provider: string;
  model_id: string;
  name: string;
  context_window: string;
  max_output: string;
  cost_input_per_mtok: string;
  cost_output_per_mtok: string;
  speed_tok_s: string;
  latency_s: string;
  intelligence_score: string;
  coding_score: string;
  math_score: string;
  source: string;
}

// ── Parse args ─────────────────────────────────────────────────────────
const [aaPath, orPath, outPath] = Bun.argv.slice(2);
if (!aaPath || !orPath) {
  console.error("Usage: bun run build-catalog.ts <aa.json> <or.json> [output.csv]");
  process.exit(1);
}
const outputPath = outPath || "llm-catalog.csv";

// ── Load JSON files ────────────────────────────────────────────────────
const aaRaw = await Bun.file(aaPath).json();
const orRaw = await Bun.file(orPath).json();

const aaModels: AAModel[] = aaRaw.data || [];
const orModels: ORModel[] = orRaw.data || [];

// ── Filter to major providers ──────────────────────────────────────────
const aaFiltered = aaModels.filter(m => m.model_creator?.name in MAJOR_PROVIDERS);
const orFiltered = orModels.filter(m => {
  const prefix = m.id.split("/")[0];
  return OR_PROVIDER_PREFIXES.includes(prefix);
});

console.error(`AA: ${aaFiltered.length}/${aaModels.length} models (major providers)`);
console.error(`OR: ${orFiltered.length}/${orModels.length} models (major providers)`);

// ── Catalog keyed by "provider|model_id" ────────────────────────────────
const catalog = new Map<string, CatalogRow>();

function catalogKey(provider: string, modelId: string): string {
  return `${provider}|${modelId}`;
}

// ── Load OpenRouter models (has context/pricing) ────────────────────────
for (const m of orFiltered) {
  const provider = m.id.split("/")[0];
  const modelId = m.id.split("/").slice(1).join("/");
  const key = catalogKey(provider, modelId);

  const promptPrice = m.pricing?.prompt ? parseFloat(m.pricing.prompt) * 1_000_000 : 0;
  const completionPrice = m.pricing?.completion ? parseFloat(m.pricing.completion) * 1_000_000 : 0;

  catalog.set(key, {
    provider,
    model_id: modelId,
    name: cleanName(m.name),
    context_window: m.context_length?.toString() || "",
    max_output: m.top_provider?.max_completion_tokens?.toString() || "",
    cost_input_per_mtok: promptPrice ? round2(promptPrice) : "",
    cost_output_per_mtok: completionPrice ? round2(completionPrice) : "",
    speed_tok_s: "",
    latency_s: "",
    intelligence_score: "",
    coding_score: "",
    math_score: "",
    source: "openrouter",
  });
}

// ── Enrich/add from Artificial Analysis (has benchmarks + speed) ────────
let merged = 0;
let added = 0;

for (const m of aaFiltered) {
  const provider = MAJOR_PROVIDERS[m.model_creator.name];

  // Try exact key match first
  let matchKey = findMatch(m.slug, m.name, provider, catalog);

  if (matchKey) {
    const row = catalog.get(matchKey)!;
    enrichRow(row, m);
    row.source = "both";
    merged++;
  } else {
    // Add as AA-only
    const modelId = m.slug;
    const key = catalogKey(provider, modelId);
    catalog.set(key, {
      provider,
      model_id: modelId,
      name: m.name,
      context_window: "",
      max_output: "",
      cost_input_per_mtok: m.pricing?.input_cost_per_1m_tokens ? round2(m.pricing.input_cost_per_1m_tokens) : "",
      cost_output_per_mtok: m.pricing?.output_cost_per_1m_tokens ? round2(m.pricing.output_cost_per_1m_tokens) : "",
      speed_tok_s: m.median_output_tokens_per_second ? round2(m.median_output_tokens_per_second) : "",
      latency_s: m.median_time_to_first_token_seconds ? round2(m.median_time_to_first_token_seconds) : "",
      intelligence_score: m.evaluations?.artificial_analysis_intelligence_index?.toString() || "",
      coding_score: m.evaluations?.coding_index?.toString() || "",
      math_score: m.evaluations?.math_index?.toString() || "",
      source: "artificialanalysis",
    });
    added++;
  }
}

console.error(`Merged: ${merged}, AA-only added: ${added}`);

// ── Sort by provider, then model_id ─────────────────────────────────────
const rows = [...catalog.values()].sort((a, b) => {
  if (a.provider !== b.provider) return a.provider.localeCompare(b.provider);
  return a.model_id.localeCompare(b.model_id);
});

// ── Write CSV ───────────────────────────────────────────────────────────
const header = "provider,model_id,name,context_window,max_output,cost_input_per_mtok,cost_output_per_mtok,speed_tok_s,latency_s,intelligence_score,coding_score,math_score,source";
const csvLines = rows.map(r => [
  csvEscape(r.provider),
  csvEscape(r.model_id),
  csvEscape(r.name),
  r.context_window,
  r.max_output,
  r.cost_input_per_mtok,
  r.cost_output_per_mtok,
  r.speed_tok_s,
  r.latency_s,
  r.intelligence_score,
  r.coding_score,
  r.math_score,
  r.source,
].join(","));

const csv = [header, ...csvLines].join("\n") + "\n";
await Bun.write(outputPath, csv);

console.error(`Done: ${rows.length} models → ${outputPath}`);
console.log(`${rows.length} models written to ${outputPath}`);

// ── Helpers ──────────────────────────────────────────────────────────────

function cleanName(name: string): string {
  return name.replace(/^(Anthropic|OpenAI|Google|Mistral|Cohere):\s*/i, "").trim();
}

function round2(n: number): string {
  return (Math.round(n * 100) / 100).toString();
}

function csvEscape(s: string): string {
  if (s.includes(",") || s.includes('"') || s.includes("\n")) {
    return `"${s.replace(/"/g, '""')}"`;
  }
  return s;
}

function normalize(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9]/g, "");
}

function enrichRow(row: CatalogRow, m: AAModel): void {
  if (m.evaluations?.artificial_analysis_intelligence_index) {
    row.intelligence_score = m.evaluations.artificial_analysis_intelligence_index.toString();
  }
  if (m.evaluations?.coding_index) {
    row.coding_score = m.evaluations.coding_index.toString();
  }
  if (m.evaluations?.math_index) {
    row.math_score = m.evaluations.math_index.toString();
  }
  if (m.median_output_tokens_per_second) {
    row.speed_tok_s = round2(m.median_output_tokens_per_second);
  }
  if (m.median_time_to_first_token_seconds) {
    row.latency_s = round2(m.median_time_to_first_token_seconds);
  }
  if (!row.cost_input_per_mtok && m.pricing?.input_cost_per_1m_tokens) {
    row.cost_input_per_mtok = round2(m.pricing.input_cost_per_1m_tokens);
  }
  if (!row.cost_output_per_mtok && m.pricing?.output_cost_per_1m_tokens) {
    row.cost_output_per_mtok = round2(m.pricing.output_cost_per_1m_tokens);
  }
}

function findMatch(aaSlug: string, aaName: string, provider: string, catalog: Map<string, CatalogRow>): string | undefined {
  // Try exact slug match against model_id
  for (const [key, row] of catalog) {
    if (row.provider !== provider) continue;
    if (normalize(row.model_id) === normalize(aaSlug)) return key;
  }

  // Try normalized name match
  const aaNorm = normalize(aaName);
  for (const [key, row] of catalog) {
    if (row.provider !== provider) continue;
    const orNorm = normalize(row.name);
    if (orNorm === aaNorm) return key;
    // Substring — AA name in OR name or vice versa (min 8 chars to avoid false matches)
    if (aaNorm.length >= 8 && orNorm.length >= 8) {
      if (orNorm.includes(aaNorm) || aaNorm.includes(orNorm)) return key;
    }
  }

  return undefined;
}
