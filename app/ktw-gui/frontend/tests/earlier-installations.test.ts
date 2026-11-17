// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import {
  canAskDelete, confirmDelete, earlierRows, humanSize, keptByDefault, planDelete, refusalKey, REFUSAL_REASONS,
  type Deleter, type Installation,
} from '../src/sections/ready/earlierInstallations'
import type { DeleteInstallationInput } from '../src/ktsp/marketplace'

// « Prêt à démarrer » lists the earlier installations and deletes
// one only after an explicit acknowledgement; the most recently used one is
// kept unless the person unticks it.

const inst = (name: string, p: Partial<Installation> = {}): Installation => ({ name, title: name, folder: `/inst/${name}`, bytes: 2048, ...p })
function hub(): Deleter & { calls: DeleteInstallationInput[] } {
  const calls: DeleteInstallationInput[] = []
  return {
    calls,
    deleteInstallation: async (input) => {
      calls.push(input)
      return { name: input.name, folders: [{ path: `/inst/${input.name}`, bytes: 2048 }], shortcuts: input.name, token: `${input.name}-1234`, deleted: !!input.ack }
    },
  }
}

describe('the earlier installations', () => {
  const out = { installations: [inst('now', { current: true, reason: 'current' }), inst('old', { last_used: '2026-01-01T00:00:00Z' }), inst('prev', { last_used: '2026-09-30T00:00:00Z' })] }

  test('every earlier one is listed, the newest used first - never the one answering', () => {
    expect(earlierRows(out).map((r) => r.name)).toEqual(['prev', 'old'])
  })

  test('the most recently used one is kept unless the person unticks it', () => {
    const rows = earlierRows(out)
    const kept = keptByDefault(rows)
    expect([...kept]).toEqual(['prev'])
    expect(canAskDelete(rows[0], kept)).toBe(false)
    expect(canAskDelete(rows[1], kept)).toBe(true)
    expect(canAskDelete(rows[0], new Set())).toBe(true)
  })

  test('nothing is deleted without the acknowledgement: the plan touches nothing, confirming unacknowledged makes no call', async () => {
    const h = hub()
    const plan = await planDelete(h, inst('old'))
    expect(h.calls).toEqual([{ name: 'old', delete_last: false }])
    expect(plan.deleted).toBe(false)
    expect(await confirmDelete(h, plan, false, false)).toBeUndefined()
    expect(h.calls.length).toBe(1)
    const done = await confirmDelete(h, plan, true, false)
    expect(h.calls[1]).toEqual({ name: 'old', ack: 'old-1234', delete_last: false })
    expect(done?.deleted).toBe(true)
  })

  test('the hub refusing one keeps it off; the last one only once the person says so', () => {
    expect(canAskDelete(inst('a', { reason: 'projects-inside' }), new Set())).toBe(false)
    expect(canAskDelete(inst('a', { reason: 'shared' }), new Set())).toBe(false)
    expect(canAskDelete(inst('a', { reason: 'last' }), new Set())).toBe(true)
    expect(refusalKey(inst('a', { reason: 'shared' }))).toBe('installations.refused.shared')
  })

  test('sizes read as a person reads them; every refusal and the screen words in English and French', () => {
    expect(humanSize(512)).toBe('512 B')
    expect(humanSize(3 * 1024 * 1024)).toBe('3.0 MB')
    for (const lang of ['en', 'fr']) {
      const msgs = JSON.parse(readFileSync(join(import.meta.dir, '..', 'src', 'locales', lang, 'ready.json'), 'utf8'))
      for (const r of REFUSAL_REASONS) expect(msgs.installations.refused[r] ?? '').not.toBe('')
      for (const k of ['title', 'hint', 'keep', 'delete', 'ack', 'confirm', 'cancel', 'goes']) expect(msgs.installations[k] ?? '').not.toBe('')
    }
    const screen = readFileSync(join(import.meta.dir, '..', 'src', 'sections', 'ready', 'EarlierInstallationsSection.tsx'), 'utf8')
    for (const id of ['section:ready/earlier-installations', 'field:ready/installation-keep', 'action:ready/installation-delete', 'field:ready/installation-ack', 'action:ready/installation-delete-confirm']) expect(screen).toContain(id)
  })
})
