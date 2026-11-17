// SPDX-License-Identifier: Apache-2.0
import { useAppStore } from '../store'
import { removeQuick, starTask, type SortKey } from '../sections/task/listPrefs'

/** The lists whose order the person picks. */
export type PrefList = 'tasks' | 'skills'

/**
 * The person's choices for the task and skill lists, read from and saved to
 * the GUI settings (GuiSettings.yaml, saved by useSettingsPersistence): the
 * tasks starred in this universe, the quick actions removed from it, and each
 * list's order.
 */
export function useListPrefs(universe: string | null | undefined) {
  const prefs = useAppStore((s) => s.appSettings.universes)
  const updateSettings = useAppStore((s) => s.updateSettings)
  const u = universe ?? ''
  const starred = prefs.starred?.[u] ?? []
  const removed = prefs.removed?.[u] ?? []
  const save = (next: { starred: string[]; removed: string[] }) =>
    updateSettings('universes', {
      starred: { ...(prefs.starred ?? {}), [u]: next.starred },
      removed: { ...(prefs.removed ?? {}), [u]: next.removed },
    })
  return {
    starred,
    removed,
    toggleStar: (uri: string) => {
      if (u) save(starTask({ starred, removed }, uri))
    },
    /** A ✕ on a quick action: it stays removed for the person. */
    removeQuick: (id: string) => {
      if (u) save(removeQuick({ starred, removed }, id))
    },
    sortOf: (list: PrefList): SortKey => (prefs.sort?.[list] as SortKey) || 'name',
    setSort: (list: PrefList, key: SortKey) =>
      updateSettings('universes', { sort: { ...(prefs.sort ?? {}), [list]: key } }),
  }
}
