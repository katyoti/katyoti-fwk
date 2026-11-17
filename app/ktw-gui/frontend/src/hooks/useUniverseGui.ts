// SPDX-License-Identifier: Apache-2.0
import { useCallback, useEffect } from 'react'
import { create } from 'zustand'
import { useAppStore } from '../store'
import { isLeftOut } from '../sections/shell/universeGui'
import { resolveView, type UniverseView } from '../ktsp/universeView'
import type { DisplayLevel } from '../types/settings'

const NONE: string[] = []

/** The resolved views read so far, by `<universe>|<level>`; null when the
 *  universe serves none (everything shows). */
const useViews = create<{ views: Record<string, UniverseView | null>; asked: Record<string, true> }>(() => ({ views: {}, asked: {} }))

/** The universe's resolved view at the person's level, read once
 *  per universe and level; undefined while it is read. */
export function useUniverseView(universe: string | null | undefined): UniverseView | null | undefined {
  const level = useDisplayLevel()
  const key = `${universe ?? ''}|${level}`
  const view = useViews((s) => s.views[key])
  useEffect(() => {
    if (!universe || useViews.getState().asked[key]) return
    useViews.setState((s) => ({ asked: { ...s.asked, [key]: true } }))
    void resolveView(universe, level).then((v) => useViews.setState((s) => ({ views: { ...s.views, [key]: v } })))
  }, [universe, level, key])
  return view
}

/**
 * What the universe's GUI shows at the person's display level (GuiSettings
 * `general.level`, once for all universes): its resolved view (its tags'
 * allow-lists), its Univers.yaml `gui` section (the hashtags
 * listed first; its `hide` is retired) and what the level
 * itself hides. At Expert an untagged universe shows everything.
 */
export function useUniverseGui(universe: string | null | undefined) {
  const gui = useAppStore((s) => s.universes.find((u) => u.name === universe)?.gui)
  const level = useDisplayLevel()
  const view = useUniverseView(universe)
  const expert = level === 'expert'
  const tagged = !!view && !view.all
  // A new function when the level or the view changes, and only then: a memo that filters by it lists it as a dependency.
  const hidden = useCallback((id: string) => isLeftOut(id, level, view), [level, view])
  return {
    gui,
    view,
    expert,
    /** Task hashtags listed first: the view's priority tags, else the universe's. */
    first: tagged && view!.first?.length ? view!.first : gui?.first ?? NONE,
    /** Whether an element is left out, by its element id (elements.ts): the
     *  level hides it, or the universe's view does not allow
     *  it - at every level in a universe with tags. */
    hidden,
  }
}

/** The person's display level; Essentiel when none was chosen yet. */
export function useDisplayLevel(): DisplayLevel {
  return useAppStore((s) => s.appSettings.general.level) ?? 'essential'
}

/** Sets the person's display level, for every universe, at once. */
export function useSetDisplayLevel() {
  const updateSettings = useAppStore((s) => s.updateSettings)
  return (level: DisplayLevel) => updateSettings('general', { level })
}
