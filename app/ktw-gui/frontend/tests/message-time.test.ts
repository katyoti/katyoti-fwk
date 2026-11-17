// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'

import { messageTime, stampReceived } from '../src/sections/agent/messageTime'

// A chat message's header shows the time it was sent, never the time the
// view is drawn: a re-render keeps each message's own time.
describe('a chat message shows the time it was sent', () => {
  const sent = '2026-10-12T09:05:00Z'
  const own = new Date(sent).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

  test('the header time is the message time, not the current time', () => {
    expect(messageTime(sent)).toBe(own)
    const now = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    const later = new Date(new Date(sent).getTime() + 3 * 3600_000).toISOString()
    expect(messageTime(later)).not.toBe(own)
    if (now !== own) expect(messageTime(sent)).not.toBe(now)
  })

  test('two renders of the same message show the same time', () => {
    expect(messageTime(sent)).toBe(messageTime(sent))
  })

  test('a message with no time or a broken one shows none', () => {
    expect(messageTime(undefined)).toBe('')
    expect(messageTime('not a date')).toBe('')
  })

  test('a received message keeps its own time, one without gets the moment it arrives', () => {
    const arrived = new Date('2026-10-12T10:00:00Z')
    expect(stampReceived({ id: 'm1', at: sent }, arrived).at).toBe(sent)
    expect(stampReceived({ id: 'm2' }, arrived).at).toBe(arrived.toISOString())
  })
})
