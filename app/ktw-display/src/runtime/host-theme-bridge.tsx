// SPDX-License-Identifier: Apache-2.0
// HostThemeBridge — subscribes to the host's `theme` postMessage and
// applies the received theme tokens + colorScheme to its children's
// MantineProvider. This makes the iframe's visual style mirror the
// host (dark/light, fonts, colors, radii) instead of being stuck on
// Mantine defaults.
//
// Wired by display-dsl-framework S9 polish (2026-05-05) — replaces the
// previous unconditional `<MantineProvider>` mount in `runtime/index.tsx`.
// Without this bridge, the iframe always rendered light theme + standard
// fonts even when the host was in dark mode.
//
// Embedded mode tweak: when `compact: true`, the bridge applies a
// reduced-density font/spacing scale on top of received tokens — useful
// for tab-embedded artifacts where the host's chrome already provides
// the title bar and margins.

import { MantineProvider, createTheme } from '@mantine/core'
import type { MantineColorScheme, MantineThemeOverride } from '@mantine/core'
import { useEffect, useState, type ReactNode } from 'react'

import { listenToHost, type ThemeMessage } from '../postmessage.ts'

interface HostThemeBridgeProps {
  /** When true, apply a reduced-density font/spacing scale on top of
   *  whatever the host pushes. Use for embedded artifact tabs where
   *  Mantine defaults feel too big against the host's denser chrome. */
  compact?: boolean
  /** Initial color scheme used until the host pushes its first theme
   *  message. Default: 'light' to match Mantine's own default. */
  initialColorScheme?: MantineColorScheme
  children: ReactNode
}

export function HostThemeBridge({
  compact = false,
  initialColorScheme = 'light',
  children,
}: HostThemeBridgeProps) {
  const [colorScheme, setColorScheme] = useState<MantineColorScheme>(initialColorScheme)
  const [tokens, setTokens] = useState<NonNullable<ThemeMessage['tokens']> | null>(null)

  useEffect(() => {
    const unsub = listenToHost((msg) => {
      if (msg.type !== 'theme') return
      setColorScheme(msg.theme as MantineColorScheme)
      if (msg.tokens) setTokens(msg.tokens)
    })
    return unsub
  }, [])

  // Build the theme override — start from received tokens, then overlay
  // a compact-density scale when in embedded mode. We pass a partial
  // MantineThemeOverride directly to MantineProvider; it merges with
  // DEFAULT_THEME under the hood. No need to round-trip through the
  // full-theme `mergeMantineTheme` (which requires non-optional fields).
  const themeOverride: MantineThemeOverride = (() => {
    // Compact density values applied AFTER token-derived ones so they
    // win for the keys they set (fontSizes, spacing, headings sizes).
    const compactOverrides: Partial<MantineThemeOverride> = compact
      ? {
          fontSizes: {
            xs: '11px',
            sm: '12px',
            md: '13px',
            lg: '14px',
            xl: '16px',
          },
          spacing: {
            xs: '6px',
            sm: '8px',
            md: '12px',
            lg: '16px',
            xl: '24px',
          },
          headings: {
            sizes: {
              h1: { fontSize: '20px', fontWeight: '700', lineHeight: '1.3' },
              h2: { fontSize: '18px', fontWeight: '600', lineHeight: '1.35' },
              h3: { fontSize: '16px', fontWeight: '600', lineHeight: '1.4' },
              h4: { fontSize: '14px', fontWeight: '600', lineHeight: '1.4' },
              h5: { fontSize: '13px', fontWeight: '600', lineHeight: '1.4' },
              h6: { fontSize: '12px', fontWeight: '600', lineHeight: '1.4' },
            },
          },
        }
      : {}

    return createTheme({
      // Token-derived (host's choices) come first; compactOverrides win
      // for the same keys when in embedded mode.
      ...(tokens?.primaryColor ? { primaryColor: tokens.primaryColor } : {}),
      ...(tokens?.fontFamily ? { fontFamily: tokens.fontFamily } : {}),
      ...(tokens?.fontFamilyMonospace ? { fontFamilyMonospace: tokens.fontFamilyMonospace } : {}),
      ...(tokens?.defaultRadius ? { defaultRadius: tokens.defaultRadius } : {}),
      ...(tokens?.radius ? { radius: tokens.radius } : {}),
      ...(tokens?.fontSizes && !compact ? { fontSizes: tokens.fontSizes } : {}),
      ...(tokens?.spacing && !compact ? { spacing: tokens.spacing } : {}),
      ...(tokens?.headings && !compact ? { headings: tokens.headings } : {}),
      // Apply compact overrides when embedded
      ...compactOverrides,
    })
  })()

  return (
    <MantineProvider theme={themeOverride} forceColorScheme={colorScheme as 'light' | 'dark'}>
      {children}
    </MantineProvider>
  )
}
