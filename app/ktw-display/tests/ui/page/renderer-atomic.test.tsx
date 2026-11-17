// SPDX-License-Identifier: Apache-2.0
// Smoke tests for the 9 atomic node renderers (S3 of display-dsl-framework).
//
// SSR via `renderToString` + `MantineProvider`. Asserts the right text
// content + key Mantine class hooks appear, and that auto-derived behaviors
// (link external detection, KPI trend color from sign) do the right thing.

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

describe('atomic — heading', () => {
  it('renders text at the right semantic level', () => {
    const html = render({ kind: 'heading', text: 'Hello', level: 2 })
    expect(html).toContain('<h2')
    expect(html).toContain('Hello')
  })

  it('defaults to level 2 when level missing', () => {
    const html = render({ kind: 'heading', text: 'Default' })
    expect(html).toContain('<h2')
  })

  it('honors level 1-6', () => {
    for (const level of [1, 3, 6] as const) {
      const html = render({ kind: 'heading', text: `H${level}`, level })
      expect(html).toContain(`<h${level}`)
    }
  })
})

describe('atomic — text', () => {
  it('renders content', () => {
    const html = render({ kind: 'text', content: 'paragraph copy' })
    expect(html).toContain('paragraph copy')
  })
})

describe('atomic — divider', () => {
  it('renders without crashing', () => {
    const html = render({ kind: 'divider' })
    expect(html.length).toBeGreaterThan(0)
  })

  it('renders a label', () => {
    const html = render({ kind: 'divider', label: 'OR' })
    expect(html).toContain('OR')
  })
})

describe('atomic — badge', () => {
  it('renders badge text', () => {
    const html = render({ kind: 'badge', text: 'OK', color: 'green' })
    expect(html).toContain('OK')
  })
})

describe('atomic — link', () => {
  it('renders an anchor with href', () => {
    const html = render({ kind: 'link', text: 'Docs', href: '/docs' })
    expect(html).toContain('href="/docs"')
    expect(html).toContain('Docs')
  })

  it('auto-detects external (https://) and adds target+rel', () => {
    const html = render({ kind: 'link', text: 'External', href: 'https://example.com' })
    expect(html).toContain('target="_blank"')
    expect(html).toContain('noopener')
  })

  it('does not mark relative URLs external', () => {
    const html = render({ kind: 'link', text: 'Local', href: '/local' })
    expect(html).not.toContain('target="_blank"')
  })
})

describe('atomic — image', () => {
  it('renders an img with src and alt', () => {
    const html = render({ kind: 'image', src: '/x.png', alt: 'X' })
    expect(html).toContain('src="/x.png"')
    expect(html).toContain('alt="X"')
  })
})

describe('atomic — kpi', () => {
  it('renders label and value', () => {
    const html = render({ kind: 'kpi', label: 'Open', value: 42 })
    expect(html).toContain('Open')
    expect(html).toContain('42')
  })

  it('auto-derives green trend color from + sign', () => {
    const html = render({ kind: 'kpi', label: 'X', value: 1, trend: '+12%' })
    expect(html).toContain('+12%')
    // Mantine class for green color shows up
    expect(html).toMatch(/green|var\(--mantine-color-green/)
  })

  it('auto-derives red trend color from - sign', () => {
    const html = render({ kind: 'kpi', label: 'X', value: 1, trend: '-3' })
    expect(html).toMatch(/red|var\(--mantine-color-red/)
  })

  it('renders description if present', () => {
    const html = render({ kind: 'kpi', label: 'X', value: 1, description: 'subtitle text' })
    expect(html).toContain('subtitle text')
  })
})

describe('atomic — code-block', () => {
  it('renders content', () => {
    const html = render({ kind: 'code-block', content: 'const x = 1' })
    expect(html).toContain('const x = 1')
  })

  it('shows language hint when set', () => {
    const html = render({ kind: 'code-block', content: 'x', language: 'typescript' })
    expect(html.toLowerCase()).toContain('typescript')
  })

  it('hides copy button when withCopyButton: false', () => {
    const htmlWith = render({ kind: 'code-block', content: 'x' })
    const htmlWithout = render({ kind: 'code-block', content: 'x', withCopyButton: false })
    expect(htmlWith).toContain('aria-label="copy"')
    expect(htmlWithout).not.toContain('aria-label="copy"')
  })
})

describe('atomic — markdown', () => {
  it('renders inline markdown to HTML', () => {
    const html = render({ kind: 'markdown', content: '# Hello\n\nworld' })
    expect(html).toContain('<h1')
    expect(html).toContain('Hello')
    expect(html).toContain('world')
  })

  it('renders GFM tables', () => {
    const md = '| a | b |\n|---|---|\n| 1 | 2 |\n'
    const html = render({ kind: 'markdown', content: md })
    expect(html).toContain('<table')
    expect(html).toContain('<th')
  })

  it('renders fenced code blocks', () => {
    const md = '```\nconst x = 1\n```'
    const html = render({ kind: 'markdown', content: md })
    expect(html).toContain('const x = 1')
    expect(html).toContain('<code')
  })
})

describe('atomic + layout integration', () => {
  it('a full dashboard mock renders end-to-end', () => {
    const dashboard: UIPageNode = {
      kind: 'vbox',
      gap: 'md',
      children: [
        { kind: 'heading', text: 'Project Dashboard', level: 2 },
        {
          kind: 'grid',
          cols: 3,
          children: [
            { kind: 'kpi', label: 'Open', value: 42, trend: '+3' },
            { kind: 'kpi', label: 'Closed', value: 12, trend: '-2' },
            { kind: 'kpi', label: 'MTTR', value: '4.2h' },
          ],
        },
        { kind: 'divider', label: 'Recent activity' },
        { kind: 'markdown', content: '- Item 1\n- Item 2' },
      ],
    }
    const html = render(dashboard)
    expect(html).toContain('Project Dashboard')
    expect(html).toContain('Open')
    expect(html).toContain('42')
    expect(html).toContain('Recent activity')
    expect(html).toContain('Item 1')
  })
})
