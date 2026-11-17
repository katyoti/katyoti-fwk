// SPDX-License-Identifier: Apache-2.0
/**
 * The Object tab's back history — where the person was before an object
 * replaced what the tab showed.
 *
 * There is ONE Object tab per universe; opening an object swaps its payload.
 * So that opening an item from a panel (a module's Planification panel, the
 * Inbox, a Global panel) never loses that panel, each open records a step on
 * the new payload: the payload the tab showed (its object, its panel, its
 * query) and the tab that was active. Closing the tab (or its back control)
 * takes the last step: the tab shows that payload again and the tab the
 * person came from is focused — or, when the Object tab was not open before,
 * it closes and that tab is focused. Kept free of the stores so it is tested
 * on its own (tests/object-tab-back.test.ts).
 */

/** The fields of an Object tab payload this history reads and writes. */
export interface BackablePayload {
  objectId: string
  objectTitle?: string
  activeKind?: string
  panelQuery?: { panel: string; query: Record<string, unknown> }
  back?: ObjectTabStep[]
}

/** One step back: what the Object tab showed (absent: it was not open) and
 *  the tab that was active when the next object was opened. */
export interface ObjectTabStep {
  payload?: BackablePayload
  label?: string
  from: string
}

/** The longest history kept; older steps are dropped first. */
export const MAX_BACK = 20

/** The tab strip as the history needs it. */
export interface StripState {
  openTabs: Array<{ id: string; label: string; payload?: unknown }>
  activeTabId: string
}

/**
 * The payload the Object tab takes when `next` is opened from `strip`: `next`
 * with the step back to what was shown. Re-opening the object the tab already
 * shows, from the tab itself and on no other panel, records no step.
 */
export function openedObjectPayload<P extends BackablePayload>(next: P, strip: StripState, tabId = 'object'): P {
  const cur = strip.openTabs.find((t) => t.id === tabId)
  if (!cur) return { ...next, back: [{ from: strip.activeTabId }] }
  const shown = (cur.payload ?? undefined) as BackablePayload | undefined
  const history = shown?.back ?? []
  if (shown && strip.activeTabId === tabId && shown.objectId === next.objectId
    && (next.activeKind === undefined || next.activeKind === shown.activeKind)) {
    return { ...next, activeKind: shown.activeKind, panelQuery: next.panelQuery ?? shown.panelQuery, back: history }
  }
  const step: ObjectTabStep = { from: strip.activeTabId, label: cur.label, ...(shown ? { payload: withoutBack(shown) } : {}) }
  return { ...next, back: [...history, step].slice(-MAX_BACK) }
}

/** What closing the Object tab does. */
export type BackMove =
  /** Nothing to go back to: the tab closes as any tab does. */
  | { kind: 'close' }
  /** The tab was not open before: it closes and `activate` is focused. */
  | { kind: 'close-to'; activate: string }
  /** The tab shows `payload` again (titled `label`) and `activate` is focused. */
  | { kind: 'restore'; payload: BackablePayload; label?: string; activate: string }

/** The move closing the Object tab makes: its last step back, if any. */
export function backMove(strip: StripState, tabId = 'object'): BackMove {
  const cur = strip.openTabs.find((t) => t.id === tabId)
  const history = ((cur?.payload ?? {}) as BackablePayload).back ?? []
  const last = history[history.length - 1]
  if (!cur || !last) return { kind: 'close' }
  const open = (id: string) => strip.openTabs.some((t) => t.id === id)
  if (!last.payload) return open(last.from) && last.from !== tabId ? { kind: 'close-to', activate: last.from } : { kind: 'close' }
  return {
    kind: 'restore',
    payload: { ...last.payload, back: history.slice(0, -1) },
    label: last.label,
    activate: open(last.from) ? last.from : tabId,
  }
}

/** The step back's label, for the back control: the previous object's title
 *  or the tab the person came from — empty when there is none. */
export function backTarget(strip: StripState, tabId = 'object'): { from: string; label?: string } | null {
  const cur = strip.openTabs.find((t) => t.id === tabId)
  const history = ((cur?.payload ?? {}) as BackablePayload).back ?? []
  const last = history[history.length - 1]
  if (!last) return null
  if (last.from !== tabId) return { from: last.from, label: strip.openTabs.find((t) => t.id === last.from)?.label }
  return { from: last.from, label: last.payload?.objectTitle ?? last.label }
}

function withoutBack(p: BackablePayload): BackablePayload {
  const { back: _back, ...rest } = p
  return rest
}

/** One crumb of the breadcrumb above an object opened from another object:
 *  a holder the person came
 *  through, its title, the tab it showed, and its step in the history. */
export interface ObjectCrumb {
  objectId: string
  title: string
  activeKind?: string
  step: number
}

/** The breadcrumb leading back from the object a payload shows: the objects
 *  the person opened it through, oldest first - the unbroken run of steps at
 *  the end of its history that showed an object in this same tab. Empty when
 *  the object was opened from elsewhere (a Global panel, the Inbox). */
export function crumbsOf(payload: BackablePayload | undefined, tabId = 'object'): ObjectCrumb[] {
  const history = payload?.back ?? []
  const out: ObjectCrumb[] = []
  for (let i = history.length - 1; i >= 0; i--) {
    const s = history[i]
    if (s.from !== tabId || !s.payload?.objectId) break
    out.unshift({
      objectId: s.payload.objectId, title: s.payload.objectTitle || s.label || s.payload.objectId,
      ...(s.payload.activeKind ? { activeKind: s.payload.activeKind } : {}), step: i,
    })
  }
  return out
}

/** What a click on a crumb restores: the holder's payload as it was shown,
 *  on its tab, its history cut at that step. */
export function crumbPayload(payload: BackablePayload, step: number): BackablePayload | undefined {
  const history = payload.back ?? []
  const s = history[step]
  if (!s?.payload) return undefined
  return { ...s.payload, back: history.slice(0, step) }
}
