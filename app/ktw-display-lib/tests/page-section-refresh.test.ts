// SPDX-License-Identifier: Apache-2.0
// A section on its own: it re-reads when an object change its
// refresh matches is over, and a section that fails shows an error card with
// « Réessayer » - only that section, the rest of the page stays.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { retryLabel, SectionFailed, sectionWatches } from '../src/ui-builders/page/nodes/section-load.tsx'

describe('a section on its own', () => {
  test('watches changes only when it declares a refresh other than none', () => {
    expect(sectionWatches({ scope: 'module', types: ['release'] })).toBe(true)
    expect(sectionWatches({ scope: 'self', object: 'm.RL-1' })).toBe(true)
    expect(sectionWatches({ scope: 'none' })).toBe(false)
    expect(sectionWatches(undefined)).toBe(false)
  })

  test('a failed section is an error card with its message and « Réessayer »', () => {
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(SectionFailed, { id: 'lane:SM-1', message: 'the lane could not be read', onRetry: () => {} })))
    expect(html).toContain('data-section="lane:SM-1"')
    expect(html).toContain('data-state="failed"')
    expect(html).toContain('the lane could not be read')
    expect(html).toContain('data-section-retry="lane:SM-1"')
    expect(retryLabel('fr')).toBe('Réessayer')
    expect(retryLabel('en')).toBe('Try again')
  })
})
