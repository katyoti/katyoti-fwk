// SPDX-License-Identifier: Apache-2.0
/**
 * The rows of the debug view's « Interrupteurs d'affichage »: one
 * per increment display switch - its variable, its
 * increment, what it changes, on or off as this GUI started - then the dev
 * front-end's list. Nothing is listed by hand: the switches are the features
 * CWM tags display-switch, each one's increment the increment taking it, its
 * variable the backend's derivation from that increment (DisplaySwitchStates).
 * A retired switch is no row: retiring it takes the tag off its feature;
 * a switch whose feature is not built yet is « pas
 * encore construit », never « éteint » - no code reads its variable yet.
 */

/** A feature tagged display-switch, as cwm object-search answers it. */
export interface SwitchFeature {
  id: string
  name?: string
  /** Its lifecycle state (realized or done: the switch is built). */
  state?: string
}

/** Whether a switch feature's state says it is built: some code reads its variable. */
export function switchBuilt(state: string | undefined): boolean {
  return state === 'realized' || state === 'done'
}

/** One increment's switch as the backend read it at this start. */
export interface SwitchState {
  increment: string
  variable: string
  on: boolean
}

/** A row of the table. */
export interface SwitchRow {
  /** The variable that turns it on. */
  variable: string
  /** The increment (an object id), empty for the dev front-end's row. */
  increment: string
  /** What it changes, in one line. */
  changes: string
  /** Its feature's full name, the line's title; empty for the dev row. */
  name: string
  /** On as this start read it. */
  on: boolean
  /** Built: some code reads its variable (the dev row always). */
  built: boolean
  /** The feature that is the switch, empty for the dev row. */
  feature: string
  /** The dev front-end's row (VITE_KTW_DISPLAY_SWITCHES). */
  dev?: boolean
  /** For the dev row: the increments it lists. */
  listed?: string[]
}

/** The dev front-end's list of switches on (devSwitch.ts). */
export const DEV_SWITCHES_VARIABLE = 'VITE_KTW_DISPLAY_SWITCHES'

/** The longest line a switch's « what it changes » keeps before its ellipsis. */
export const CHANGE_LINE_MAX = 100

/**
 * What a switch changes, in one line: its feature's name up to « stay(s)
 * behind … » - « One source selector's source-model screen changes (…) » -
 * and up to its first colon or semicolon outside parentheses (« The study's
 * pane holds the whole as-is study: a header … » keeps its head), capped at
 * CHANGE_LINE_MAX characters with an ellipsis at a word's end. The table
 * gives the full name as the cell's title.
 */
export function changeLine(name: string | undefined): string {
  const s = (name ?? '').replace(/\s+/g, ' ').trim()
  let line = s.replace(/[\s,;:-]*\bstays? behind\b.*$/i, '').trim() || s
  let depth = 0
  for (let i = 0; i < line.length; i++) {
    const c = line[i]
    if (c === '(') depth++
    else if (c === ')') depth = Math.max(0, depth - 1)
    else if ((c === ':' || c === ';') && depth === 0 && line.slice(0, i).trim()) {
      line = line.slice(0, i).trim()
      break
    }
  }
  if (line.length <= CHANGE_LINE_MAX) return line
  const head = line.slice(0, CHANGE_LINE_MAX - 1)
  const space = head.lastIndexOf(' ')
  return (space > CHANGE_LINE_MAX / 2 ? head.slice(0, space) : head).replace(/[\s,;:(-]+$/, '') + '…'
}

/** An increment's short name: `IN-n` for `<module>.IN-n`. */
export function incrementLabel(id: string): string {
  const i = id.lastIndexOf('.')
  return i < 0 ? id : id.slice(i + 1)
}

/** The increment that takes a feature, among the objects taking it. */
export function incrementOf(takers: string[]): string {
  return takers.find((t) => /(^|\.)IN-\d+$/.test(t)) ?? ''
}

/**
 * The table's rows: one per switch feature, sorted by variable (a switch whose
 * increment is unknown last), then the dev front-end's row.
 */
export function switchRows(
  features: SwitchFeature[],
  incrementByFeature: Record<string, string>,
  states: SwitchState[],
  dev: { enabled: boolean; value: string },
): SwitchRow[] {
  const byInc = new Map(states.map((s) => [s.increment, s]))
  const rows: SwitchRow[] = features.map((f) => {
    const inc = incrementByFeature[f.id] ?? ''
    const st = inc ? byInc.get(inc) : undefined
    const built = switchBuilt(f.state)
    return { variable: st?.variable ?? '', increment: inc, changes: changeLine(f.name), name: (f.name ?? '').replace(/\s+/g, ' ').trim(), on: built && st?.on === true, built, feature: f.id }
  })
  rows.sort((a, b) => (a.variable === '') !== (b.variable === '')
    ? (a.variable === '' ? 1 : -1)
    : a.variable.localeCompare(b.variable) || a.feature.localeCompare(b.feature))
  const listed = dev.enabled
    ? dev.value.split(',').map((s) => s.trim().toLowerCase()).filter(Boolean)
    : []
  rows.push({ variable: DEV_SWITCHES_VARIABLE, increment: '', changes: '', name: '', on: listed.length > 0, built: true, feature: '', dev: true, listed })
  return rows
}
