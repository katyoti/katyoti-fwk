// SPDX-License-Identifier: Apache-2.0
// An accordion fold draws its badge beside its label, as a
// dimmed count (mock-up delivery-views-v2 view 5) - not inside the label,
// not a pill; bound to the page's data like a tab's badge.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'

const page = (node: unknown, data?: unknown) => renderToStaticMarkup(
  createElement(MantineProvider, null, createElement(UIPageView, { node: node as never, data })))

const folds = (multiple: boolean) => ({ kind: 'accordion', multiple, panels: [
  { id: 'r1', label: 'KaTyoti 1.005', badge: 6, content: { kind: 'text', content: 'x' } },
  { id: 'r2', label: 'KaTyoti 1.004', badgeField: '$.counts.r2', content: { kind: 'text', content: 'y' } },
  { id: 'none', label: 'No release', content: { kind: 'text', content: 'z' } },
] })

describe('an accordion fold badge', () => {
  for (const multiple of [true, false]) {
    test(`the label and the dimmed count are drawn apart (multiple: ${multiple})`, () => {
      const html = page(folds(multiple), { counts: { r2: 3 } })
      expect(html).toMatch(/<span>KaTyoti 1\.005<\/span>/)
      expect(html).toMatch(/data-guide="fold.badge" data-guide-name="r1"[^>]*>6</)
      expect(html).toContain('--mantine-color-dimmed')
      expect(html).not.toContain('mantine-Badge-root')
    })

    test(`a badge bound to the page data follows it (multiple: ${multiple})`, () => {
      expect(page(folds(multiple), { counts: { r2: 3 } })).toMatch(/data-guide-name="r2"[^>]*>3</)
      expect(page(folds(multiple), { counts: { r2: 0 } })).toMatch(/data-guide-name="r2"[^>]*>0</)
      expect(page(folds(multiple), {})).not.toContain('data-guide-name="r2"')
    })

    test(`a fold without a badge draws none (multiple: ${multiple})`, () => {
      const html = page(folds(multiple), {})
      expect(html).toContain('No release')
      expect(html).not.toContain('data-guide-name="none"')
      expect(html.match(/data-guide="fold.badge"/g)?.length).toBe(1)
    })
  }
})
