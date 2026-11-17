// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { headerChips, headerParts, typeCarries, addedCapabilities } from '../src/sections/workspace/paneHeader'
import { ESSENTIAL_HIDDEN } from '../src/elements'
import { typeStylesOf } from '../src/hooks/useTypeStyles'

// A pane's header is the object-levels mock-up's - its
// type's icon large, its type as the eyebrow over its name, the chips of its
// level, the solution it requires, its parent and its aspects, and a
// « Capabilities » box with what its type carries.

describe("a pane's header says what the object is", () => {
  test('a type gives its level from its capabilities, and the solution it requires', () => {
    const styles = typeStylesOf({
      object_types: [
        { name: 'dossier-architecture', icon: 'folder', capabilities: ['project', 'contributions'], solution: 'ArchiFactory' },
        { name: 'study', capabilities: ['module', 'sources'] },
        { name: 'note', capabilities: ['want'] },
      ],
    })
    expect(styles['dossier-architecture']).toEqual({ icon: 'folder', color: undefined, label: undefined, level: 'project', solution: 'ArchiFactory' })
    expect(styles.study).toEqual({ icon: undefined, color: undefined, label: undefined, level: 'module' })
    expect(styles.note).toBeUndefined()
  })

  test('its chips: level, the solution it requires, its parent - in that order', () => {
    expect(headerChips({ parentId: 'af.demo' }, { level: 'project', solution: 'ArchiFactory' })).toEqual([
      { kind: 'level', value: 'project' },
      { kind: 'requires', value: 'ArchiFactory' },
      { kind: 'parent', value: 'af.demo', opens: 'af.demo' },
    ])
    expect(headerChips({}, undefined)).toEqual([])
  })

  test('the Capabilities box lists what its type carries; what is added stays a badge', () => {
    const infos = [{ id: 'module' }, { id: 'code', source: 'type' }, { id: 'checkable', source: 'aspect:checks' }, { id: 'want', source: 'object' }]
    expect(typeCarries(infos).map((c) => c.id)).toEqual(['module', 'code'])
    expect(addedCapabilities(infos).map((c) => c.id)).toEqual(['checkable', 'want'])
  })

  test('the pane draws them: the declared look, the eyebrow, the chips, the box', () => {
    const src = readFileSync(new URL('../src/sections/workspace/ObjectTab.tsx', import.meta.url), 'utf8')
    for (const want of ['useTypeStyles(', 'headerChips(', 'object.type-eyebrow', 'object.chips', 'object.capabilities', 'only={typeCarries}', 'only={addedCapabilities}', "borderStyle: 'dashed'"]) {
      expect(src).toContain(want)
    }
    expect(src).not.toMatch(/iconForType\(detail\.type\)/)
  })
})

// A non-expert reads mock-up v6's header: the type in
// plain words, the name and its state pill - no « Capacités » box, no level /
// parent / aspect chips, no description, no id line with its raw chips.
describe("a non-expert's header is mock-up v6's", () => {
  test('at Essentiel: the type in plain words, no chips, no description, no id line', () => {
    expect(headerParts('essential')).toEqual({ eyebrowCaps: false, chips: false, description: false, idLine: false })
    // the aspects and the « Capacités » box (with « Planned », « Préparation »…) are left out too
    expect(ESSENTIAL_HIDDEN.has('badge:aspect')).toBe(true)
    expect(ESSENTIAL_HIDDEN.has('badge:capability')).toBe(true)
  })

  test('at Standard and Expert: the whole object-levels header', () => {
    for (const level of ['standard', 'expert'] as const) {
      expect(headerParts(level)).toEqual({ eyebrowCaps: true, chips: true, description: true, idLine: true })
    }
  })

  test('the pane follows the parts and keeps the state pill beside the name', () => {
    const src = readFileSync(new URL('../src/sections/workspace/ObjectTab.tsx', import.meta.url), 'utf8')
    for (const want of ['headerParts(useDisplayLevel())', 'parts.eyebrowCaps ?', '{parts.chips && <Group gap={6} data-guide="object.chips">', '!core && parts.description &&', '{parts.idLine && <Group gap={6} data-guide="object.id-line">', '<ObjectBadges badges={detail.badges}', 'data-guide="object.state"']) {
      expect(src).toContain(want)
    }
    // the eyebrow is capitals only when the parts say so
    expect(src).not.toMatch(/data-guide="object\.type-eyebrow"[^>]*tt="uppercase"/)
  })
})
