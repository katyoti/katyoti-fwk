// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, test } from 'bun:test'
import { devSwitchOn } from '../src/sections/workspace/devSwitch'

// A scenario case's dev GUI turns an increment's display switch on
// for a live check through VITE_KTW_DISPLAY_SWITCHES; outside a dev build the
// list is never read, so the installed build keeps every switch as built.
describe('a display switch on for a case\'s dev GUI only', () => {
  const env = process.env
  afterEach(() => {
    delete env.DEV
    delete env.VITE_KTW_DISPLAY_SWITCHES
  })
  test('off outside a dev build, whatever the list says', () => {
    env.VITE_KTW_DISPLAY_SWITCHES = 'in-037'
    expect(devSwitchOn('in-037')).toBe(false)
  })
  test('in a dev build, on only for the increments listed', () => {
    env.DEV = 'true'
    env.VITE_KTW_DISPLAY_SWITCHES = 'in-036, IN-037'
    expect(devSwitchOn('in-037')).toBe(true)
    expect(devSwitchOn('in-036')).toBe(true)
    expect(devSwitchOn('in-043')).toBe(false)
  })
})
