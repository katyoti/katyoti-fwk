// SPDX-License-Identifier: Apache-2.0
// An array field's inline tree/table extension decides its columns:
// the field renderers resolve the extension, name it, and the
// ui-tree-table / ui-table builders must find it by that name - an unnamed
// lookup misses it and every item field becomes a column, in the order the
// schema lists them (Go sends them alphabetically).

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { wireFieldRenderers } from '../src/ui-builders/fields/factory.ts'

// The Confluence run panel's item type, its fields in Go's alphabetical order.
const statusNode = {
  type: 'object', label: 'Page', idField: ['id'], titleField: ['title'],
  fields: {
    direction: { type: 'string', label: '' },
    id: { type: 'string', label: 'ID' },
    local_state: { type: 'string', label: 'Local state' },
    local_version: { type: 'string', label: 'Local' },
    parent_id: { type: 'string', label: 'Parent' },
    remote_state: { type: 'string', label: 'Remote state' },
    remote_version: { type: 'string', label: 'Remote' },
    state: { type: 'string', label: 'State' },
    status: { type: 'string', label: 'Status' },
    title: { type: 'string', label: 'Page' },
  },
}

const columns = [
  { id: 'local_version', label: 'Local', kind: 'version', stateField: 'local_state', align: 'center', width: 90 },
  { id: 'direction', label: '', kind: 'direction', align: 'center', width: 56 },
  { id: 'remote_version', label: 'Remote', kind: 'version', stateField: 'remote_state', align: 'center', width: 90 },
  { id: 'title', label: 'Page' },
  { id: 'status', label: 'Status' },
]

function schemaWith(content: Record<string, unknown>, item: Record<string, unknown> = statusNode) {
  return {
    name: 'confluence-progress', version: 1, root: 'ProgressTree',
    types: {
      ProgressTree: {
        type: 'object', label: 'Synchronisation', idField: ['phase'], titleField: ['summary'],
        fields: {
          nodes: { type: 'array', label: 'Pages', items: 'StatusNode', content },
          phase: { type: 'string', label: 'Phase' },
          status: { type: 'string', label: 'State' },
          summary: { type: 'string', label: 'Summary' },
        },
        extensions: [{ type: 'ui-view', name: 'default', layout: 'card', groups: [
          { id: 'hdr', label: 'Run', cols: 3, fields: ['phase', 'status', 'summary'] },
          { id: 'pages', label: 'Pages', fields: ['nodes'] },
        ] }],
      },
      StatusNode: item,
    },
  }
}

const data = {
  phase: 'pull', status: 'running', summary: '1 page',
  nodes: [{
    id: '4657643699', parent_id: '3742007373', title: 'Architecture overview', state: 'discovered',
    local_version: '3', local_state: 'ok', direction: 'pull', remote_version: '4', remote_state: 'newer', status: 'pending',
  }],
}

function rowCells(content: Record<string, unknown>, item?: Record<string, unknown>): string[][] {
  const html = renderToStaticMarkup(
    createElement(MantineProvider, null,
      createElement(UIPageView, { node: { kind: 'view', id: 'run', schema: schemaWith(content, item), data } as never, onAction: (() => {}) as never })))
  const body = html.split('<tbody')[1] ?? ''
  return body.split('<tr').slice(1).map((tr) =>
    [...tr.matchAll(/<td[^>]*>([\s\S]*?)<\/td>/g)].map((m) => m[1].replace(/<[^>]+>/g, '').trim()))
}

describe('an array field shows the columns its inline extension declares', () => {
  wireFieldRenderers()

  test('tree: Local, direction, Remote, Page, Status - the title in the 4th, no id cell', () => {
    const rows = rowCells({ type: 'tree', extension: {
      idField: 'id', parentField: 'parent_id', stateField: 'status', treeColumn: 'title', indentPx: 22, columns,
    } })
    expect(rows.length).toBe(1)
    expect(rows[0].length).toBe(5)
    expect(rows[0][3]).toContain('Architecture overview')
    expect(rows[0].some((c) => c.includes('4657643699'))).toBe(false)
    expect(rows[0].some((c) => c.includes('3742007373'))).toBe(false)
  })

  test('table: the inline ui-table columns, in their order', () => {
    const rows = rowCells({ type: 'table', extension: {
      columns: [{ id: 'title', label: 'Page' }, { id: 'status', label: 'Status' }],
    } })
    expect(rows.length).toBe(1)
    expect(rows[0].length).toBe(2)
    expect(rows[0][0]).toContain('Architecture overview')
    expect(rows[0][1]).toContain('pending')
  })

  test('no extension: every item field is a column, as before', () => {
    const rows = rowCells({ type: 'tree' })
    expect(rows.length).toBe(1)
    expect(rows[0].length).toBe(Object.keys(statusNode.fields).length)
    expect(rows[0].some((c) => c.includes('4657643699'))).toBe(true)
    expect(rows[0].some((c) => c.includes('Architecture overview'))).toBe(true)
  })

  test('named extension: the item type extension of that name gives the columns', () => {
    const item = { ...statusNode, extensions: [
      { type: 'ui-tree-table', name: 'other', columns: [{ id: 'id', label: 'ID' }] },
      { type: 'ui-tree-table', name: 'pages', idField: 'id', parentField: 'parent_id', treeColumn: 'title',
        columns: [{ id: 'title', label: 'Page' }, { id: 'status', label: 'Status' }] },
    ] }
    const rows = rowCells({ type: 'tree', extension: 'pages' }, item)
    expect(rows.length).toBe(1)
    expect(rows[0].length).toBe(2)
    expect(rows[0][0]).toContain('Architecture overview')
    expect(rows[0][1]).toContain('pending')
  })
})
