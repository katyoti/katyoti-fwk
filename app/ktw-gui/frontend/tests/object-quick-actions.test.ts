// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'

// The actions a type or a capability declares quick show
// as small buttons on the object's first page (its Information), run as the
// action panel runs them, and stay in the action panel.
const src = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')

describe('quick actions on an object\'s first page', () => {
  test('the Information page draws the declared quick actions as buttons, first', () => {
    const info = src('../src/sections/workspace/ObjectCorePanels.tsx')
    expect(info).toContain('const quick = (detail.quickActions ?? []).filter((a) => a.quick)')
    expect(info).toContain('<ActionsPanel actions={quick} universeName={universeName} nodeId={objectRef(detail.id, detail.project)} layout="buttons" />')
  })
  test('a button runs the action as the panel does (confirm, form, chat, result)', () => {
    const panel = src('../src/sections/workspace/ActionsPanel.tsx')
    expect(panel).toContain("layout === 'buttons'")
    expect(panel).toContain('onClick={() => void handleClick({ id: bindingId(a), title: a.label })}')
  })
  test('the GUI keeps the mark the pane sends', () => {
    expect(src('../wailsjs/go/models.ts')).toContain('this.quick = source["quick"];')
  })
})
