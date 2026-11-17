// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { afterExportPanel, exportPanelLife, exportRunOutcome } from '../src/components/document/exportRun'

// The document export runs the host's conversion (a long
// ktw-docs method) through the action surface: its run's panel drawn where
// the export was asked, the file read and saved once the panel completes ok.

describe('what an export run shows', () => {
  test('a long conversion: its run\'s panel, the export kept to read when it ends', () => {
    expect(exportRunOutcome({ exportId: 'export-1', taskId: 'run-1', panel: 'dp-1' })).toEqual({ panel: 'dp-1', exportId: 'export-1' })
  })

  test('a conversion that answers at once: the document to save', () => {
    const document = { mediaType: 'application/pdf', title: 'note.pdf', content: 'JVBERg==', encoding: 'base64' }
    expect(exportRunOutcome({ document })).toEqual({ document })
  })

  test('an error, never a panel; an empty answer is an error', () => {
    expect(exportRunOutcome({ error: 'refused', panel: 'dp-1', exportId: 'export-1' })).toEqual({ error: 'refused' })
    expect('error' in exportRunOutcome({})).toBe(true)
  })
})

describe('once the export\'s panel ends', () => {
  test('its file is read when it completed ok', () => {
    expect(afterExportPanel('ok')).toBe('read')
  })

  test('a failed or cancelled export is discarded', () => {
    expect(afterExportPanel('failed')).toBe('discard')
    expect(afterExportPanel('cancelled')).toBe('discard')
  })
})

// An export whose panel is closed, replaced by another export or
// unmounted before its run ended is discarded - its job and temp folder never
// stay in the host until the GUI exits; one already read is left alone.
describe('an export panel that goes away', () => {
  const setup = () => {
    const discarded: string[] = []
    const ticks: (() => void)[] = []
    const life = exportPanelLife('export-7', (id) => discarded.push(id), (f) => ticks.push(f))
    const tick = () => { while (ticks.length) ticks.shift()!() }
    life.enter()
    return { life, discarded, tick }
  }

  test('closed or replaced before its run ended: the export is discarded', () => {
    const { life, discarded, tick } = setup()
    life.leave()
    tick()
    expect(discarded).toEqual(['export-7'])
  })

  test('mounted again at once (StrictMode): the export is kept', () => {
    const { life, discarded, tick } = setup()
    life.leave()
    life.enter()
    tick()
    expect(discarded).toEqual([])
  })

  test('after a completed read: nothing is discarded', () => {
    const { life, discarded, tick } = setup()
    expect(life.ended('ok')).toBe('read')
    life.leave()
    tick()
    expect(discarded).toEqual([])
  })

  test('after a failed run: discarded once, never again on leave', () => {
    const { life, discarded, tick } = setup()
    expect(life.ended('failed')).toBe('discard')
    life.leave()
    tick()
    expect(discarded).toEqual(['export-7'])
  })

  test('left first, a late end neither reads nor discards again', () => {
    const { life, discarded, tick } = setup()
    life.leave()
    tick()
    expect(life.ended('ok')).toBe('settled')
    expect(discarded).toEqual(['export-7'])
  })
})

describe('the document view', () => {
  const src = readFileSync(join(import.meta.dir, '../src/components/document/DocumentView.tsx'), 'utf8')

  test('routes the export through its run and draws the run\'s panel in place - no direct call that waits', () => {
    expect(src).not.toContain('useDisplaySwitch')
    expect(src).not.toMatch(/\bDocumentExport\(/)
    expect(src).toContain('DocumentExportRun')
    expect(src).toContain('<DisplayPanel')
    expect(src).toContain('DocumentExportRead')
    expect(src).toContain('DocumentExportDiscard')
  })

  test('the export panel discards its export when it goes away before its run ended', () => {
    expect(src).toContain('exportPanelLife(run.exportId')
    expect(src).toMatch(/useEffect\(\(\) => \{ life\.current\?\.enter\(\); return \(\) => life\.current\?\.leave\(\) \}, \[\]\)/)
  })
})
