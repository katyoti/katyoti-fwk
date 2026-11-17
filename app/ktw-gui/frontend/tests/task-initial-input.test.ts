// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'fs'
import { join } from 'path'
import { NO_INITIAL_INPUT_TAG, runOpening, takesNoInitialInput } from '../src/hooks/taskInitialInput'

// A task that asks its questions itself (a wizard whose every
// question is a form card in its chat) declares it takes no initial message,
// and its run starts at once - no free-text « Entrée de la tâche » box - from
// every entry point (they all open the run through RunTaskPanel).
describe('what a run opens with', () => {
  test('a task declaring no initial input opens no input box: it starts at once', () => {
    const wizard = { tags: ['quick-action', NO_INITIAL_INPUT_TAG] }
    expect(takesNoInitialInput(wizard)).toBe(true)
    expect(runOpening(wizard)).toBe('start')
  })
  test('any other task opens the initial-message box, as before', () => {
    expect(runOpening({ tags: ['quick-action'] })).toBe('input-box')
    expect(runOpening({})).toBe('input-box')
    expect(runOpening(null)).toBe('input-box')
  })
  test('a task with a form opens its form', () => {
    expect(runOpening({ inputSchemaURI: 'p:/data-type/t.input', tags: [NO_INITIAL_INPUT_TAG] })).toBe('form')
  })
})

describe('RunTaskPanel', () => {
  const panel = readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'agent', 'RunTaskPanel.tsx'), 'utf-8')
  test('returns before the box when the task starts at once', () => {
    const start = panel.indexOf("if (opening === 'start') {")
    const box = panel.indexOf('<Textarea')
    expect(start).toBeGreaterThan(0)
    expect(box).toBeGreaterThan(start)
  })
  test('every opener hands it the task\'s tags', () => {
    for (const f of ['sections/home/HomeTab.tsx', 'sections/task/TaskGalleryPanel.tsx', 'sections/workspace/EmptyInvite.tsx']) {
      const src = readFileSync(join(import.meta.dir, '..', 'src', f), 'utf-8')
      expect(src.includes('tags:')).toBe(true)
    }
  })
})
