// SPDX-License-Identifier: Apache-2.0
// A page button can hand a file chosen on the person's
// machine - its name and content (base64) laid over the action's args, the
// other args kept; a file too large is refused with why, in the person's
// language, and no declared
// maxSize lets one above what the hub carries through.

import { describe, expect, test } from 'bun:test'

import { UIPageNodeJSONSchema } from '../src/ui-builders/page/ui-page.json-schema.ts'
import { actionVerb, FILE_MAX_SIZE, resolveAction, withFile, type UIPageAction } from '../src/ui-builders/page/ui-page.interactive.ts'

const deposit: UIPageAction = { uri: 'ktw-agents:sources/deposit', args: { id: 'etude-x', by: 'p' }, file: { accept: '.pdf,.docx' } }

describe('a button handing a file', () => {
  test('the chosen file rides with the args, the others kept, file dropped', () => {
    const r = withFile(deposit, { name: 'cahier.pdf', content: 'JVBERi0=', size: 5 })
    expect('action' in r).toBe(true)
    if (!('action' in r)) return
    // As the host runs it: its args resolved with the event's extra.
    expect(resolveAction(r.action, undefined, r.extra).args).toEqual({ id: 'etude-x', by: 'p', name: 'cahier.pdf', content: 'JVBERi0=' })
    expect(r.action.file).toBeUndefined()
    expect(r.action.uri).toBe(deposit.uri)
    expect(actionVerb(r.action)).toBe('uri')
  })
  test('an action declaring no args gets the file alone', () => {
    const r = withFile({ uri: 'ktw-agents:x/y', file: {} }, { name: 'a.txt', content: 'YQ==', size: 1 })
    expect('action' in r && resolveAction(r.action, undefined, r.extra).args).toEqual({ name: 'a.txt', content: 'YQ==' })
  })
  test('a name holding a template is laid as it is, never templated', () => {
    const r = withFile(deposit, { name: 'note ${item.id} {x}.txt', content: 'YQ==', size: 1 })
    expect('action' in r && resolveAction(r.action, { id: 'Z' }, r.extra).args?.name).toBe('note ${item.id} {x}.txt')
  })
  test('a file over the declared maxSize is refused, saying why', () => {
    const r = withFile({ ...deposit, file: { maxSize: 1024 } }, { name: 'big.pdf', content: '', size: 2 * 1024 * 1024 })
    expect('error' in r && r.error).toContain('big.pdf')
    expect('error' in r && r.error).toContain('too large')
  })
  // The refusal speaks the person's language.
  test('the refusal is in French with fr, in English with en or none', () => {
    const big = { name: 'big.pdf', content: '', size: 2 * 1024 * 1024 + 300 * 1024 }
    const small = { ...deposit, file: { maxSize: 1024 * 1024 } }
    const fr = withFile(small, big, 'fr')
    expect('error' in fr && fr.error).toBe('big.pdf est trop volumineux (2,3 Mo, 1 Mo au plus).')
    const frFR = withFile(small, big, 'fr-FR')
    expect('error' in frFR && frFR.error).toContain('trop volumineux')
    const en = withFile(small, big, 'en')
    expect('error' in en && en.error).toBe('big.pdf is too large (2.3 MB, at most 1 MB).')
    const none = withFile(small, big)
    expect('error' in none && none.error).toContain('too large')
  })
  test('no declared maxSize lets more than the hub carries through', () => {
    expect(FILE_MAX_SIZE).toBeLessThanOrEqual(6 * 1024 * 1024)
    const over = FILE_MAX_SIZE + 1
    expect('error' in withFile(deposit, { name: 'x', content: '', size: over })).toBe(true)
    expect('error' in withFile({ ...deposit, file: { maxSize: 100 * 1024 * 1024 } }, { name: 'x', content: '', size: over })).toBe(true)
    expect('action' in withFile(deposit, { name: 'x', content: 'eA==', size: FILE_MAX_SIZE })).toBe(true)
  })
  test('the page schema accepts an action with a file', () => {
    const action = (UIPageNodeJSONSchema as { definitions: Record<string, { properties: Record<string, unknown> }> }).definitions.Action
    expect(action.properties.file).toBeDefined()
  })
})
