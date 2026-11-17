// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { createElement, type ReactElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { guiHostServices, loadPart, SECTION_LOADS_AT_ONCE } from '../src/components/compositions/hostServices'
import { SectionFailed } from '@ktw/display-lib/ui-builders/page/nodes/section-load.tsx'
import { LoadedPart, partOnFail, partOnLoad, partShown, runPart, type Got, type HostLoad } from '../src/hooks/useHostLoad'

// A view's loaded part (a source panel, the request
// list, the Global panels, the task list, the objects tree) goes through the
// host's one load - one gate per universe shared with the page sections, a
// load asked twice at once run once, the cold-start retry in one place, the
// error card with « Réessayer », a re-read keeping its content.

const tick = () => new Promise((r) => setTimeout(r, 5))

describe("a view's part goes through the universe's one load", () => {
  test(`one gate per universe bounds loadSection and the parts together (at most ${SECTION_LOADS_AT_ONCE} at once)`, async () => {
    let open!: () => void
    const gate = new Promise<void>((go) => { open = go })
    let running = 0
    let most = 0
    const busy = async <T>(v: T): Promise<T> => { running++; most = Math.max(most, running); await gate; running--; return v }
    ;(globalThis as unknown as { window: unknown }).window = {
      go: { main: { App: {
        WorkspaceRunActionAs: async () => busy({ value: 'null' }),
        WorkspaceCancelCall: async () => {},
      } } },
    }
    // Two hosts of the universe (two open pages) and three view parts.
    const a = guiHostServices('gate-u')
    const b = guiHostServices('gate-u')
    const all = [
      a.loadSection!({ method: 's:x/1' }), a.loadSection!({ method: 's:x/2' }), b.loadSection!({ method: 's:x/3' }),
      loadPart('gate-u', 'p1', () => busy(1)), loadPart('gate-u', 'p2', () => busy(2)), loadPart('gate-u', 'p3', () => busy(3)),
    ]
    await tick()
    expect(running).toBe(SECTION_LOADS_AT_ONCE)
    // Another universe has its own gate.
    let other = false
    const elsewhere = loadPart('other-u', 'p1', async () => { other = true; return 0 })
    await tick()
    expect(other).toBe(true)
    open()
    await Promise.all([...all, elsewhere])
    expect(most).toBe(SECTION_LOADS_AT_ONCE)
  })

  test('a load asked twice at once (the Home tab and the task gallery) runs once', async () => {
    let calls = 0
    const tasks = async () => { calls++; await tick(); return ['t1'] }
    const ctl = new AbortController()
    const [home, gallery] = await Promise.all([
      runPart({ universe: 'share-u', key: 'tasks', load: tasks }, ctl.signal),
      runPart({ universe: 'share-u', key: 'tasks', load: tasks }, ctl.signal),
    ])
    expect(calls).toBe(1)
    expect(home).toEqual(['t1'])
    expect(gallery).toEqual(['t1'])
  })

  test('a cold-start empty answer is asked again, then the filled one taken', async () => {
    const answers = [[], [], ['t1']]
    let calls = 0
    const got = await runPart({
      universe: 'cold-u', key: 'tasks', load: async () => answers[calls++], isEmpty: (v: string[]) => v.length === 0,
    }, new AbortController().signal, [1, 1, 1])
    expect(got).toEqual(['t1'])
    expect(calls).toBe(3)
  })

  test('a failure is retried while the core starts, then thrown once the backoff is spent', async () => {
    let calls = 0
    const failing = { universe: 'cold-u', key: 'tree', load: async () => { calls++; throw new Error('core unreachable') }, retryErrors: true }
    await expect(runPart(failing, new AbortController().signal, [1, 1])).rejects.toThrow('core unreachable')
    expect(calls).toBe(3)
    // Without retryErrors a failure is the error at once.
    calls = 0
    await expect(runPart({ ...failing, retryErrors: false }, new AbortController().signal, [1, 1])).rejects.toThrow('core unreachable')
    expect(calls).toBe(1)
  })

  test('a part given up stops: no further try', async () => {
    let calls = 0
    const ctl = new AbortController()
    const p = runPart({ universe: 'stop-u', key: 'k', load: async () => { calls++; return [] as string[] }, isEmpty: (v: string[]) => v.length === 0 }, ctl.signal, [50, 50])
    await tick()
    ctl.abort()
    await expect(p).rejects.toHaveProperty('name', 'AbortError')
    expect(calls).toBe(1)
  })
})

describe('a part as a view draws it', () => {
  const at = 'u|source|a'
  const shown: Got<string> = { at, data: 'page', done: true }

  test('a re-read keeps the shown content until its answer', () => {
    const during = partOnLoad(shown, at)
    expect(partShown(during, at, true)).toEqual({ data: 'page', loading: false, error: null })
  })

  test('another part shows the placeholder, never the last one', () => {
    const other = partOnLoad(shown, 'u|source|b')
    expect(partShown(other, 'u|source|b', true)).toEqual({ data: undefined, loading: true, error: null })
  })

  test('a failure is the error card; « Réessayer » shows the placeholder and loads again', () => {
    const failed = partOnFail(shown, at, 'boom')
    expect(partShown(failed, at, true).error).toBe('boom')
    const retry = () => {}
    const part: HostLoad<string> = { ...partShown(failed, at, true), retry, reread: () => {} }
    // The display library's error card (its data-state=failed and its
    // data-section-retry button), wired to the part's retry. Drawn by element:
    // under bun the library resolves its own React copy (vite dedupes it).
    const card = LoadedPart<string>({ id: 'source:x', part, children: (d: string) => d }) as ReactElement<{ id: string; message: string; onRetry: () => void }>
    expect(card.type).toBe(SectionFailed)
    expect(card.props).toMatchObject({ id: 'source:x', message: 'boom' })
    expect(card.props.onRetry).toBe(retry)
    // After the failure a load starts afresh: the placeholder, not the old page.
    expect(partShown(partOnLoad(failed, at), at, true)).toEqual({ data: undefined, loading: true, error: null })
  })

  test('the content once loaded, the placeholder before', () => {
    const loaded: HostLoad<string> = { ...partShown(shown, at, true), retry: () => {}, reread: () => {} }
    const draw = (part: HostLoad<string>) => renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(LoadedPart<string>, { id: 'source:x', part, children: (d: string) => `<${d}>` })))
    expect(draw(loaded)).toContain('&lt;page&gt;')
    expect(draw({ ...loaded, data: undefined, loading: true })).toContain('data-state="loading"')
  })
})
