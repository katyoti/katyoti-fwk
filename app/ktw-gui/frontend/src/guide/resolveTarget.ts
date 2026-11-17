// SPDX-License-Identifier: Apache-2.0
/**
 * Target resolver — finds the control a recipe step points at by its anchor
 * only (modules/guide, OD-007): `[data-guide="<guide>"]`, and
 * `[data-guide-name="<name>"]` for an instance in a list. Never by role or
 * visible label: those are GUI text, and the GUI is translated. The anchors
 * are listed in ./anchors.ts. Returns null when the control is not on screen:
 * the player then plays the step as manual, without a highlight, and reports
 * the missing control.
 */

import type { Recipe } from '../generated/guide-recipe'

export type StepTarget = NonNullable<NonNullable<Recipe['steps']>[number]['target']>

/** True when the element occupies space on screen and is not hidden. */
export function isVisible(el: Element): boolean {
  const r = el.getBoundingClientRect()
  if (r.width === 0 && r.height === 0) return false
  const s = window.getComputedStyle(el)
  return s.display !== 'none' && s.visibility !== 'hidden' && s.opacity !== '0'
}

function escapeAttr(v: string): string {
  return v.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
}

/** Resolve a step's target to the first visible element carrying its anchor.
 *  `exclude` keeps the player's own overlay out of the search. */
export function resolveTarget(target: StepTarget | undefined, exclude?: Element | null): HTMLElement | null {
  if (!target?.guide) return null
  let sel = `[data-guide="${escapeAttr(target.guide)}"]`
  if (target.name) sel += `[data-guide-name="${escapeAttr(target.name)}"]`
  for (const el of Array.from(document.querySelectorAll(sel))) {
    if (exclude && exclude.contains(el)) continue
    if (isVisible(el)) return el as HTMLElement
  }
  return null
}
