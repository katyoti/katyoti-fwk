// SPDX-License-Identifier: Apache-2.0
import { beforeEach, describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { panelRefreshMatches } from '../src/hooks/usePanelRefresh'
import { bareOfRef, objectRef, rowProject } from '../src/sections/workspace/objectRef'
import { retitledObjectPayload } from '../src/sections/workspace/objectTabTitle'
import { buildObjectTree } from '../src/sections/workspace/WorkspaceObjectsTree'
import { openObjectTab } from '../src/sections/workspace/openObjectTab'
import { openObjectId } from '../src/sections/workspace/treeSelection'
import { useTabsStore } from '../src/store/tabsSlice'
import type { main } from '../wailsjs/go/models'

const src = (p: string) => readFileSync(join(import.meta.dir, '../src', p), 'utf-8')
const U = 'u'
const row = (id: string, project?: string) => ({ id, title: id, type: 'feature', module: 'm', depth: 0, hasChildren: false, ...(project ? { project } : {}) }) as main.WorkspaceObject

beforeEach(() => {
  useTabsStore.setState({ byUniverse: new Map([[U, {
    openTabs: [{ id: 'home', kind: 'home', label: 'Global', closeable: false, openedAt: 0, lastActivityAt: 0 }] as never,
    activeTabId: 'home',
  }]]) })
})

// The pane's remaining calls - comments, quick actions, Verify,
// « + New » under it, the chat's target, a page row's navigate / drawer /
// create / chat - send the qualified id when the object's project is named;
// one key per object whatever id opened it; counts kept per qualified id.
describe('the remaining calls send the id as sent', () => {
  test('comments, quick actions and Verify on the Information page', () => {
    const core = src('sections/workspace/ObjectCorePanels.tsx')
    expect(core).toContain('<ObjectComments universeName={universeName} objectId={objectRef(detail.id, detail.project)} />')
    expect(core).toContain('nodeId={objectRef(detail.id, detail.project)} layout="buttons"')
    expect(core).toContain('verifyObject(universeName, objectRef(detail.id, detail.project))')
  })

  test('Verify, « + New » and the create dialog under the open object', () => {
    const tab = src('sections/workspace/ObjectTab.tsx')
    expect(tab).toContain('verifyObject(universeName, sendId)')
    expect(tab).toContain('<NewMenu universeName={universeName} parentId={sendId}')
    expect(tab).toContain('runCreateTask(universeName, pick.choice, sendId,')
    expect(tab).toMatch(/<ObjectCreateModal[\s\S]*?parentId=\{sendId\}/)
    // A child created under another project's object opens in that project.
    expect(src('sections/workspace/ObjectCreateModal.tsx')).toContain('openObjectTab(universeName, { id: objectRef(r.object.id, projectOfRef(parentId))')
  })

  test('no call about the open object sends its bare detail id - the remaining surfaces', () => {
    for (const f of ['sections/workspace/ObjectTab.tsx', 'sections/workspace/ObjectCorePanels.tsx']) {
      const s = src(f)
      expect(s).not.toMatch(/verifyObject\(universeName, detail\.id\)/)
      expect(s).not.toMatch(/(parentId|nodeId|objectId)=\{detail\.id\}/)
      expect(s).not.toMatch(/runCreateTask\(universeName, [^,]+, detail\.id/)
    }
  })

  test('opened bare and answered with its project, the tab takes the qualified id: one key per object', () => {
    // A page row opened m.F-1 bare; the detail names erp.
    const next = retitledObjectPayload({ objectId: 'm.F-1' }, 'm.F-1', 'F-1', objectRef('m.F-1', 'erp'))
    expect(next).toEqual({ objectId: 'erp:m.F-1', objectTitle: 'F-1' })
    // The tree's node of that object is the same key: opening it adds no step back.
    const tree = buildObjectTree([row('m.F-1', 'crm'), row('m.F-1', 'erp')])
    openObjectTab(U, { id: 'm.F-1' })
    // The pane read the detail: the tab's own payload (its step back kept) takes the qualified id.
    const opened = useTabsStore.getState().getForUniverse(U).openTabs.find((t) => t.id === 'object')?.payload as { objectId: string; objectTitle?: string }
    useTabsStore.getState().updateTabPayload(U, 'object', retitledObjectPayload(opened, 'm.F-1', 'F-1', 'erp:m.F-1')!, 'F-1')
    openObjectTab(U, { id: tree[1].value, title: 'F-1' })
    const strip = useTabsStore.getState().getForUniverse(U)
    expect(openObjectId(strip)).toBe('erp:m.F-1')
    const back = ((strip.openTabs.find((t) => t.id === 'object')?.payload ?? {}) as { back?: unknown[] }).back ?? []
    expect(back.length).toBe(1) // the step back to home only, never one to the same object
    // A bare id answered with no project (one project served) stays as it is.
    expect(retitledObjectPayload({ objectId: 'm.F-2', objectTitle: 'F-2' }, 'm.F-2', 'F-2', objectRef('m.F-2', ''))).toBeNull()
    // The tab moved on meanwhile: nothing changes.
    expect(retitledObjectPayload({ objectId: 'm.F-3' }, 'm.F-1', 'F-1', 'erp:m.F-1')).toBeNull()
    const tab = src('sections/workspace/ObjectTab.tsx')
    expect(tab).toContain("retitledObjectPayload(cur?.payload as ObjectTabPayload | undefined, objectId, d?.title, d.error ? undefined : ref)")
    // The chat's target follows the open object.
    expect(tab).toContain('if (next.objectId !== objectId) app.selectNode(next.objectId)')
  })

  test('panel counts are kept per qualified id; an object event (bare) still re-reads the panel', () => {
    const tab = src('sections/workspace/ObjectTab.tsx')
    expect(tab).toContain('const countObject = detail ? sendId : undefined')
    expect(tab).toContain("object: countObject, module: detail?.module")
    const place = { universe: U, object: 'erp:m.F-1', module: 'm' }
    expect(bareOfRef('erp:m.F-1')).toBe('m.F-1')
    expect(bareOfRef('m.F-1')).toBe('m.F-1')
    expect(panelRefreshMatches({ scope: 'self' }, { universe: U, subject: 's', object: 'm.F-1' }, place)).toBe(true)
    expect(panelRefreshMatches({ scope: 'self' }, { universe: U, subject: 's', object: 'm.F-9' }, place)).toBe(false)
    expect(panelRefreshMatches({ scope: 'children' }, { universe: U, subject: 's', object: 'm.F-7', parent: 'm.F-1' }, place)).toBe(true)
  })

  test('a page row naming its project: navigate, drawer, create under and chat send its qualified id', () => {
    expect(rowProject({ id: 'm.F-1', project: 'erp' })).toBe('erp')
    expect(rowProject({ id: 'm.F-1' })).toBe('')
    expect(rowProject(undefined)).toBe('')
    expect(rowProject({ project: 3 })).toBe('')
    const host = src('components/compositions/PageHost.tsx')
    expect(host).toContain('const inRow = (id: string) => objectRef(id, rowProject(event.item))')
    expect(host).toContain('navigate({ ...action.navigate, object: inRow(action.navigate.object) })')
    expect(host).toContain('object: action.drawer.object ? inRow(action.drawer.object) : here')
    expect(host).toContain('parent: action.create?.parent ? inRow(action.create.parent) : here')
    expect(host).toContain('(action.chat.target ? inRow(action.chat.target) : here) || undefined')
  })
})
