// SPDX-License-Identifier: Apache-2.0
/**
 * Controls named in a recipe's text (modules/guide, OD-007). A step never
 * writes a control's label (GUI text, translated): it writes
 * `{control:<anchor>}`, or `{control:<anchor>|<name>}` for an instance in a
 * list, and the player says the label the GUI shows for that control, in
 * bold, in the GUI's language. One rule picks the label:
 *
 *   1. the control is on screen and carries `data-guide-label` → that text
 *      (what it shows now: a contribution's translated name, a toggle's
 *      current word);
 *   2. else the anchor's `label` key in the registry (./anchors.ts),
 *      translated now (`{name}` in the key is the instance);
 *   3. else the instance name, else the anchor id (their last segment:
 *      `ktw-demo:signoff/approve` → `approve`).
 *
 * Pure: the DOM and the translation are passed in, so it is unit tested
 * (tests/guide-controls.test.ts).
 */
import { GUIDE_ANCHORS, type GuideAnchor } from './anchors'

/** A `{control:…}` placeholder: the anchor, and the instance after `|`. */
export const CONTROL_PLACEHOLDER = /\{control:([a-z0-9][a-z0-9.:-]*)(?:\|([^{}|]+))?\}/g

export interface ControlLabelSources {
  /** The `data-guide-label` of the control on screen, or null. */
  live: (anchor: string, name?: string) => string | null
  /** A registry key (`<namespace>:<key>`) in the GUI's language, or null when
   *  the key is missing. */
  translate: (nsKey: string) => string | null
  /** The registry; the GUI's own by default. */
  anchors?: Record<string, GuideAnchor>
}

/** The label a placeholder says, by the rule above. */
export function controlLabel(anchor: string, name: string | undefined, src: ControlLabelSources): string {
  const live = src.live(anchor, name)?.trim()
  if (live) return live
  const key = (src.anchors ?? GUIDE_ANCHORS)[anchor]?.label
  if (key && (name || !key.includes('{name}'))) {
    const said = src.translate(key.replace('{name}', name ?? ''))
    if (said) return said
  }
  return lastSegment(name || anchor)
}

function lastSegment(s: string): string {
  const parts = s.split(/[/.:]/).filter(Boolean)
  return parts.length ? parts[parts.length - 1] : s
}

function escapeMarkdown(s: string): string {
  return s.replace(/([\\*_`[\]])/g, '\\$1')
}

/** The text with every placeholder replaced by its label in bold. */
export function sayControls(text: string, src: ControlLabelSources): string {
  return text.replace(CONTROL_PLACEHOLDER, (_m, anchor: string, name?: string) =>
    `**${escapeMarkdown(controlLabel(anchor, name?.trim() || undefined, src))}**`)
}

function escapeAttr(v: string): string {
  return v.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
}

/** The `data-guide-label` of the first control on screen carrying the anchor
 *  (and the instance), outside `exclude`. */
export function liveControlLabel(anchor: string, name?: string, exclude?: Element | null): string | null {
  if (typeof document === 'undefined') return null
  let sel = `[data-guide="${escapeAttr(anchor)}"]`
  if (name) sel += `[data-guide-name="${escapeAttr(name)}"]`
  for (const el of Array.from(document.querySelectorAll(sel))) {
    if (exclude && exclude.contains(el)) continue
    const label = el.getAttribute('data-guide-label')
    if (label) return label
  }
  return null
}
