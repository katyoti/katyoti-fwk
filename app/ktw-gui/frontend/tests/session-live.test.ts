// SPDX-License-Identifier: Apache-2.0
// A session still running is followed live (read again until it ends); an
// ended one — completed, failed, or with no status yet known — is read once.

import { describe, expect, it } from 'bun:test'

import { sessionIsLive } from '../src/sections/shell/SessionViewer'

describe('SessionViewer follows a running session', () => {
  it('a running session is live', () => {
    expect(sessionIsLive('running')).toBe(true)
  })
  it('an ended or unknown one is not', () => {
    for (const s of ['completed', 'failed', 'error', '', undefined]) expect(sessionIsLive(s)).toBe(false)
  })
})
