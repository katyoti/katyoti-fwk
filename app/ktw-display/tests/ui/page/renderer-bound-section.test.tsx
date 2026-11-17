// SPDX-License-Identifier: Apache-2.0
// A section bound to the collection it presents (a panel page over a source's
// answer): its count is the badge, and with no element its empty state shows
// instead of its children. A badge may keep its text's case; a link takes a
// size and a weight.

import { describe, expect, it } from 'bun:test'
import { renderToString } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'
import { applyDataBinding } from '../../../../ktw-display-lib/src/ui-builders/page/nodes/data-binding.ts'
import type { UIPageNode } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'

function render(node: UIPageNode, data?: unknown): string {
  return renderToString(
    <MantineProvider>
      <UIPageView node={node} data={data} />
    </MantineProvider>,
  )
}

const goals: UIPageNode = {
  kind: 'section', title: 'Goals', dataField: '$.goals', empty: 'No goals yet.',
  children: [{ kind: 'list', dataField: '$.goals', item: { kind: 'text', content: '', dataField: '$.item.title' } }],
}

describe('a section bound to a collection', () => {
  it('counts it in its badge and shows its elements', () => {
    const bound = applyDataBinding(goals, { goals: [{ title: 'A' }, { title: 'B' }] })
    expect((bound as { badge?: unknown }).badge).toBe(2)
    const html = render(goals, { goals: [{ title: 'Alpha' }, { title: 'Beta' }] })
    expect(html).toContain('Alpha')
    expect(html).not.toContain('No goals yet.')
  })

  it('shows its empty state when the collection has none', () => {
    const html = render(goals, { goals: [] })
    expect(html).toContain('No goals yet.')
    expect((applyDataBinding(goals, { goals: [] }) as { badge?: unknown }).badge).toBe(0)
  })

  it('keeps a declared badge and ignores a value that is no collection', () => {
    expect((applyDataBinding({ ...goals, badge: 'new' } as UIPageNode, { goals: [1] }) as { badge?: unknown }).badge).toBe('new')
    expect(applyDataBinding(goals, { goals: 'x' })).toBe(goals)
  })
})

describe('badge keepCase, link size', () => {
  it('a badge keeps its case when asked', () => {
    expect(render({ kind: 'badge', text: 'Sign up', keepCase: true })).toContain('text-transform:none')
    expect(render({ kind: 'badge', text: 'Sign up' })).not.toContain('text-transform:none')
  })

  it('a link takes a size and a weight', () => {
    const html = render({ kind: 'link', text: 'Open', href: 'https://x.test', size: 'sm', fw: 600 })
    expect(html).toContain('font-weight:600')
  })
})
