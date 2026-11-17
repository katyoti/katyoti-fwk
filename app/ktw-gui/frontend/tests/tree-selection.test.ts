// SPDX-License-Identifier: Apache-2.0
import { beforeEach, describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { openObjectTab, closeObjectTab } from '../src/sections/workspace/openObjectTab'
import { useTabsStore } from '../src/store/tabsSlice'
import { openObjectId } from '../src/sections/workspace/treeSelection'

// The tree marks the node of the object open in the pane: opening another
// object moves the mark, going back restores it, a closed tab marks nothing.
const U = 'u'
const marked = () => openObjectId(useTabsStore.getState().getForUniverse(U))

beforeEach(() => {
  useTabsStore.setState({ byUniverse: new Map([[U, {
    openTabs: [{ id: 'home', kind: 'home', label: 'Global', closeable: false, openedAt: 0, lastActivityAt: 0 }] as never,
    activeTabId: 'home',
  }]]) })
})

describe('the node marked in the tree', () => {
  test('is the object opened, and moves with the next one', () => {
    expect(marked()).toBeUndefined()
    openObjectTab(U, { id: 'm.RQ-15', title: 'RQ-15' })
    expect(marked()).toBe('m.RQ-15')
    openObjectTab(U, { id: 'gui', title: 'GUI' })
    expect(marked()).toBe('gui')
  })
  test('follows the way back, and is none once the tab is closed', () => {
    openObjectTab(U, { id: 'gui', title: 'GUI' })
    openObjectTab(U, { id: 'm.RQ-15', title: 'RQ-15' })
    closeObjectTab(U)
    expect(marked()).toBe('gui')
    closeObjectTab(U)
    expect(marked()).toBeUndefined()
  })
  test('the tree draws it bold on the primary light background', () => {
    const src = readFileSync(new URL('../src/sections/workspace/WorkspaceObjectsTree.tsx', import.meta.url), 'utf8')
    expect(src).toContain('const selected = node.value === openId')
    expect(src).toContain("fw={selected ? 700 : undefined}")
    expect(src).toContain("var(--mantine-primary-color-light)")
    expect(src).toContain('aria-selected={selected}')
  })
})
