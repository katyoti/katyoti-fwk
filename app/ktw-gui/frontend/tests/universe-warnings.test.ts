// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { UNIVERSE_CARD_CHECKS, universeWarnings } from '../src/guide/universeWarnings'
import { errorTextWith } from '../src/i18n/errors'
import en from '../src/locales/en/guide.json'
import fr from '../src/locales/fr/guide.json'

// A universe's card shows the failed universe checks as warnings, said by
// their key in the guide namespace (modules/guide/tools.md, « Checks »).
const noRemote = { ok: false, detail: 'No remote for D:/work: its work is not backed up off this machine.', detailMsg: { key: 'checks.noRemote', params: { repositories: 'D:/work' } } }

function tOf(table: Record<string, unknown>) {
  return (key: string, opts?: Record<string, unknown>) => {
    let s = key.split('.').reduce<unknown>((o, k) => (o as Record<string, unknown> | undefined)?.[k], table) as string | undefined
    if (s === undefined) return String(opts?.defaultValue ?? key)
    for (const [k, v] of Object.entries(opts ?? {})) s = s.split(`{{${k}}}`).join(String(v))
    return s
  }
}

describe('a universe card warns of its failed universe checks', () => {
  test('the card runs the repositories-have-remote check', () => {
    expect(UNIVERSE_CARD_CHECKS).toContain('ktw-univers:universe-checks/repositories-have-remote')
  })
  test('a failed check is a warning; a passed or unreachable one is none', async () => {
    const answers: Record<string, unknown> = { 'a:c/fail': noRemote, 'a:c/pass': { ok: true, detail: 'Every repository has a remote.' } }
    const run = async (uri: string, universe: string) => {
      expect(universe).toBe('work')
      if (uri === 'a:c/down') throw new Error('no hub')
      return answers[uri]
    }
    const w = await universeWarnings('work', ['a:c/pass', 'a:c/fail', 'a:c/down'], run)
    expect(w).toHaveLength(1)
    expect(w[0].detailMsg?.key).toBe('checks.noRemote')
  })
  test('the warning is said in the GUI language, naming the repository', () => {
    const say = (t: ReturnType<typeof tOf>) => errorTextWith(t as never, noRemote.detail, noRemote.detailMsg)
    expect(say(tOf(fr))).toBe('Aucun dépôt distant pour D:/work : son travail n’est pas sauvegardé hors de ce poste.')
    expect(say(tOf(en))).toBe('No remote for D:/work: its work is not backed up off this machine.')
    expect(tOf(fr)('checks.remotes', { universe: 'work' })).not.toBe('checks.remotes')
    expect(tOf(fr)('checks.noRepository', { universe: 'work' })).not.toBe('checks.noRepository')
  })
})
