// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { type ReactElement } from 'react'

import { SectionFailed } from '@ktw/display-lib/ui-builders/page/nodes/section-load.tsx'
import { RESTART_BACKOFF_MS, runPart } from '../src/hooks/useHostLoad'
import { GlobalPanelsFailed, globalPanelsPart } from '../src/sections/home/GlobalPanels'

// The Global strip's declared panels (« À faire », planification…)
// survive a hub restart: a failed global-panels load is asked again over the
// restart's window, and a load still failing is the error card with
// « Réessayer » - never a strip silently left with no panels.

const PANELS = { panels: [{ id: 'panel:person-queue', name: 'À faire', order: 1, kind: 'source', source: 's:q' }] }

/** A host whose global-panels method fails `fails` times, then answers. */
function hostFailing(fails: number): { calls: () => number } {
  let n = 0
  ;(globalThis as unknown as { window: unknown }).window = {
    go: { main: { App: {
      WorkspaceRunActionAs: async () => (n++ < fails ? { error: 'hub unreachable' } : { value: JSON.stringify(PANELS) }),
      WorkspaceCancelCall: async () => {},
    } } },
  }
  return { calls: () => n }
}

describe('the Global strip panels over a hub restart', () => {
  test('a failed load is asked again, then the panels drawn', async () => {
    const host = hostFailing(2)
    const part = { ...globalPanelsPart('restart-u'), key: 'global-panels-a' }
    const got = await runPart(part, new AbortController().signal, [1, 1, 1])
    expect(got.map((p) => p.id)).toEqual(['panel:person-queue'])
    expect(host.calls()).toBe(3)
  })

  test('its retries outlast a hub restart, not only a core starting (~30 s)', () => {
    const part = globalPanelsPart('restart-u')
    expect(part.retryErrors).toBe(true)
    expect(part.backoff).toBe(RESTART_BACKOFF_MS)
    expect(RESTART_BACKOFF_MS.reduce((a, b) => a + b, 0)).toBeGreaterThanOrEqual(30000)
  })

  test('a load still failing is the error card with « Réessayer », never no panels', async () => {
    hostFailing(99)
    const part = { ...globalPanelsPart('restart-u'), key: 'global-panels-b' }
    await expect(runPart(part, new AbortController().signal, [1, 1])).rejects.toThrow('hub unreachable')
    const retry = () => {}
    const card = GlobalPanelsFailed({ part: { error: 'hub unreachable', retry } }) as ReactElement<{ id: string; message: string; onRetry: () => void }>
    expect(card.type).toBe(SectionFailed)
    expect(card.props).toMatchObject({ id: 'global-panels', message: 'hub unreachable' })
    expect(card.props.onRetry).toBe(retry)
    // A load that answers draws no card.
    expect(GlobalPanelsFailed({ part: { error: null, retry } })).toBeNull()
  })
})
