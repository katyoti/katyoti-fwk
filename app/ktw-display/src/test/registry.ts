// SPDX-License-Identifier: Apache-2.0
// Case registry — drop-a-file scan of `tests/cases/`.
//
// Each file is a `.yaml` declaring a TestCase (id, category, title,
// description, schema, value, mode, tags, optional script). Refactored
// 2026-05-06 (display-package-restructure Phase 2.3): replaced the
// dynamic `.ts` import + walkTsFiles + import() with a YAML reader.
// The schema parser handles the canonical AST regardless of source
// (YAML object → schema parser is sufficient; no custom YAML tags).
//
// Hot-reload is NOT supported — the harness restarts to pick up new
// cases (acceptable for a dev tool).

import { readdirSync, statSync, readFileSync } from 'node:fs'
import { join, resolve, sep, posix } from 'node:path'
import { parse as yamlParse } from 'yaml'
import type { TestCase, CaseSummary } from './types.ts'

let cases: Map<string, TestCase> | null = null

/**
 * Scan tests/cases/ for YAML case files and load them. Idempotent:
 * subsequent calls return the cached registry.
 */
export async function loadCases(): Promise<Map<string, TestCase>> {
  if (cases !== null) return cases

  const result = new Map<string, TestCase>()
  // Resolve tests/cases/ relative to this module's location at runtime.
  // From src/test/registry.ts → tests/cases/ requires going up to project
  // root then into tests/cases/.
  const here = new URL('.', import.meta.url).pathname
  const cleaned = process.platform === 'win32' && here.startsWith('/') ? here.slice(1) : here
  // src/test/ → ../../tests/cases/
  const testCasesDir = resolve(cleaned, '..', '..', '..', 'tests', 'cases')

  const files = walkYamlFiles(testCasesDir)
  for (const filePath of files) {
    try {
      const text = readFileSync(filePath, 'utf8')
      const def = yamlParse(text) as TestCase | undefined
      if (!def || typeof def !== 'object' || typeof def.id !== 'string') {
        console.error(`ktw-display.test.registry: ${filePath} is not a valid TestCase YAML — skipping`)
        continue
      }
      if (result.has(def.id)) {
        console.error(`ktw-display.test.registry: duplicate case id "${def.id}" in ${filePath} — skipping`)
        continue
      }
      result.set(def.id, def)
    } catch (err) {
      console.error(`ktw-display.test.registry: failed to load ${filePath}: ${(err as Error).message}`)
    }
  }

  cases = result
  return cases
}

/**
 * Return the catalog as slim summaries. Used by /test/api/cases.
 */
export async function listCases(): Promise<CaseSummary[]> {
  const map = await loadCases()
  const out: CaseSummary[] = []
  for (const c of map.values()) {
    out.push({
      id: c.id,
      category: c.category,
      title: c.title,
      description: c.description,
      url: `/test/${c.id}`,
      hasScript: Array.isArray(c.script) && c.script.length > 0,
    })
  }
  // Sort by category then title for stable tree rendering.
  out.sort((a, b) => {
    if (a.category !== b.category) return a.category.localeCompare(b.category)
    return a.title.localeCompare(b.title)
  })
  return out
}

/**
 * Look up a case by id. Returns undefined if missing.
 */
export async function getCase(id: string): Promise<TestCase | undefined> {
  const map = await loadCases()
  return map.get(id)
}

/**
 * Walk a directory recursively and return all *.yaml file paths.
 * Skips files starting with `_` (convention for private helpers).
 */
function walkYamlFiles(dir: string): string[] {
  const out: string[] = []
  let entries: string[]
  try {
    entries = readdirSync(dir)
  } catch {
    // Directory doesn't exist yet — return empty list.
    return out
  }
  for (const entry of entries) {
    if (entry.startsWith('_') || entry.startsWith('.')) continue
    const full = join(dir, entry)
    let st
    try {
      st = statSync(full)
    } catch {
      continue
    }
    if (st.isDirectory()) {
      out.push(...walkYamlFiles(full))
    } else if (st.isFile() && (entry.endsWith('.yaml') || entry.endsWith('.yml'))) {
      out.push(full.split(sep).join(posix.sep))
    }
  }
  return out
}
