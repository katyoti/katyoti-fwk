// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'

import { panelOfAnswer } from '../src/sections/panels/panelAnswer'

// An object or global panel whose source answers a display panel
// (render.Answer{panel}) draws it - the GUI page host mounts
// DisplayPanel in place, which draws the panel's skeleton and its pushed
// sections from the open event.
describe("a source's panel answer", () => {
  test('names its panel', () => {
    expect(panelOfAnswer({ panel: 'dp-1' })).toBe('dp-1')
    expect(panelOfAnswer({ panel: 'dp-1', data: { n: 1 }, parts: {} })).toBe('dp-1')
  })
  test('is no panel when it is a tree, a Def or data with a panel field', () => {
    expect(panelOfAnswer({ panel: '' })).toBeUndefined()
    expect(panelOfAnswer({ panel: 'dp-1', tree: { type: 'page' } })).toBeUndefined()
    expect(panelOfAnswer({ kind: 'page', panel: 'dp-1' })).toBeUndefined()
    expect(panelOfAnswer({ panel: 'dp-1', items: [] })).toBeUndefined()
    expect(panelOfAnswer([{ panel: 'dp-1' }])).toBeUndefined()
    expect(panelOfAnswer(null)).toBeUndefined()
  })
})

describe('the page host draws it', () => {
  const src = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
  test('SourcePanel mounts DisplayPanel in place for a panel answer', () => {
    const panel = src('../src/sections/workspace/RenderDefView.tsx')
    expect(panel).not.toContain('useDisplaySwitch')
    expect(panel).toContain('const panelId = panelOfAnswer(result)')
    expect(panel).toContain('if (panelId) return <DisplayPanel universeName={universeName} panelId={panelId} variant="tab" />')
  })
})
