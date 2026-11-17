// SPDX-License-Identifier: Apache-2.0
import { beforeEach, describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { objectRef, projectOfRef } from '../src/sections/workspace/objectRef'
import { buildObjectTree } from '../src/sections/workspace/WorkspaceObjectsTree'
import { openObjectTab } from '../src/sections/workspace/openObjectTab'
import { openObjectId } from '../src/sections/workspace/treeSelection'
import { useTabsStore } from '../src/store/tabsSlice'
import type { main } from '../wailsjs/go/models'

// A universe serving several projects answers each object bare
// with the key of the project holding it; two projects may hold one bare id.
// The GUI opens, saves and follows the links of such an object by its
// qualified id <key>:<module>.<local>; a row naming no project (one project
// served) keeps the bare id - today's calls.
const src = (p: string) => readFileSync(join(import.meta.dir, '../src', p), 'utf-8')
const U = 'u'
const row = (id: string, project?: string) => ({ id, title: id, type: 'feature', module: 'm', depth: 0, hasChildren: false, ...(project ? { project } : {}) }) as main.WorkspaceObject

beforeEach(() => {
  useTabsStore.setState({ byUniverse: new Map([[U, {
    openTabs: [{ id: 'home', kind: 'home', label: 'Global', closeable: false, openedAt: 0, lastActivityAt: 0 }] as never,
    activeTabId: 'home',
  }]]) })
})

describe('the id the GUI sends for an object', () => {
  test('qualified when the row names its project, bare otherwise', () => {
    expect(objectRef('m.F-1', 'crm')).toBe('crm:m.F-1')
    expect(objectRef('m.F-1', undefined)).toBe('m.F-1')
    expect(objectRef('m.F-1', '')).toBe('m.F-1')
    expect(objectRef('m.F-1', '  ')).toBe('m.F-1')
    expect(objectRef('gui', 'crm')).toBe('crm:gui') // a module's own id
  })

  test('an id written qualified already is kept, never qualified twice', () => {
    expect(objectRef('erp:m.F-1', 'crm')).toBe('erp:m.F-1')
    expect(projectOfRef('crm:m.F-1')).toBe('crm')
    expect(projectOfRef('m.F-1')).toBe('')
    expect(projectOfRef(undefined)).toBe('')
  })

  test('two projects holding one bare id open two objects; a row without a project opens bare', () => {
    const tree = buildObjectTree([row('m.F-1', 'crm'), row('m.F-1', 'erp'), row('m.F-2')])
    expect(tree.map((n) => n.value)).toEqual(['crm:m.F-1', 'erp:m.F-1', 'm.F-2'])
    // A click opens the node by its value: the tab asks the detail for it.
    openObjectTab(U, { id: tree[1].value, title: 'F-1' })
    expect(openObjectId(useTabsStore.getState().getForUniverse(U))).toBe('erp:m.F-1')
    openObjectTab(U, { id: tree[2].value, title: 'F-2' })
    expect(openObjectId(useTabsStore.getState().getForUniverse(U))).toBe('m.F-2')
  })

  test('the pane saves, deletes and draws its panels with the id as sent', () => {
    const tab = src('sections/workspace/ObjectTab.tsx')
    expect(tab).toContain('const sendId = detail && !detail.error && detail.id ? objectRef(detail.id, detail.project) : objectId')
    expect(tab).toContain("WorkspaceObjectUpdate(universeName, sendId, JSON.stringify({ identity: core }), '', revisionOf(sendId))")
    expect(tab).toContain("WorkspaceObjectUpdate(universeName, sendId, '', JSON.stringify(names), revisionOf(sendId))")
    expect(tab).toContain('WorkspaceObjectDelete(universeName, sendId)')
    expect(tab).toMatch(/objectId=\{sendId\}\s+tab=\{activeTab\}/)
    // Opened bare and answered with a project: the revision read is kept under the id saves send.
    expect(tab).toContain('if (ref !== objectId) noteRevision(ref, d.revision)')
    const core = src('sections/workspace/ObjectCorePanels.tsx')
    expect(core).toContain('objectId={objectRef(detail.id, detail.project)}')
    expect(core).toContain("WorkspaceObjectUpdateWithBody(universeName, id, '', draft, revisionOf(id))")
    expect(src('sections/workspace/PanelDrawer.tsx')).toContain('objectId={sendId}')
  })

  test('a link, a child and the parent chip of another project open in that project', () => {
    const core = src('sections/workspace/ObjectCorePanels.tsx')
    expect(core).toContain('openObjectTab(universeName, { id: objectRef(l.targetId, project), title: l.targetTitle })')
    expect(core).toContain('project={projectOfRef(objectId)}')
    expect(core).toContain('WorkspaceObjectLinkAdd(universeName, fromId, kind, objectRef(target.id, target.project))')
    const tab = src('sections/workspace/ObjectTab.tsx')
    // A child is a card whose actions name it in its own project (childCards.ts).
    expect(tab).toContain('childCardsPage(group, project,')
    expect(src('sections/workspace/childCards.ts')).toContain('ref: objectRef(o.id, project)')
    expect(tab).toContain('openObjectTab(universeName, { id: objectRef(c.opens!, detail.project) })')
  })

  test('no call about the open object sends its bare detail id', () => {
    for (const f of ['sections/workspace/ObjectTab.tsx', 'sections/workspace/ObjectCorePanels.tsx', 'sections/workspace/PanelDrawer.tsx']) {
      const s = src(f)
      expect(s).not.toMatch(/WorkspaceObject(Update|UpdateWithBody|Delete|DeleteDetach)\(universeName, detail\.id/)
      expect(s).not.toMatch(/<(ObjectFieldsForm|RenderDefView|SourcePanel)[^>]*objectId=\{detail\.id\}/)
    }
  })
})
