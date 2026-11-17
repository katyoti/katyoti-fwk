// SPDX-License-Identifier: Apache-2.0
// A live progress list - one row per stage with a bar, a count
// and a success or failure tone - updated as the run reports through its
// progress method, never by reading the panel again.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { applyDataBinding } from '../src/ui-builders/page/nodes/data-binding.ts'
import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { INTERACTIVE_KINDS } from '../src/ui-builders/page/ui-page.types.ts'
import { pollProgress, progressOver, progressRow } from '../src/ui-builders/page/ui-page.interactive.ts'

const page = (node: unknown, data?: unknown) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(UIPageView, { node: node as never, data })))

const stages = [
  { id: 'sources', label: 'Lire les sources', done: 3, total: 3, state: 'done', ms: 4200 },
  { id: 'kb', label: 'Base de connaissances', done: 12, total: 40, state: 'running' },
  { id: '05', label: '05 Architecture applicative', state: 'failed', message: 'LeanIX ne répond pas' },
  { id: '06', label: '06 Architecture technique', total: 1 },
]

describe('the live progress list', () => {
  test('is a declared interactive kind of the page grammar', () => {
    expect(INTERACTIVE_KINDS).toContain('progress-list')
    expect((UIPageNodeJSONSchema as any).definitions.ProgressList.properties.poll.required).toEqual(['uri'])
  })

  test('each stage is a bar, a count and a tone: success, running, failure, waiting', () => {
    expect(progressRow(stages[0] as never)).toEqual({ state: 'done', percent: 100, count: '3/3', tone: 'teal' })
    expect(progressRow(stages[1] as never)).toEqual({ state: 'running', percent: 30, count: '12/40', tone: 'blue' })
    expect(progressRow(stages[2] as never).tone).toBe('red')
    expect(progressRow(stages[3] as never)).toEqual({ state: 'waiting', percent: 0, count: '0/1', tone: 'gray' })
    expect(progressRow({ id: 'x', label: 'x', done: 2, total: 2 })).toEqual({ state: 'done', percent: 100, count: '2/2', tone: 'teal' })
  })

  test('the rows are drawn from the stages bound, one per stage, each with its state and what it says', () => {
    const node = applyDataBinding({ kind: 'progress-list', id: 'run', dataField: '$.run.stages' } as never, { run: { stages } })
    const html = page({ kind: 'vbox', children: [node] })
    expect(html).toContain('data-element="progress-list:run"')
    expect([...html.matchAll(/data-guide="progress.stage" data-guide-name="([^"]+)" data-state="([a-z]+)"/g)].map((m) => `${m[1]}:${m[2]}`))
      .toEqual(['sources:done', 'kb:running', '05:failed', '06:waiting'])
    expect(html).toContain('12/40')
    expect(html).toContain('LeanIX ne répond pas')
    expect(html).toContain('4.2 s')
  })

  test('as the run reports, the progress method gives the next stages; it stops once the run is over', async () => {
    const asked: unknown[] = []
    const answers = [
      { stages: [{ id: 'kb', label: 'Base', done: 20, total: 40, state: 'running' }] },
      { done: true, stages: [{ id: 'kb', label: 'Base', done: 40, total: 40, state: 'done' }] },
    ]
    const call = async (uri: string, input?: unknown) => { asked.push({ uri, input }); return answers[asked.length - 1] }
    const poll = { uri: 'ktw-example:studies/run-progress', args: { run: 'r1' } }
    const first = await pollProgress(call, poll, [])
    expect(first.over).toBe(false)
    expect(first.stages[0].done).toBe(20)
    const second = await pollProgress(call, poll, first.stages)
    expect(second.over).toBe(true)
    expect(asked).toEqual([{ uri: poll.uri, input: { run: 'r1' } }, { uri: poll.uri, input: { run: 'r1' } }])
    expect(progressOver(undefined, [{ id: 'a', label: 'a', state: 'done' }, { id: 'b', label: 'b', state: 'failed' }])).toBe(true)
    expect(progressOver(undefined, [{ id: 'a', label: 'a', state: 'running' }])).toBe(false)
    // The run says it is over even with a stage left running (stopped): polling stops.
    expect(progressOver({ done: true }, [{ id: 'a', label: 'a', state: 'running' }])).toBe(true)
  })

  test('a progress call that fails keeps the rows and says why; the answer can hold the stages elsewhere', async () => {
    const kept = [{ id: 'kb', label: 'Base', state: 'running' as const }]
    const failed = await pollProgress(async () => { throw new Error('hub unreachable') }, { uri: 'x:y/z' }, kept)
    expect(failed).toEqual({ stages: kept, over: false, error: 'hub unreachable' })
    const nested = await pollProgress(async () => ({ progress: { steps: [{ id: 's', label: 's', state: 'done' }] } }), { uri: 'x:y/z', field: 'progress.steps' }, [])
    expect(nested.stages[0].id).toBe('s')
    expect(nested.over).toBe(true)
  })
})
