// SPDX-License-Identifier: Apache-2.0
/**
 * messageTime — the time a chat message shows in its sender header.
 *
 * A message carries `at`, the moment it was sent (the persisted message's
 * timestamp, or the moment the chat received it when it arrived without
 * one): the header shows that moment, never the time the view is drawn, so
 * a re-render keeps each message's own time.
 */

const TIME_FORMAT: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit' }

/** The hour and minute of a message's `at`; empty when it has none or it is not a date. */
export function messageTime(at?: string): string {
  if (!at) return ''
  const d = new Date(at)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleTimeString([], TIME_FORMAT)
}

/** The message with its `at` - its own when it has one, else `now` (the moment it is received). */
export function stampReceived<T extends { at?: string }>(msg: T, now: Date = new Date()): T {
  return msg.at ? msg : { ...msg, at: now.toISOString() }
}
