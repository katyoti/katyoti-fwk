// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { GUIDE_ANCHORS } from '../src/guide/anchors'

// Every part of the approved object-levels mock-up
// (modules/research-object-levels/object-levels-mockup.html) is drawn by
// the screen at a registered anchor - a GUI test reaches it by id, and a part
// the screen stops drawing fails here.
const MOCKUP_PARTS: Record<string, string> = {
  'the tree: one header per module type': 'tree.group',
  'the pane: the type icon, large': 'object.type-icon',
  'the pane: the type as the eyebrow': 'object.type-eyebrow',
  'the pane: level, requires, parent and aspect chips': 'object.chips',
  'the pane: the Capabilities box': 'object.capabilities',
  'the pane: « + New », its toolbar button': 'object.new',
  'the pane: the menu of « + New »': 'object.new-menu',
  'the pane: one type to add': 'object.new-type',
  '« + New » anywhere: the panel « … » adds a project': 'project.add-project',
  'the new-project dialog': 'project.new-project',
}

describe('the object-levels mock-up, part by part', () => {
  for (const [part, anchor] of Object.entries(MOCKUP_PARTS)) {
    test(`${part} - ${anchor}`, () => {
      const a = GUIDE_ANCHORS[anchor]
      expect(a).toBeDefined()
      const src = readFileSync(join(import.meta.dir, '..', 'src', a.file), 'utf8')
      expect(src).toContain(`data-guide="${anchor}"`)
    })
  }
})
