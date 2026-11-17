// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'
import ts from 'typescript'

// Every screen speaks the person's language: a .tsx under src/
// shows no user-facing literal - no JSX text, no label-like prop written as a
// string, no notification or modal title or message written as a string - but
// reads a key of src/locales/<lang>/*.json through useT. The source is parsed
// (TypeScript's own parser), so code is never mistaken for text. The Debug tab
// (sections/dev) is developer tooling, seen only at Expert: it is exempt, as
// the few non-words below are; a word that is data takes an `i18n-ignore`
// comment on its line (modules/gui/i18n.md).
const SRC = join(import.meta.dir, '..', 'src')

/** Props a person reads. */
export const LABEL_PROPS = new Set([
  'label', 'placeholder', 'title', 'description', 'aria-label', 'tooltip', 'alt', 'error', 'confirmLabel', 'cancelLabel', 'emptyLabel', 'nothingFoundMessage',
])

/** The fields of a notification or a modal a person reads. */
export const MESSAGE_FIELDS = new Set(['title', 'message', 'labels'])

/** Callees whose object argument a person reads. */
const MESSAGE_CALLEES = /(notifications|modals)\./

/** Folders and files the guard does not read. */
export const EXEMPT_PATHS = [
  'sections/dev/', // the Debug tab: developer tooling, Expert only
  'generated/',
]

/** Texts that are not words of a language: names, symbols, units. */
export const EXEMPT_TEXTS = new Set(['KaTyoti', 'ArchiFactory', 'KTSP', 'NATS', 'MCP', 'JSON', 'YAML', 'URL', 'URI', 'ID', 'OK', 'Git', 'GitHub', 'Confluence', 'LeanIX', 'Dynatrace', 'Drawio', 'draw.io', 'VS Code', 'Claude Code'])

/** A text is a word a person reads when it holds at least two letters. */
function isWords(s: string): boolean {
  const t = s.trim()
  if (t.length === 0 || EXEMPT_TEXTS.has(t)) return false
  return /\p{L}.*\p{L}/u.test(t)
}

export interface Literal { file: string; line: number; text: string }

/** The user-facing literals of one .tsx source. */
export function literalsIn(file: string, source: string): Literal[] {
  const sf = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX)
  const out: Literal[] = []
  const lines = source.split('\n')
  const at = (n: ts.Node) => sf.getLineAndCharacterOfPosition(n.getStart(sf)).line + 1
  const push = (n: ts.Node, text: string) => {
    if (!lines[at(n) - 1]?.includes('i18n-ignore')) out.push({ file, line: at(n), text })
  }
  const stringOf = (e: ts.Expression | undefined) =>
    e && (ts.isStringLiteral(e) || ts.isNoSubstitutionTemplateLiteral(e)) ? e.text : undefined
  const walk = (n: ts.Node) => {
    if (ts.isJsxText(n) && isWords(n.text)) {
      push(n, n.text.trim())
    } else if (ts.isCallExpression(n) && MESSAGE_CALLEES.test(n.expression.getText(sf))) {
      for (const arg of n.arguments) {
        if (!ts.isObjectLiteralExpression(arg)) continue
        for (const p of arg.properties) {
          if (ts.isPropertyAssignment(p) && MESSAGE_FIELDS.has(p.name.getText(sf))) {
            const text = stringOf(p.initializer)
            if (text !== undefined && isWords(text)) push(p, text)
          }
        }
      }
    } else if (ts.isJsxAttribute(n) && LABEL_PROPS.has(n.name.getText(sf)) && n.initializer) {
      const init = n.initializer
      const lit = ts.isStringLiteral(init) ? init
        : ts.isJsxExpression(init) && init.expression && (ts.isStringLiteral(init.expression) || ts.isNoSubstitutionTemplateLiteral(init.expression)) ? init.expression
        : undefined
      if (lit && isWords(lit.text)) push(n, lit.text)
    }
    ts.forEachChild(n, walk)
  }
  walk(sf)
  return out
}

function tsxFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) return tsxFiles(p)
    return (name.endsWith('.tsx') || (name.endsWith('.ts') && !name.endsWith('.d.ts'))) ? [p] : []
  })
}

export function allLiterals(): Literal[] {
  return tsxFiles(SRC)
    .map((p) => relative(SRC, p).replaceAll('\\', '/'))
    .filter((rel) => !EXEMPT_PATHS.some((x) => rel.startsWith(x)))
    .flatMap((rel) => literalsIn(rel, readFileSync(join(SRC, rel), 'utf8')))
}

describe('no user-facing literal in a screen', () => {
  test('every .tsx reads its labels from the locales', () => {
    const found = allLiterals().map((l) => `${l.file}:${l.line} ${JSON.stringify(l.text)}`)
    expect(found).toEqual([])
  })
  test('the guard finds a planted literal', () => {
    expect(literalsIn('x.tsx', 'export const X = () => <Button label="Save the file">Cancel the run</Button>').map((l) => l.text))
      .toEqual(['Save the file', 'Cancel the run'])
    expect(literalsIn('x.tsx', 'export const X = () => <Text>{t("ok")}</Text>')).toEqual([])
    expect(literalsIn('x.tsx', 'notifications.show({ title: "Saved", message: t("m"), color: "green" })').map((l) => l.text)).toEqual(['Saved'])
    expect(literalsIn('x.tsx', 'export const X = () => <Text>KaTyoti Studio</Text> // i18n-ignore')).toEqual([])
    expect(literalsIn('x.tsx', "function f(k: string) { switch (k) { case 'area': return <IconA size={12} /> } }")).toEqual([])
  })
})
