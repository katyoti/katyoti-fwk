// SPDX-License-Identifier: Apache-2.0
// One-shot migration script: TS test cases → YAML.
//
// Reads each `src/test-cases/<category>/<name>.ts`, imports the default
// export (must be a TestCase), serializes to YAML, writes to
// `tests/cases/<category>/<name>.yaml`. Run once (`bun run tests/migrate-test-cases.ts`),
// manually diff/review the output, then delete the .ts files.
//
// Wired by display-package-restructure Phase 2.1 (2026-05-06).

import { readdir, mkdir, writeFile } from 'node:fs/promises'
import { join, dirname, posix } from 'node:path'
import { existsSync, statSync } from 'node:fs'
import { stringify as yamlStringify } from 'yaml'

import type { TestCase } from '../src/test/types.ts'

const SRC_ROOT = join(import.meta.dir, '..', 'src', 'test-cases')
const DST_ROOT = join(import.meta.dir, 'cases')

async function walkTs(dir: string): Promise<string[]> {
  const out: string[] = []
  if (!existsSync(dir)) return out
  for (const entry of await readdir(dir)) {
    if (entry.startsWith('_') || entry.startsWith('.')) continue
    const full = join(dir, entry)
    const st = statSync(full)
    if (st.isDirectory()) {
      out.push(...(await walkTs(full)))
    } else if (st.isFile() && entry.endsWith('.ts') && !entry.endsWith('.d.ts')) {
      out.push(full)
    }
  }
  return out
}

async function migrate() {
  const files = await walkTs(SRC_ROOT)
  console.log(`Found ${files.length} TS test cases`)

  let written = 0
  for (const filePath of files) {
    try {
      // Use POSIX-style path for dynamic import on Windows.
      const importPath = filePath.split(/[\\/]/).join(posix.sep)
      const mod = await import(importPath)
      const tc = (mod.default ?? mod.testCase ?? mod.case_) as TestCase | undefined
      if (!tc || typeof tc !== 'object' || typeof tc.id !== 'string') {
        console.error(`SKIP: ${filePath} — no default TestCase export`)
        continue
      }

      // Serialize to YAML. Use defaultStringType: 'QUOTE_DOUBLE' for safety
      // around special characters; lineWidth: 0 to avoid forced wrapping.
      const yaml = yamlStringify(tc, {
        lineWidth: 0,
        indent: 2,
        defaultStringType: 'PLAIN',
        defaultKeyType: 'PLAIN',
      })

      // Compute target path: src/test-cases/foo/bar.ts → tests/cases/foo/bar.yaml
      const rel = filePath.slice(SRC_ROOT.length).replace(/\\/g, '/').replace(/^\//, '')
      const targetPath = join(DST_ROOT, rel.replace(/\.ts$/, '.yaml'))
      await mkdir(dirname(targetPath), { recursive: true })
      await writeFile(targetPath, yaml, 'utf8')
      written++
      console.log(`  wrote ${rel.replace(/\.ts$/, '.yaml')}`)
    } catch (err) {
      console.error(`FAIL: ${filePath} — ${(err as Error).message}`)
    }
  }

  console.log(`\nDone. ${written}/${files.length} cases migrated.`)
  console.log(`Review the YAML files under tests/cases/, then delete the TS files.`)
}

migrate().catch((err) => {
  console.error(err)
  process.exit(1)
})
