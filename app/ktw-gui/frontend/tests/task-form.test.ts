// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { taskActionURI } from '../src/hooks/taskActionURI'

// A task started from « + Nouveau » or a page's chat button opens its form
// first: its reference names the task action whose form is looked up.
describe('taskActionURI', () => {
  test('a task action is kept', () => {
    expect(taskActionURI('ktw-acme:tasks/new-study')).toBe('ktw-acme:tasks/new-study')
  })
  test('a task resource URI names its action', () => {
    expect(taskActionURI('ktw-acme:/task/new-study')).toBe('ktw-acme:tasks/new-study')
  })
  test('a bare name names none (started as before)', () => {
    expect(taskActionURI('new-study')).toBe('')
    expect(taskActionURI('')).toBe('')
  })
})
