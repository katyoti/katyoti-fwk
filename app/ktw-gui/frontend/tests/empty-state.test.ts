// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { isEmptyAnswer, isTaskURI } from '../src/sections/workspace/emptyState'

// A Home panel - or a list it shows - with nothing to show invites
// the next action its pack declares: the sentence, and a button that opens a
// task's run form or runs a method.
describe('a source answer with nothing to show', () => {
  test('is no answer, an empty list, or data whose lists are all empty', () => {
    expect(isEmptyAnswer(null)).toBe(true)
    expect(isEmptyAnswer([])).toBe(true)
    expect(isEmptyAnswer({})).toBe(true)
    expect(isEmptyAnswer({ items: [], groups: { open: [], done: [] }, total: 0 })).toBe(true)
    expect(isEmptyAnswer({ kind: 'page', data: { rows: [] } })).toBe(true)
  })
  test('is not a list with an item, nor data without any list', () => {
    expect(isEmptyAnswer({ items: [], done: [{ id: 'a' }] })).toBe(false)
    expect(isEmptyAnswer({ kind: 'page', data: { rows: [{ id: 'a' }] } })).toBe(false)
    expect(isEmptyAnswer({ title: 'Overview', count: 3 })).toBe(false)
  })
})

describe('its action', () => {
  test('opens a task\'s run form for a task, runs a method otherwise', () => {
    expect(isTaskURI('ktw-acme:tasks/new-study')).toBe(true)
    expect(isTaskURI('ktw-acme:/task/new-study')).toBe(true)
    expect(isTaskURI('ktw-agents:views/person-queue-page')).toBe(false)
  })
})

describe('the GUI draws it', () => {
  const src = (p: string) => readFileSync(new URL(p, import.meta.url), 'utf8')
  test('a source panel shows the declared invitation instead of an empty page', () => {
    const panel = src('../src/sections/workspace/RenderDefView.tsx')
    expect(panel).toContain('if (empty && isEmptyAnswer(result)) return <>{empty}</>')
  })
  test('a Home panel passes its declared empty state, in the person\'s language', () => {
    const home = src('../src/sections/home/GlobalPanels.tsx')
    expect(home).toContain('<EmptyInvite empty={panel.empty}')
    expect(home).toContain("ct(panelURI, 'empty.text', panel.empty.text)")
  })
  test('its button opens the task or runs the method', () => {
    const invite = src('../src/sections/workspace/EmptyInvite.tsx')
    expect(invite).toContain('if (isTaskURI(uri)) {')
    expect(invite).toContain('WorkspaceRunAction(universeName, uri, JSON.stringify(empty.input ?? {}))')
  })
})
