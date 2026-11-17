// SPDX-License-Identifier: Apache-2.0
// A source-set form field - one picker per contributed source
// connector, its value the items picked, each with its connector - generic:
// the connectors and their methods come from the method it names, called
// through the host's callMethod.

import { describe, expect, test } from 'bun:test'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { HostServicesContext } from '../src/ui-core/host/index.tsx'
import SourceSetRenderer, {
  asSourceSet, byConnector, connectorLabel, ofConnector, SourceSetPickers, withConnector,
} from '../src/ui-builders/fields/source-set/renderer.tsx'

const value = [
  { connector: 'confluence', key: 'P1', title: 'ERP Arbor' },
  { connector: 'leanix', key: 'A1', title: 'ZContact' },
  { connector: 'confluence', key: 'P2', title: 'Flux' },
]

describe('the source-set field', () => {
  test('reads a value as items picked, ignoring what is not one', () => {
    expect(asSourceSet([...value, { key: 'x' }, 'y', null]).length).toBe(3)
    expect(asSourceSet('nope')).toEqual([])
  })

  test('gives each connector its items and replaces them, the others kept', () => {
    expect(ofConnector(value, 'confluence').map((i) => i.key)).toEqual(['P1', 'P2'])
    expect(ofConnector(value, 'confluence')[0]).not.toHaveProperty('connector')
    const next = withConnector(value, 'confluence', [{ key: 'P3', title: 'Interfaces' }])
    expect(next.map((i) => `${i.connector}:${i.key}`)).toEqual(['leanix:A1', 'confluence:P3'])
  })

  test('groups the value by connector for display', () => {
    expect(byConnector(value).map((g) => [g.connector, g.items.length])).toEqual([['confluence', 2], ['leanix', 1]])
  })

  test('is a form field with a view, a form and a column', () => {
    expect(typeof SourceSetRenderer.form).toBe('function')
    expect(typeof SourceSetRenderer.view).toBe('function')
    expect(typeof SourceSetRenderer.column).toBe('function')
  })

  test('without a host calling methods, shows the value only', () => {
    const html = renderToStaticMarkup(createElement(MantineProvider, null,
      createElement(HostServicesContext.Provider, { value: {} },
        createElement(SourceSetPickers, { content: { type: 'source-set', connectors: 'ktw-agents:sources/connectors' } as any, value, onChange: () => {} }))))
    expect(html).toContain('ERP Arbor')
    expect(html).toContain('ZContact')
    expect(html).not.toContain('source-set.connector')
  })
})

describe('the source-set field says what happens at creation', () => {
  test('its manifest declares the note', async () => {
    const fs = await import('node:fs')
    const raw = fs.readFileSync(new URL('../src/ui-builders/fields/source-set/manifest.yaml', import.meta.url), 'utf8')
    expect(raw).toContain('note:')
  })
})

describe('a source is named by its declared label, never its name', () => {
  test('a group shows the label its connector declares, its name only when none is known', () => {
    const connectors = [{ name: 'leanix-application', label: 'Applications LeanIX', search: 's' }]
    expect(connectorLabel('leanix-application', connectors)).toBe('Applications LeanIX')
    expect(connectorLabel('confluence', connectors)).toBe('confluence')
    expect(connectorLabel('confluence', null)).toBe('confluence')
  })
})
