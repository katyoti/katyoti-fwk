// SPDX-License-Identifier: Apache-2.0
import { describe, expect, test } from 'bun:test'
import { readFileSync } from 'node:fs'

// « Nouvelle étude v1 » opens its task's entry form in the
// main area (RunTaskPanel's FormWidgetPanel - taskForm.ts), and
// the study walks waited for a modal that never came. The form's root carries
// the element id task.form, and the walks find the form through one helper,
// tests/live/task-form.js, whether it shows in the main area or in a dialog.
const helper = readFileSync(new URL('./live/task-form.js', import.meta.url), 'utf8')

type El = { id: string }
function install(found: Record<string, El | null>) {
  const w: Record<string, unknown> = {}
  const doc = { querySelector: (sel: string) => found[sel] ?? null }
  new Function('window', 'document', helper)(w, doc)
  return w.__taskForm as { shown(): El | null; open(entry: () => { click(): void } | undefined, tries?: number): Promise<El | null>; submit(root?: unknown): unknown }
}

describe('the walks find a task entry form wherever it shows', () => {
  test('the main area form carries the element id the helper reads', () => {
    const src = readFileSync(new URL('../src/sections/agent/FormWidgetPanel.tsx', import.meta.url), 'utf8')
    expect(src).toContain('data-testid="task.form"')
  })

  test('the main area form first, a dialog otherwise, null when none shows', () => {
    const page = { id: 'page' }, dialog = { id: 'dialog' }
    expect(install({ '[data-testid="task.form"]': page, '[role="dialog"]': dialog }).shown()).toBe(page)
    expect(install({ '[role="dialog"]': dialog }).shown()).toBe(dialog)
    expect(install({}).shown()).toBeNull()
  })

  test('open clicks the entry until the form shows in the main area', async () => {
    const page = { id: 'page' }
    const found: Record<string, El | null> = {}
    let clicks = 0
    const form = await install(found).open(() => ({ click: () => { clicks++; found['[data-testid="task.form"]'] = page } }))
    expect(form).toBe(page)
    expect(clicks).toBe(1)
  })

  // The main area form's submit is « Lancer » (RunTaskPanel's
  // [Cancel, Run]), whatever the task's DSL names it: a walk submits the form
  // by its type=submit button, never by a label.
  test('submit clicks the form type=submit button, null when it has none', () => {
    let clicked = 0
    const btn = { click: () => { clicked++ } }
    const root = { querySelector: (sel: string) => (sel === 'button[type="submit"]' ? btn : null) }
    const f = install({})
    expect(f.submit(root)).toBe(btn)
    expect(clicked).toBe(1)
    expect(f.submit({ querySelector: () => null })).toBeNull()
  })

  test('the study walks submit the entry form through the helper, never by a label', () => {
    for (const w of ['study-page.probe.js', 'study-run.probe.js']) {
      const src = readFileSync(new URL('./live/' + w, import.meta.url), 'utf8')
      expect(src, w).toContain('window.__taskForm.submit(modal)')
      expect(src, w).not.toContain(`button(modal, "Créer l'étude")`)
      expect(src, w).not.toContain(`buttons(modal, "Créer l'étude")`)
    }
    const one = readFileSync(new URL('./live/study-create-one-form.sh', import.meta.url), 'utf8')
    expect(one).not.toMatch(/submitted=.*Créer l/)
  })

  test('the walks that open « Nouvelle étude v1 » wait for no modal', () => {
    const walks = ['study-page.probe.js', 'study-run.probe.js']
    for (const w of walks) {
      const src = readFileSync(new URL('./live/' + w, import.meta.url), 'utf8')
      expect(src, w).toContain('window.__taskForm.open(')
      expect(src, w).not.toMatch(/modal\s*=\s*await until\(\(\) ?=> ?document\.querySelector\('\[role=\\?"dialog\\?"\]'\)/)
    }
  })
})
