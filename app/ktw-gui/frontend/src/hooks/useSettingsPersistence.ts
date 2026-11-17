// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef } from 'react'
import { useAppStore } from '../store'
import { mergeLoadedSettings } from '../types/settings'
import type { AppSettings } from '../types/settings'

/**
 * Persists app settings to disk via the Wails-bound App.GetGuiSettings /
 * SaveGuiSettings methods. Writes go to ~/.config/ktw-agents/config/GuiSettings.yaml.
 *
 * Boot sequence:
 *   1. Mount → call GetGuiSettings() → hydrate store with the result.
 *   2. Subscribe to appSettings changes → debounced save (300ms) to disk.
 *
 * If Wails bindings are unavailable (mock mode, dev without Go backend),
 * persistence is a no-op — the store still works in-memory.
 */
export function useSettingsPersistence() {
  const updateSettings = useAppStore((s) => s.updateSettings)
  const hydrated = useRef(false)
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  // ── 1. hydrate on boot ──────────────────────────────────────────────
  useEffect(() => {
    if (hydrated.current) return
    hydrated.current = true

    const win = window as unknown as {
      go?: { main?: { App?: { GetGuiSettings?: () => Promise<AppSettings> } } }
    }
    const fn = win.go?.main?.App?.GetGuiSettings
    if (!fn) {
      // Dev / mock mode without Wails — skip; store keeps DEFAULT_SETTINGS.
      return
    }
    fn()
      .then((loaded) => {
        // Merge loaded with defaults to fill any missing fields safely.
        const merged = mergeLoadedSettings(loaded)
        // Apply each section so the existing updateSettings reducer is reused.
        ;(Object.entries(merged) as [keyof AppSettings, AppSettings[keyof AppSettings]][])
          .forEach(([section, value]) => updateSettings(section, value as never))
      })
      .catch((err) => {
        // Non-fatal — keep defaults; log for diagnostics.
        // eslint-disable-next-line no-console
        console.warn('[settings] GetGuiSettings failed:', err)
      })
  }, [updateSettings])

  // ── 2. persist on change (debounced) ────────────────────────────────
  useEffect(() => {
    const unsub = useAppStore.subscribe((state, prev) => {
      if (state.appSettings === prev.appSettings) return
      if (!hydrated.current) return // don't write before initial hydrate completes

      if (saveTimer.current) clearTimeout(saveTimer.current)
      saveTimer.current = setTimeout(() => {
        const win = window as unknown as {
          go?: { main?: { App?: { SaveGuiSettings?: (s: AppSettings) => Promise<string> } } }
        }
        const fn = win.go?.main?.App?.SaveGuiSettings
        if (!fn) return
        fn(state.appSettings)
          .then((errMsg) => {
            if (errMsg) {
              // eslint-disable-next-line no-console
              console.warn('[settings] SaveGuiSettings:', errMsg)
            }
          })
          .catch((err) => {
            // eslint-disable-next-line no-console
            console.warn('[settings] SaveGuiSettings failed:', err)
          })
      }, 300)
    })
    return () => {
      unsub()
      if (saveTimer.current) clearTimeout(saveTimer.current)
    }
  }, [])
}
