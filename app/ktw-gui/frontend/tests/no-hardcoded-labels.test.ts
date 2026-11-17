// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import enWorkspace from '../src/locales/en/workspace.json'
import frWorkspace from '../src/locales/fr/workspace.json'
import enRuns from '../src/locales/en/runs.json'
import frRuns from '../src/locales/fr/runs.json'

// The interface's buttons and messages speak the person's language: the places
// below once wrote their Cancel / Save buttons, an error title, a confirmation
// and a preview badge as literal English or French. Each now reads a key that
// every language translates. The check is precise to these files.
const src = (rel: string) => readFileSync(join(import.meta.dir, '..', 'src', rel), 'utf8')

// A literal label rendered as JSX text, a button child, a tooltip label or a
// string/template literal.
const LITERAL = [
  />\s*Cancel\s*</,
  />\s*Save\s*</,
  /label="Cancel"/,
  /['"`]Action failed/,
  /['"`]À venir['"`]/,
  /`Cancel \$\{/,
]

const FILES = [
  'components/document/DocumentView.tsx',
  'components/compositions/DeckSettingsPanel.tsx',
  'components/document/SaveToWorkspace.tsx',
  'sections/marketplace/ConfigFileEditor.tsx',
  'sections/shell/LiveSessionList.tsx',
  'sections/team/AgentNode.tsx',
  'sections/workspace/ActionsPanel.tsx',
]

describe('no hardcoded button or message label', () => {
  for (const file of FILES) {
    test(file, () => {
      const text = src(file)
      for (const re of LITERAL) expect({ file, found: re.exec(text)?.[0] }).toEqual({ file, found: undefined })
    })
  }
})

const at = (texts: Record<string, unknown>, key: string) =>
  key.split('.').reduce<unknown>((o, k) => (o as Record<string, unknown> | undefined)?.[k], texts) as string | undefined

describe('the keys they read are translated', () => {
  test('workspace: common and the actions panel', () => {
    for (const key of ['common.cancel', 'common.save', 'actionsPanel.failed', 'actionsPanel.preview', 'actionsPanel.previewBody', 'actionsPanel.unavailable']) {
      expect(at(enWorkspace, key)).toBeTruthy()
      expect(at(frWorkspace, key)).toBeTruthy()
    }
    expect(at(enWorkspace, 'actionsPanel.preview')).not.toBe(at(frWorkspace, 'actionsPanel.preview'))
  })
  test('runs: cancelling a team child', () => {
    expect(at(enRuns, 'team.cancelChild')).toContain('{{label}}')
    expect(at(frRuns, 'team.cancelChild')).toContain('{{label}}')
    expect(at(enRuns, 'team.cancelChildLabel')).toContain('{{label}}')
    expect(at(frRuns, 'team.cancelChildTip')).toBeTruthy()
  })
  test('a page confirmation reuses common.cancel: no duplicate key', () => {
    expect(at(enWorkspace, 'pageConfirm.cancel')).toBeUndefined()
    expect(at(frWorkspace, 'pageConfirm.cancel')).toBeUndefined()
  })
})
