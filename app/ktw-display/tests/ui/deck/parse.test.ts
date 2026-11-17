// SPDX-License-Identifier: Apache-2.0
// The deck corpus, run against the TypeScript splitter. The same cases run
// against the Go splitter (internal/extensions/slidedeck/api/corpus_test.go);
// the expected <case>.json is written by the Go side and reviewed. A case that
// passes there and fails here is drift between the two readers of the grammar.

import { describe, expect, it } from 'bun:test'
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

import { parseDeck } from '../../../../ktw-display-lib/src/ui-builders/deck/parse.ts'

const corpus = join(import.meta.dir, '../../../../../internal/extensions/slidedeck/api/testdata/corpus')
const cases = readdirSync(corpus).filter((f) => f.endsWith('.md')).map((f) => f.slice(0, -3))

describe('deck corpus — TypeScript splitter matches the Go splitter', () => {
  it('has cases', () => expect(cases.length).toBeGreaterThan(5))
  for (const name of cases) {
    it(name, () => {
      const src = readFileSync(join(corpus, `${name}.md`), 'utf8')
      const want = JSON.parse(readFileSync(join(corpus, `${name}.json`), 'utf8'))
      expect(parseDeck(src)).toEqual(want)
    })
  }
  it('CRLF parses like LF', () => {
    const lf = readFileSync(join(corpus, 'basic.md'), 'utf8').replace(/\r\n/g, '\n')
    expect(parseDeck(lf.replace(/\n/g, '\r\n'))).toEqual(parseDeck(lf))
  })
})
