// SPDX-License-Identifier: Apache-2.0
// Smoke tests for <UIPageView> + the 8 layout node renderers (S2 of
// display-dsl-framework).
//
// These use `react-dom/server` to render representative trees to HTML
// strings and assert the structure looks right (no exceptions, expected
// text appears, expected element classes appear). Layout nodes only —
// atomic + schema-bound nodes are placeholders here (S3/S4 land them).
//
// Why renderToString and not a full DOM harness: the project doesn't
// have @testing-library installed, and we just need a "renders without
// crashing + produces sensible HTML" smoke check. Full interactive
// tests live downstream in `display-demo` (S9).

import { describe, expect, it } from 'bun:test'
import { renderToString } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'
import type { UIPageNode } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'

function render(node: UIPageNode): string {
  return renderToString(
    <MantineProvider>
      <UIPageView node={node} />
    </MantineProvider>,
  )
}

describe('<UIPageView> — layout dispatch', () => {
  it('renders an empty vbox without crashing', () => {
    const html = render({ kind: 'vbox', children: [] })
    expect(html.length).toBeGreaterThan(0)
  })

  it('renders a vbox with atomic children (S3 wired)', () => {
    const html = render({
      kind: 'vbox',
      children: [
        { kind: 'heading', text: 'Welcome' },
        { kind: 'text', content: 'body copy' },
      ],
    })
    expect(html).toContain('Welcome')
    expect(html).toContain('body copy')
  })

  it('renders an hbox', () => {
    const html = render({
      kind: 'hbox',
      gap: 'sm',
      children: [{ kind: 'badge', text: 'A' }, { kind: 'badge', text: 'B' }],
    })
    expect(html.length).toBeGreaterThan(0)
  })

  it('renders a 3-column grid with KPI placeholders', () => {
    const html = render({
      kind: 'grid',
      cols: 3,
      children: [
        { kind: 'kpi', label: 'A', value: 1 },
        { kind: 'kpi', label: 'B', value: 2 },
        { kind: 'kpi', label: 'C', value: 3 },
      ],
    })
    expect(html.length).toBeGreaterThan(0)
  })

  it('renders a card', () => {
    const html = render({
      kind: 'card',
      withBorder: true,
      children: [{ kind: 'heading', text: 'In a card' }],
    })
    expect(html.length).toBeGreaterThan(0)
  })

  it('renders a section with title + description', () => {
    const html = render({
      kind: 'section',
      title: 'Project Status',
      description: 'Q3 snapshot',
      children: [{ kind: 'text', content: 'body' }],
    })
    expect(html).toContain('Project Status')
    expect(html).toContain('Q3 snapshot')
  })

  it('renders tabs with two panels', () => {
    const html = render({
      kind: 'tabs',
      tabs: [
        { id: 'a', label: 'Alpha', content: { kind: 'heading', text: 'A' } },
        { id: 'b', label: 'Beta', content: { kind: 'heading', text: 'B' } },
      ],
    })
    expect(html).toContain('Alpha')
    expect(html).toContain('Beta')
  })

  it('renders accordion with single-open mode', () => {
    const html = render({
      kind: 'accordion',
      panels: [
        { id: 'p1', label: 'Section 1', content: { kind: 'text', content: 'body 1' } },
        { id: 'p2', label: 'Section 2', content: { kind: 'text', content: 'body 2' } },
      ],
    })
    expect(html).toContain('Section 1')
    expect(html).toContain('Section 2')
  })

  it('renders accordion with multiple-open mode', () => {
    const html = render({
      kind: 'accordion',
      multiple: true,
      defaultOpen: ['p1'],
      panels: [
        { id: 'p1', label: 'Open by default', content: { kind: 'text', content: 'a' } },
      ],
    })
    expect(html).toContain('Open by default')
  })

  it('renders deeply nested layouts with atomic leaves', () => {
    const html = render({
      kind: 'vbox',
      children: [
        {
          kind: 'card',
          children: [
            {
              kind: 'grid',
              cols: 2,
              children: [
                { kind: 'box', children: [{ kind: 'heading', text: 'Inner' }] },
                { kind: 'box', children: [{ kind: 'heading', text: 'Inner 2' }] },
              ],
            },
          ],
        },
      ],
    })
    expect(html).toContain('Inner')
    expect(html).toContain('Inner 2')
  })
})

describe('<UIPageView> — schema-bound v1 limitations', () => {
  it('a schema named by id reaching the page unresolved surfaces a diagnostic', () => {
    const html = render({ kind: 'form', schema: 'ktw-demo:/data-type/book' })
    expect(html).toContain('not resolved')
  })

  it('a node without a schema surfaces a diagnostic', () => {
    const html = render({ kind: 'view' })
    expect(html).toContain('has no schema')
  })
})
