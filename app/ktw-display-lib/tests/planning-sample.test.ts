// SPDX-License-Identifier: Apache-2.0
// A visual check of the planning marks: a release card (slots), a lane header
// (slots, a select and a button) and three increment rows (a compact table),
// from sample data shaped like the planification data types. Set
// F035_SAMPLE_OUT to a file path to save the static markup — Mantine's CSS
// inlined — as a page to screenshot (light, and `-dark` beside it).

import { describe, expect, test } from 'bun:test'
import { readFileSync, writeFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'
import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import {
  dataType, deliveryLanes, incrementRows, releaseCard, releaseRows, streamLane,
  timelineSchema,
} from './fixtures/delivery-sample.ts'

wireFieldRenderers()

function sample(scheme: 'light' | 'dark'): string {
  const node = {
    kind: 'vbox', gap: 'md', children: [
      // The lane header and the release card are framed by their host; here
      // a card each, like the mock-up's lane head and `.rel`.
      { kind: 'card', children: [{ kind: 'view', schema: dataType('stream-lane'), variant: 'lane-header', data: streamLane }] },
      { kind: 'card', children: [{ kind: 'view', schema: dataType('release-card'), variant: 'card', data: releaseCard }] },
      // The release level: its header (the stepper, the labelled gates, its
      // actions) and the rows that ship in it, as the mock-up draws them.
      { kind: 'card', children: [
        { kind: 'view', schema: dataType('release-rows'), variant: 'header', data: releaseRows },
        { kind: 'table', schema: dataType('increment-row'), data: incrementRows },
      ] },
    ],
  }
  const body = renderToStaticMarkup(createElement(MantineProvider, { forceColorScheme: scheme },
    createElement(UIPageView, { node: node as never, data: { selection: { increment: 'IN-036' } }, onAction: () => {} })))
  return body
}

describe('the F-035 sample', () => {
  test('renders the three pieces', () => {
    const out = sample('light')
    expect(out).toContain('data-ktw-view="slots"')
    expect(out).toContain('data-ktw-table="compact"')
    const file = process.env.F035_SAMPLE_OUT
    if (!file) return
    const req = createRequire(import.meta.url)
    const css = readFileSync(req.resolve('@mantine/core/styles.css'), 'utf8')
    for (const scheme of ['light', 'dark'] as const) {
      const markup = sample(scheme)
      const page = `<!doctype html><html data-mantine-color-scheme="${scheme}"><head><meta charset="utf-8"><title>F-035 sample</title>
<style>${css}</style><style>body{background:var(--mantine-color-body);padding:24px;max-width:1100px;margin:0 auto}
</style></head><body>${markup}</body></html>`
      writeFileSync(scheme === 'light' ? file : file.replace(/\.html$/, '-dark.html'), page)
    }
  })
})

// A visual check of the timeline lanes: the mock-up's timeline — four streams,
// their releases as cards by date, their backlogs, today fixed at 18/10,
// RL-033 selected. Set F034_SAMPLE_OUT to save it (light, and `-dark`).
function timelineSample(scheme: 'light' | 'dark'): string {
  const node = { kind: 'timeline-lanes', schema: timelineSchema('2026-10-18'), data: deliveryLanes }
  return renderToStaticMarkup(createElement(MantineProvider, { forceColorScheme: scheme },
    createElement(UIPageView, { node: node as never, data: { selection: { release: 'RL-033' } }, onAction: () => {} })))
}

describe('the F-034 sample', () => {
  test('renders the four streams on one axis', () => {
    const out = timelineSample('light')
    expect((out.match(/data-part="lane"/g) ?? []).length).toBe(4)
    expect((out.match(/data-part="item"/g) ?? []).length).toBe(5)
    expect(out).toContain('Backlog 2: IN-001p, IN-002p')
    expect(out).toMatch(/aria-pressed="true"[^>]*data-item-id="RL-033"/)
    const file = process.env.F034_SAMPLE_OUT
    if (!file) return
    const req = createRequire(import.meta.url)
    const css = readFileSync(req.resolve('@mantine/core/styles.css'), 'utf8')
    for (const scheme of ['light', 'dark'] as const) {
      const page = `<!doctype html><html data-mantine-color-scheme="${scheme}"><head><meta charset="utf-8"><title>F-034 sample</title>
<style>${css}</style><style>body{background:var(--mantine-color-body);padding:24px;max-width:1240px;margin:0 auto}
</style></head><body>${timelineSample(scheme)}</body></html>`
      writeFileSync(scheme === 'light' ? file : file.replace(/\.html$/, '-dark.html'), page)
    }
  })
})
