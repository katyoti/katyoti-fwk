// SPDX-License-Identifier: Apache-2.0
// The dashboard blocks of the page DSL — what a declared panel's page needs to
// read like a registered view: a section with its icon, count badge, aside and
// empty state; a badge with an icon, a tooltip and monospace text; a gauge
// stacked from segments with its legend; a collapsible markdown; a dashed
// divider. The pure measure (gaugeBar) is tested apart from the render (SSR).

import { describe, expect, it } from 'bun:test'
import { renderToString } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView, childrenOf, gaugeBar } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'
import type { UIPageNode } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'

function render(node: UIPageNode): string {
  return renderToString(
    <MantineProvider>
      <UIPageView node={node} />
    </MantineProvider>,
  )
}

describe('gauge — gaugeBar', () => {
  it('a single bar is its value against its max', () => {
    const { value, max, parts } = gaugeBar({ kind: 'gauge', label: 'tier', value: 3, max: 4 })
    expect([value, max, parts.length, parts[0].percent]).toEqual([3, 4, 1, 75])
  })

  it('stacked segments sum to the value; the max is their sum unless declared', () => {
    const g = gaugeBar({ kind: 'gauge', segments: [{ value: 3, color: 'green' }, { value: 1, color: 'red' }] })
    expect([g.value, g.max, g.parts.map((p) => p.percent)]).toEqual([4, 4, [75, 25]])
    const declared = gaugeBar({ kind: 'gauge', max: 8, segments: [{ value: 2 }] })
    expect(declared.parts[0].percent).toBe(25)
  })

  it('a negative or non-numeric part counts 0 and no share overflows', () => {
    const g = gaugeBar({ kind: 'gauge', max: 1, segments: [{ value: -2 }, { value: 'x' as never }, { value: 5 }] })
    expect(g.parts.map((p) => p.percent)).toEqual([0, 0, 100])
  })
})

describe('dashboard blocks — render', () => {
  it('a stacked gauge draws its parts and, with legend, one chip per non-zero part', () => {
    const html = render({
      kind: 'gauge', label: 'Use-cases', valueText: '', legend: true,
      segments: [{ value: 3, color: 'green', label: 'pass' }, { value: 0, color: 'yellow', label: 'deferred' }, { value: 1, color: 'red', label: 'failing' }],
    })
    expect(html).toContain('3 pass')
    expect(html).toContain('1 failing')
    expect(html).not.toContain('0 deferred')
    expect(html).not.toContain(' / ')
  })

  it('a section shows its icon row, count badge and aside, or its empty state', () => {
    const html = render({
      kind: 'section', title: 'Promote gates', icon: 'clock', size: 'sm', badge: '2 blocking', badgeColor: 'orange',
      aside: [{ kind: 'text', content: '12 total' }],
      children: [{ kind: 'text', content: 'charter version < 0.1' }],
    })
    expect(html).toContain('Promote gates')
    expect(html).toContain('2 blocking')
    expect(html).toContain('12 total')
    expect(html).toContain('charter version')
    const empty = render({ kind: 'section', title: 'Promote gates', children: [], empty: 'All gates clear', emptyColor: 'green' })
    expect(empty).toContain('All gates clear')
  })

  it('a section walks its aside as children (ids, patches)', () => {
    const kids = childrenOf({ kind: 'section', aside: [{ kind: 'badge', text: 'a' }], children: [{ kind: 'text', content: 'b' }] })
    expect(kids.map((k) => k.kind)).toEqual(['badge', 'text'])
  })

  it('a badge carries its icon and keeps the case of monospace text', () => {
    const html = render({ kind: 'badge', text: 'internal/Core', icon: 'circle-check', mono: true, tooltip: 'role' })
    expect(html).toContain('internal/Core')
    expect(html).toContain('text-transform:none')
  })

  it('a markdown with maxHeight is collapsible', () => {
    const html = render({ kind: 'markdown', content: 'long text', maxHeight: 80 })
    expect(html).toContain('long text')
    // The toggle appears once the browser measures the text; the fold is there.
    expect(html).toContain('mantine-Spoiler-root')
    expect(render({ kind: 'markdown', content: 'short' })).not.toContain('mantine-Spoiler-root')
  })
})
