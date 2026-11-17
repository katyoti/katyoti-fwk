// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { colorForType, iconForType, labelForType } from '../src/sections/workspace/type-style'
import { typeStylesOf } from '../src/hooks/useTypeStyles'

// The tree and the lists draw each project and module with the
// icon, colour and name its type declares; a type that declares
// none gets the default icon and a colour hashed from its name.

describe('a type is drawn as it declares', () => {
  test('the type list answer becomes the declared looks, only for types declaring one', () => {
    const styles = typeStylesOf({
      object_types: [
        { name: 'dossier-architecture', icon: 'folder', color: 'teal', label: "Dossier d'architecture" },
        { name: 'documentation', color: 'indigo' },
        { name: 'plain' },
      ],
    })
    expect(styles['dossier-architecture']).toEqual({ icon: 'folder', color: 'teal', label: "Dossier d'architecture" })
    expect(styles.documentation).toEqual({ icon: undefined, color: 'indigo', label: undefined })
    expect(styles.plain).toBeUndefined()
    expect(typeStylesOf(undefined)).toEqual({})
  })

  test('a declared colour and label win; the hash is the fallback', () => {
    expect(colorForType('dossier-architecture', { color: 'teal' })).toBe('teal')
    expect(colorForType('dossier-architecture')).toBe(colorForType('dossier-architecture', {}))
    expect(colorForType('dossier-architecture')).not.toBe('')
    expect(labelForType('documentation', { label: 'Documentation' })).toBe('Documentation')
    expect(labelForType('documentation')).toBe('documentation')
  })

  test('a declared icon is resolved by name; none draws the one default icon, never a hash of the name', () => {
    const fallback = iconForType('dossier-architecture')
    expect(iconForType('dossier-architecture', {})).toBe(fallback)
    expect(iconForType('software-module')).toBe(fallback)
    expect(iconForType('anything-else')).toBe(fallback)
    expect(iconForType('dossier-architecture', { icon: 'folder' })).not.toBe(fallback)
  })

  test('the tree and the links panel read the declared looks', () => {
    for (const f of ['../src/sections/workspace/WorkspaceObjectsTree.tsx', '../src/sections/workspace/ObjectCorePanels.tsx']) {
      const src = readFileSync(new URL(f, import.meta.url), 'utf8')
      expect(src).toContain('useTypeStyles(')
      expect(src).not.toMatch(/iconForType\([^,)]+\)/)
    }
  })
})

// A type is drawn with the colour and the icon it declares - never
// a hash of the name the conversion gave it (software-module hashed to a folder).
describe('a declared colour and a declared icon are drawn', () => {
  test('a declared colour is drawn', () => {
    expect(colorForType('dossier-architecture', { icon: 'folders', color: 'violet' })).toBe('violet')
  })
  test('software-module, documentation and system-module draw their declared icon', () => {
    for (const [type, icon] of [['software-module', 'code'], ['documentation', 'book-2'], ['system-module', 'settings']]) {
      expect(iconForType(type, { icon })).not.toBe(iconForType(type))
    }
  })
})
