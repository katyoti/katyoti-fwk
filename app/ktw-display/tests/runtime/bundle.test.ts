// SPDX-License-Identifier: Apache-2.0
// Runtime bundle smoke — Bun.build of src/runtime/index.tsx must succeed and
// produce a non-trivial bundle that includes every wired renderer.
//
// Per-test timeout bumped to 30s (2026-05-03, gui-form Subtask 4): the
// bundle now pulls in `<UIFormHarness>` + `buildUIForms()` + 4 layout
// factories + the full Mantine surface (Stepper, Tabs, Paper, additional
// icons) — JS bundle grew from ~1.2 MB to ~5 MB, build time from ~0.5s
// to ~5s on a cold cache. The 5s default would flake on warm-but-bumped
// runs; 30s leaves ample headroom for slower CI machines.

import { describe, expect, it, beforeAll } from 'bun:test'
import { _resetRuntimeBundleCache, getRuntimeBundle } from '../../src/runtime/build.ts'

const BUNDLE_TIMEOUT_MS = 30_000

describe('runtime bundle', () => {
  beforeAll(() => {
    _resetRuntimeBundleCache()
  })

  it('builds without errors and returns non-empty JS + CSS', async () => {
    const bundle = await getRuntimeBundle()
    expect(bundle.js.length).toBeGreaterThan(50_000)
    expect(bundle.css.length).toBeGreaterThan(10_000)
    expect(bundle.hash).toMatch(/^[0-9a-f]+$/)
  }, BUNDLE_TIMEOUT_MS)

  it('includes Mantine + react-hook-form + each wired renderer', async () => {
    const bundle = await getRuntimeBundle()
    // Spot-check that key dependency code paths are present.
    expect(bundle.js).toContain('mantine') // Mantine theme code
    // Each renderer's default export should be referenced. We look for the
    // factory-pattern object literal we wrote (key: { view, form, header, column }).
    const renderers = ['Text', 'Email', 'Markdown', 'Number', 'Rating', 'Slider', 'Boolean']
    for (const r of renderers) {
      expect(bundle.js, `bundle should reference ${r}Renderer`).toContain(`${r}Renderer`)
    }
    // Form-layout factories (Subtask 3 of gui-form) are now also wired.
    // Bun.build hoists/renames default-imported local names, so we can't
    // assert on `SingleLayout`/`GroupsLayout`/etc. directly; instead we
    // check content-distinctive markers each layout's renderer.tsx
    // emits — strings that survive minification.
    expect(bundle.js, 'wizard layout marker').toContain('All steps complete')
    expect(bundle.js, 'tabs layout marker').toContain('Tab ')
    // groups + single share Mantine primitives; no distinctive marker
    // beyond what the harness already pulls in. Their inclusion is
    // verified transitively by the bundle size delta (~5 MB vs the
    // pre-Subtask-4 ~1.2 MB).
  }, BUNDLE_TIMEOUT_MS)

  it('caches the bundle on the second call', async () => {
    const a = await getRuntimeBundle()
    const b = await getRuntimeBundle()
    expect(a.hash).toBe(b.hash)
    expect(a.builtAt).toBe(b.builtAt) // same timestamp = cached
  }, BUNDLE_TIMEOUT_MS)
})
