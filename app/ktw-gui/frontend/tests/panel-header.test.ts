// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { headerIdentity } from '../src/sections/shell/panelHeader'

// The left panel's header names the universe on display, as the rail shows
// it; the project is the tree's root and is not named twice.
const universes = [
  { name: 'admin', displayName: 'Admin', icon: 'A', color: 'gray' },
  { name: 'dev', displayName: 'Développement', icon: 'D', color: 'red' },
  { name: 'acme', icon: 'S', color: 'grape' },
]
const project = { name: 'ktw-agents', icon: 'KTS', color: 'green' }

describe('the panel header', () => {
  test('names the universe on display, with its icon and colour', () => {
    expect(headerIdentity(universes, 'dev', project)).toEqual({ label: 'Développement', icon: 'D', color: 'red', universe: true })
  })
  test('a universe without a display name shows its name', () => {
    expect(headerIdentity(universes, 'acme', project).label).toBe('acme')
  })
  test('never the project while the universe is known', () => {
    expect(headerIdentity(universes, 'dev', project).label).not.toBe('ktw-agents')
  })
  test('the project only until the universe list has loaded', () => {
    expect(headerIdentity([], 'dev', project)).toEqual({ label: 'ktw-agents', icon: 'KTS', color: 'green', universe: false })
  })
})
