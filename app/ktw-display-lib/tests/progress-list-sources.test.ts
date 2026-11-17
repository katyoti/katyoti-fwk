// SPDX-License-Identifier: Apache-2.0
// A run's progress list shows one line per source its reading
// syncs - its count n/m, then done, or failed with why - folded from the
// source.sync events each source's sync raises, followed
// through the host's followEvents.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { applySourceSync, progressRow, sourceLines, type UIProgressFollow, type UIProgressStage } from '../src/ui-builders/page/ui-page.interactive.ts'

const page = (node: unknown) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(UIPageView, { node: node as never })))

const follow: UIProgressFollow = {
  event: 'source.sync',
  sources: ['acme-da.SRC-leanix', 'acme-da.SRC-confluence'],
  labels: { 'acme-da.SRC-leanix': 'LeanIX', 'acme-da.SRC-confluence': 'Confluence' },
}

// A fake per-source event stream: LeanIX reads its three items, Confluence
// fails after one of four - interleaved as two syncs run side by side.
const stream = [
  { source: 'acme-da.SRC-leanix', phase: 'started' },
  { source: 'acme-da.SRC-confluence', phase: 'started' },
  { source: 'acme-da.SRC-leanix', phase: 'items', read: 1, total: 3 },
  { source: 'acme-da.SRC-confluence', phase: 'items', read: 1, total: 4 },
  { source: 'acme-da.SRC-leanix', phase: 'items', read: 3, total: 3 },
  { source: 'acme-da.SRC-confluence', phase: 'failed', read: 1, total: 4, error: 'Confluence injoignable : délai dépassé' },
  { source: 'acme-da.SRC-leanix', phase: 'ended', read: 3, total: 3 },
]

const fold = (events: unknown[], from: UIProgressStage[], f?: UIProgressFollow) => events.reduce<UIProgressStage[]>((ls, e) => applySourceSync(ls, e, f), from)
const said = (ls: UIProgressStage[]) => ls.map((l) => `${l.label}:${progressRow(l).state}:${progressRow(l).count}${l.message ? `:${l.message}` : ''}`)

describe('a run shows one progress line per source', () => {
  test('the sources the run reads each wait on a line of their own before any event', () => {
    expect(said(sourceLines(follow))).toEqual(['LeanIX:waiting:', 'Confluence:waiting:'])
    expect(sourceLines(undefined)).toEqual([])
  })

  test('a source whose reading already ended starts its line where it stands, a late count does not reopen it', () => {
    const ended: UIProgressFollow = { ...follow, states: { 'acme-da.SRC-leanix': { state: 'done', done: 3, total: 3 }, 'acme-da.SRC-confluence': { state: 'failed', done: 0, total: 1, message: 'inconnu' } } }
    expect(said(sourceLines(ended))).toEqual(['LeanIX:done:3/3', 'Confluence:failed:0/1:inconnu'])
    expect(fold([{ source: 'acme-da.SRC-leanix', phase: 'items', read: 1, total: 3 }], sourceLines(ended), ended)).toEqual(sourceLines(ended))
  })

  test('each source line follows its events: its count while it reads, then done or failed with why', () => {
    const lines = sourceLines(follow)
    expect(said(fold(stream.slice(0, 4), lines, follow))).toEqual(['LeanIX:running:1/3', 'Confluence:running:1/4'])
    expect(said(fold(stream, lines, follow))).toEqual(['LeanIX:done:3/3', 'Confluence:failed:1/4:Confluence injoignable : délai dépassé'])
  })

  test('a failure without its why still says it failed; a late count does not reopen an ended line; a new start reads again', () => {
    const failed = fold([{ source: 'a', phase: 'started' }, { source: 'a', phase: 'failed' }], [])
    expect(said(failed)).toEqual(['a:failed::La lecture a échoué.'])
    expect(fold([{ source: 'a', phase: 'items', read: 2, total: 3 }], failed)).toEqual(failed)
    expect(said(fold([{ source: 'a', phase: 'started', total: 2 }], failed))).toEqual(['a:running:0/2'])
  })

  test('without sources named, a line shows for each source as its sync starts; with them, another source is ignored', () => {
    expect(said(fold(stream.slice(0, 2), []))).toEqual(['acme-da.SRC-leanix:running:', 'acme-da.SRC-confluence:running:'])
    const other = { source: 'acme-da.SRC-dynatrace', phase: 'started' }
    expect(fold([other], sourceLines(follow), follow)).toEqual(sourceLines(follow))
    // Not an event of a source, or a phase not known: nothing changes.
    expect(fold([null, 'x', { phase: 'started' }, { source: 'a', phase: 'paused' }], [])).toEqual([])
  })

  test('the lines are drawn after the stages, by element ids: one progress.stage per source with its state', () => {
    const html = page({ kind: 'progress-list', id: 'study-run', stages: [{ id: 'sources', label: 'Lire les sources', state: 'running' }], follow })
    expect(html).toContain('data-element="progress-list:study-run"')
    expect([...html.matchAll(/data-guide="progress.stage" data-guide-name="([^"]+)" data-state="([a-z]+)"/g)].map((m) => `${m[1]}:${m[2]}`))
      .toEqual(['sources:running', 'acme-da.SRC-leanix:waiting', 'acme-da.SRC-confluence:waiting'])
    expect(html).toContain('LeanIX')
  })

  test('the page grammar declares follow on a progress list', () => {
    const f = (UIPageNodeJSONSchema as any).definitions.ProgressList.properties.follow
    expect(f.required).toEqual(['event'])
    expect(f.properties.event.const).toBe('source.sync')
  })
})
