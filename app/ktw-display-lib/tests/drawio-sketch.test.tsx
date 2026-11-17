// SPDX-License-Identifier: Apache-2.0
// A draw.io thumbnail is the drawing's
// sketch - its boxes in the Mantine hue nearest their draw.io colour, its
// arrows - drawn at once in the page's colours: never the viewer's fixed
// white sheet (a white box on the dark theme) nor its loading splash (a dark
// tile on the light theme).

import { describe, expect, test } from 'bun:test'
import { deflateRawSync } from 'node:zlib'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import { UIPageView } from '../src/ui-builders/page/ui-page.view.tsx'
import { drawioMarkLabel, drawioOpenLabel } from '../src/ui-builders/page/nodes/drawio-thumbnail.tsx'
import { inflateDiagram, plainLabel, sketchOf, toneOf } from '../src/ui-builders/drawio/sketch.ts'

const page = (node: unknown) => renderToStaticMarkup(createElement(MantineProvider, null, createElement(UIPageView, { node: node as never })))

const cell = (id: string, label: string, x: number, y: number, style: string, parent = '1') =>
  `<mxCell id="${id}" value="${label}" style="${style}" vertex="1" parent="${parent}"><mxGeometry x="${x}" y="${y}" width="80" height="30" as="geometry"/></mxCell>`
const model = `<mxGraphModel><root><mxCell id="0"/><mxCell id="1" parent="0"/>`
  + cell('a', 'Arbor', 10, 10, 'rounded=1;fillColor=#d5e8d4;strokeColor=#82b366;')
  + cell('b', '&lt;b&gt;Talend&lt;/b&gt;', 200, 10, 'html=1;fillColor=#e1d5e7;strokeColor=#9673a6;')
  + `<UserObject label="Sigma" id="c"><mxCell style="ellipse;fillColor=#ffffff;strokeColor=#6c8ebf;" vertex="1" parent="1"><mxGeometry x="200" y="120" width="80" height="30" as="geometry"/></mxCell></UserObject>`
  + `<mxCell id="g" value="Zone" style="swimlane;" vertex="1" parent="1"><mxGeometry x="400" y="0" width="200" height="120" as="geometry"/></mxCell>`
  + cell('d', 'Oracle', 20, 40, 'fillColor=#f5f5f5;', 'g')
  + `<mxCell id="e1" edge="1" source="a" target="b" parent="1"><mxGeometry relative="1" as="geometry"/></mxCell>`
  + `<mxCell id="e2" edge="1" parent="1"><mxGeometry relative="1" as="geometry"><mxPoint x="240" y="40" as="sourcePoint"/><mxPoint x="240" y="120" as="targetPoint"/></mxGeometry></mxCell>`
  + `</root></mxGraphModel>`
const xml = `<mxfile><diagram id="p" name="Flux">${model}</diagram></mxfile>`

describe('a draw.io drawing read as a sketch', () => {
  test('its boxes, labels, shapes and tones, a child placed in its parent', () => {
    const r = sketchOf(xml)
    if (!('sketch' in r)) throw new Error('no sketch: ' + JSON.stringify(r))
    const by = Object.fromEntries(r.sketch.boxes.map((b) => [b.id, b]))
    expect(by.a).toMatchObject({ label: 'Arbor', shape: 'rounded', tone: 'green' })
    expect(by.b).toMatchObject({ label: 'Talend', tone: 'grape' })
    expect(by.c).toMatchObject({ label: 'Sigma', shape: 'ellipse', tone: 'blue' })
    expect(by.g.container).toBe(true)
    expect(by.d).toMatchObject({ x: 420, y: 40, tone: 'gray' })
    expect(r.sketch.boxes[0].id).toBe('g') // a container under what it holds
    expect(r.sketch.edges).toHaveLength(2)
    // From box a's right side to box b's left side.
    expect(r.sketch.edges[0]).toMatchObject({ x1: 90, y1: 25, x2: 200, y2: 25 })
    expect(r.sketch.view.x).toBeLessThan(10)
  })

  test('a numeric entity out of the Unicode range stays as text', () => {
    expect(plainLabel('A &#99999999; B &#x110000; &#65;&#x42;')).toBe('A &#99999999; B &#x110000; AB')
    const bad = `<mxfile><diagram id="p">${model.replace('Arbor', 'Arbor &amp;#99999999;')}</diagram></mxfile>`
    const r = sketchOf(bad)
    if (!('sketch' in r)) throw new Error('no sketch')
    expect(r.sketch.boxes.find((b) => b.id === 'a')?.label).toBe('Arbor &#99999999;')
  })

  test('the hue nearest a colour; none for no colour', () => {
    expect(toneOf('#dae8fc')).toBe('blue')
    expect(toneOf('#ffe6cc')).toBe('orange')
    expect(toneOf('#f8cecc')).toBe('red')
    expect(toneOf('#ffffff')).toBe('gray')
    expect(toneOf('none')).toBe('')
    expect(sketchOf('').toString()).toBeDefined()
    expect('none' in sketchOf('<mxfile/>')).toBe(true)
  })

  test('a compressed page is inflated first', async () => {
    const packed = Buffer.from(deflateRawSync(Buffer.from(encodeURIComponent(model)))).toString('base64')
    const r = sketchOf(`<mxfile><diagram id="p">${packed}</diagram></mxfile>`)
    if (!('compressed' in r)) throw new Error('not seen as compressed')
    const inflated = sketchOf(await inflateDiagram(r.compressed))
    expect('sketch' in inflated && inflated.sketch.boxes.length).toBe(5)
  })
})

describe('a drawio thumbnail', () => {
  test('draws the sketch in the colour scheme: no white sheet, no splash', () => {
    const html = page({ kind: 'drawio', id: 'flux', xml, thumbnail: true, title: 'Flux applicatifs' })
    expect(html).toContain('data-guide="drawio.sketch"')
    expect(html).toContain('Arbor')
    expect(html).toContain('var(--mantine-color-green-light)')
    expect(html).toContain('light-dark(var(--mantine-color-gray-0), var(--mantine-color-dark-6))')
    expect(html).not.toMatch(/#fff(fff)?\b/i)
    expect(html).not.toContain('preparing your')
    expect(html).not.toContain('drawio.viewer')
  })

  test('a label holding an out-of-range entity never breaks the page', () => {
    const bad = xml.replace('value="Arbor"', 'value="Arbor &#99999999;"')
    const html = page({ kind: 'drawio', id: 'flux', xml: bad, thumbnail: true, title: 'Flux' })
    expect(html).toContain('data-guide="drawio.sketch"')
  })

  test('its labels follow the language', () => {
    expect(drawioOpenLabel('fr', 'Flux')).toBe('Ouvrir le schéma « Flux »')
    expect(drawioOpenLabel('en-US', 'Flux')).toBe('Open the diagram “Flux”')
    expect(drawioOpenLabel('en', undefined)).toBe('Open the diagram')
    expect(drawioMarkLabel('fr-FR')).toBe('Schéma')
    expect(drawioMarkLabel('en')).toBe('Diagram')
  })

  test('a drawing it cannot draw is a quiet mark, never the splash', () => {
    const html = page({ kind: 'drawio', xml: '', thumbnail: true })
    expect(html).toContain('data-guide="drawio.mark"')
    expect(html).not.toContain('preparing your')
  })
})
