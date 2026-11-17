// SPDX-License-Identifier: Apache-2.0
import { beforeEach, describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { openOf } from '../src/hooks/toolOpen'
import { openRunResult } from '../src/hooks/runOpen'
import { useAppStore } from '../src/store'
import { useTabsStore } from '../src/store/tabsSlice'
import { objectTabId } from '../src/elements'

// A create task started from its entry form ends on the object
// its result names - on the panel after `#` - and the form gives way to it,
// as a page form's submit navigates. The run's open (run.open, a tool's
// {ok, open}) reaches openRunResult whatever place opened the form.
const U = 'u'
const form = { name: 'ktw-acme:tasks/new-study', description: '', inputSchemaURI: 'ktw-acme:/data-type/new-study-input', targetId: 'dossier-1' }

beforeEach(() => {
  useTabsStore.setState({ byUniverse: new Map([[U, { openTabs: [{ id: 'home', kind: 'home', label: 'Global', closeable: false, openedAt: 0, lastActivityAt: 0 }] as never, activeTabId: 'home' }]]) })
  useAppStore.getState().openRunTask(form)
})

describe('a run opening what it made closes its entry form', () => {
  test('an open with id#panel opens the object on that panel and closes the form', () => {
    expect(useAppStore.getState().pendingTask).not.toBeNull()
    const target = openOf(JSON.stringify({ ok: true, open: 'etude-x#panel:etude' }))!
    openRunResult(U, target)
    expect(useAppStore.getState().pendingTask).toBeNull()
    const strip = useTabsStore.getState().getForUniverse(U)
    expect(strip.activeTabId).toBe('object')
    const shown = strip.openTabs.find((t) => t.id === 'object')?.payload as { objectId: string; activeKind?: string }
    expect(shown.objectId).toBe('etude-x')
    expect(shown.activeKind).toBe(objectTabId('panel:etude'))
  })

  test('every open of a run goes through it: a workflow answer and a tool answer', () => {
    const src = readFileSync(new URL('../src/hooks/useAgentEvents.ts', import.meta.url), 'utf8')
    const runOpen = src.slice(src.indexOf("EventsOn('run.open'"), src.indexOf("EventsOn('session.ended'"))
    expect(runOpen).toContain('openRunResult(uni, target)')
    expect(src.match(/openRunResult\(uni, target\)/g)?.length).toBe(2)
  })
})
