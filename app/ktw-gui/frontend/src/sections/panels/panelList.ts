// SPDX-License-Identifier: Apache-2.0
// The panels screen's pure part: the list of open panels the
// approved display-panels mock-up draws on the left - each its title, its id
// and type, its state as a chip - and which one is selected, and the lines
// the selected panel shows under its work - the answer sent, how it ended.

/** One panel as the panel server lists it (ktw-hub:panel-service/list). */
export interface PanelSummary {
  panel: string
  seq?: number
  state?: string
  type?: string
  title?: string
  owner?: string
  waiting?: boolean
  updated?: string
}

/** A panel's state as the mock-up's chip says it. */
export type PanelChip = 'running' | 'asking' | 'complete'

/** One entry of the list. */
export interface PanelListItem {
  id: string
  title?: string
  /** `<id> · <type>`, the mock-up's mono line. */
  meta: string
  chip: PanelChip
}

/** panelTypeName is a panel-type URI's name: `ktw-agents:/panel-type/progress` → `progress`. */
export function panelTypeName(type?: string): string {
  return (type ?? '').replace(/^.*\/panel-type\//, '')
}

/** panelChip is the chip of a panel's state: asking while a question waits,
 *  complete once it ended, running otherwise. */
export function panelChip(state?: string, waiting?: boolean): PanelChip {
  if (state === 'complete' || state === 'closed') return 'complete'
  if (state === 'asking' || waiting) return 'asking'
  return 'running'
}

/** panelListItems is the list of open panels: every panel the server keeps
 *  that is not closed, in its order (the last changed first). */
export function panelListItems(panels: readonly PanelSummary[] | undefined): PanelListItem[] {
  return (panels ?? [])
    .filter((p) => p.panel && p.state !== 'closed')
    .map((p) => {
      const type = panelTypeName(p.type)
      return { id: p.panel, title: p.title, meta: type ? `${p.panel} · ${type}` : p.panel, chip: panelChip(p.state, p.waiting) }
    })
}

/** selectedPanel keeps the selection while its panel is listed; otherwise
 *  the first panel of the list, none when it is empty. */
export function selectedPanel(items: readonly PanelListItem[], current?: string): string | undefined {
  if (current && items.some((i) => i.id === current)) return current
  return items[0]?.id
}

/** answerLabel is what the person answered, as the panel's question offered
 *  it: the label of the action chosen, else the text written. */
export function answerLabel(actions: ReadonlyArray<{ name: string; label?: string }> | undefined, answer: { action?: string; text?: string }): string | undefined {
  if (actions && actions.length > 0) {
    const a = actions.find((x) => x.name === answer.action) ?? actions[0]
    return a.label ?? a.name
  }
  const t = answer.text?.trim()
  return t ? t : undefined
}

/** completionMessage is what the work said when it ended - its result's
 *  text or message (a method run answers the method's value) - for the
 *  mock-up's « ✓ <message> » line; undefined when it said nothing. */
export function completionMessage(complete?: { status: string; result?: unknown }): string | undefined {
  const r = complete?.result
  if (typeof r === 'string') return r.trim() || undefined
  if (r && typeof r === 'object' && !Array.isArray(r)) {
    const o = r as Record<string, unknown>
    const value = o.value && typeof o.value === 'object' ? (o.value as Record<string, unknown>) : undefined
    for (const v of [o.message, o.error, value?.message]) {
      if (typeof v === 'string' && v.trim()) return v.trim()
    }
  }
  return undefined
}
