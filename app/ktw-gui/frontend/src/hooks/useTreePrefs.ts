// SPDX-License-Identifier: Apache-2.0
import { useAppStore } from '../store'
import { DEFAULT_SETTINGS } from '../types/settings'

/**
 * The person's projects-tree settings, read from and saved to
 * the GUI settings (GuiSettings.yaml, saved by useSettingsPersistence), so
 * they come back at the next start.
 */
export function useTreePrefs() {
  const tree = useAppStore((s) => s.appSettings.tree) ?? DEFAULT_SETTINGS.tree
  const updateSettings = useAppStore((s) => s.updateSettings)
  return {
    settings: tree,
    showTypes: !tree.hideTypes,
    setShowTypes: (show: boolean) => updateSettings('tree', { hideTypes: !show }),
  }
}
