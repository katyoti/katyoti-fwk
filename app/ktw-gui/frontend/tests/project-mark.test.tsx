// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { ProjectMarkView, projectMarkOf, unlocate } from '../src/sections/workspace/projectMark'

// The tree's nodes, the object pane's header and the search
// results show the project holding each object when the universe serves
// several (cwm's store set names it), for everyone - no display switch
// (the one it had is retired); with one project (no key in the answer), no mark.
const src = (p: string) => readFileSync(join(import.meta.dir, '../src', p), 'utf-8')
const html = (node: React.ReactNode) => renderToStaticMarkup(<MantineProvider>{node}</MantineProvider>)

describe("an object's project mark", () => {
  test('drawn whenever a project is named', () => {
    expect(projectMarkOf('crm')).toBe('crm')
    expect(projectMarkOf(' crm ')).toBe('crm')
    expect(projectMarkOf('')).toBeNull()
    expect(projectMarkOf(undefined)).toBeNull()
    expect(projectMarkOf(null)).toBeNull()
    expect(projectMarkOf('  ')).toBeNull()
  })

  test('the mark names its project by its element id', () => {
    const out = html(<ProjectMarkView mark="crm" guide="tree.project-mark" title="Dans le projet crm" />)
    expect(out).toContain('data-guide="tree.project-mark"')
    expect(out).toContain('data-project="crm"')
    expect(out).toContain('>crm<')
  })

  test('an answer read in either form: bare (one store) or located (the store set)', () => {
    expect(unlocate({ id: 'crm.F-1', type: 'feature' })).toEqual({ id: 'crm.F-1', type: 'feature' })
    expect(unlocate({ project: 'crm', object: { id: 'crm.F-1', type: 'feature' } })).toEqual({ id: 'crm.F-1', type: 'feature', project: 'crm' })
    expect(unlocate({ object: { id: 'crm.F-1' } })).toEqual({ id: 'crm.F-1' })
  })

  test('the tree, the object pane header and the search results draw it', () => {
    const tree = src('sections/workspace/WorkspaceObjectsTree.tsx')
    expect(tree).toContain('project: o.project')
    expect(tree).toContain('<ProjectMark project={node.nodeProps?.project as string | undefined} guide="tree.project-mark" />')
    expect(src('sections/workspace/ObjectTab.tsx')).toContain('<ProjectMark project={detail.project} guide="object.project-mark" />')
    expect(src('sections/workspace/ObjectCorePanels.tsx')).toContain('<ProjectMark project={o.project} guide="search.project-mark" />')
    expect(src('components/compositions/hostServices.tsx')).toContain('.map((o) => unlocate(o))')
  })

  test('the mark reads no display switch: it shows for everyone', () => {
    const mark = src('sections/workspace/projectMark.tsx')
    expect(mark).not.toContain('useDisplaySwitch')
  })
})
