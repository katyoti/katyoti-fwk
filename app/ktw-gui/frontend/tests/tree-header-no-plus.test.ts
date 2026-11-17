// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

// The project tree's header draws no « + »: creating goes through an object's
// « + Nouveau », and the project additions (a new project, a linked
// repository) through the header's « … » menu.
const src = (file: string) => readFileSync(join(import.meta.dir, '..', 'src', file), 'utf8')

describe('the project tree header', () => {
  test('holds no add button', () => {
    const anywhere = src('sections/workspace/NewAnywhere.tsx')
    expect(anywhere).not.toContain('<ActionIcon')
    expect(anywhere).not.toContain('IconPlus')
    expect(anywhere).not.toContain('data-guide="project.new"')
  })
  test('its « … » menu opens the new-project dialog', () => {
    const panel = src('sections/shell/ProjectPanel.tsx')
    const menu = panel.slice(panel.indexOf('<Menu.Dropdown>'), panel.indexOf('</Menu.Dropdown>'))
    expect(menu).toContain('data-guide="project.add-project"')
    expect(menu).toContain('setNewProject(true)')
    expect(panel).toContain('newProject={newProject} onNewProject={setNewProject}')
  })
})
