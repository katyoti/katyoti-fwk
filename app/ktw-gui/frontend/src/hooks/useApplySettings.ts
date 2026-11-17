// SPDX-License-Identifier: Apache-2.0
import { useEffect } from 'react'
import { useMantineColorScheme } from '@mantine/core'
import { useAppStore } from '../store'

/**
 * Applies AppSettings to Mantine + DOM CSS variables.
 * Must be called inside MantineProvider.
 */
export function useApplySettings() {
  const { themeMode, fontSize, fontFamily, monoFamily } = useAppStore(
    (s) => s.appSettings.appearance,
  )
  const { setColorScheme } = useMantineColorScheme()

  useEffect(() => {
    setColorScheme(themeMode === 'system' ? 'auto' : themeMode)
  }, [themeMode, setColorScheme])

  useEffect(() => {
    document.documentElement.style.setProperty(
      '--mantine-font-family',
      `'${fontFamily}', system-ui, -apple-system, sans-serif`,
    )
    document.documentElement.style.setProperty(
      '--mantine-font-family-monospace',
      `'${monoFamily}', ui-monospace, 'Cascadia Code', 'Fira Code', monospace`,
    )
  }, [fontFamily, monoFamily])

  useEffect(() => {
    // Mantine uses rem units, so scaling :root font-size scales all components
    document.documentElement.style.fontSize = `${fontSize}px`
  }, [fontSize])
}
