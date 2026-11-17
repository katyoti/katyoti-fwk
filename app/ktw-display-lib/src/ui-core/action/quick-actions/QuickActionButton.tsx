// SPDX-License-Identifier: Apache-2.0
// QuickActionButton — single styled action button (icon + label +
// description). Used as a primary-action card in dashboards / landing
// pages.
//
// Ported from _ktw_workspace/.../cpm-core/actions/quick-actions/QuickActionButton.tsx
// during gui-action Subtask 2 (2026-05-04). Adaptations:
//   - `@core/cpm-core/icons` → `'../../components/icons/index.ts'`
//   - `@core/cpm-core/texts` → drop; UITexts is a prop
//   - The companion CSS file from old code (`QuickActionButton.css`) is
//     dropped — old class hooks weren't used anywhere outside the component
//     and the inline styles already give the visual we want.

import { Group, luminance, Paper, Stack, Text, UnstyledButton, useMantineTheme } from '@mantine/core'
import type { FC } from 'react'

import { Icon } from '../../components/icons/index.ts'
import type { UITexts } from '../../i18n/index.ts'
import type { UIAction } from '../types.ts'
import { getattributs } from '../utils.tsx'
import { surfaceBg } from '../../../ui-builders/page/nodes/surface.ts'

interface QuickActionButtonProps {
  action: UIAction
  onClick: () => void
  texts: UITexts
  disabled?: boolean
  running?: boolean
}

export const QuickActionButton: FC<QuickActionButtonProps> = ({
  action,
  onClick,
  disabled = false,
  running = false,
  texts,
}) => {
  const theme = useMantineTheme()
  const { label, icon, color: inputColor = 'gray', description } = getattributs({ action } as Parameters<typeof getattributs>[0], texts)

  const colorName = inputColor in theme.colors ? inputColor : 'gray'
  const colors = theme.colors[colorName as keyof typeof theme.colors]
  // The icon tile follows the colour scheme: its light
  // shade in the light scheme, the colour's dark tint in the dark one - never
  // a light square on the dark theme. surfaceBg leaves a shade it does not
  // turn (the `dark` palette's) as given: its value is then taken as is.
  const tileBg = (shade: string, fallback: string) => {
    const bg = surfaceBg(shade)
    return bg === shade ? fallback : bg
  }
  // The `dark` palette's shade 0 is a light grey surfaceBg leaves alone: its
  // tile takes the palette's dark tint in the dark scheme.
  const enabledBg = colorName === 'dark'
    ? 'light-dark(var(--mantine-color-dark-0), var(--mantine-color-dark-light))'
    : tileBg(`${colorName}.0`, colors[0])
  // The icon keeps 3:1 on its tile in both schemes: in the
  // light scheme the first shade from 6 that reaches it on the tile's shade 0
  // (the black when none does, as yellow), in the dark one the colour's
  // `-light-color` - the `dark` palette's lightest shade for it.
  const ratio = (a: string, b: string) => {
    const [x, y] = [luminance(a), luminance(b)]
    return (Math.max(x, y) + 0.05) / (Math.min(x, y) + 0.05)
  }
  const lightIcon = [6, 7, 8, 9].map((k) => colors[k]).find((c) => ratio(c, colors[0]) >= 3) ?? theme.black
  const darkIcon = colorName === 'dark' ? 'var(--mantine-color-dark-0)' : `var(--mantine-color-${colorName}-light-color)`
  const enabledIcon = `light-dark(${lightIcon}, ${darkIcon})`

  return (
    <UnstyledButton onClick={onClick} disabled={disabled || running}>
      <Paper
        p="md"
        withBorder
        style={{
          borderColor: disabled ? theme.colors.gray[3] : colors[3],
          cursor: disabled ? 'not-allowed' : 'pointer',
          height: '100%',
        }}
      >
        <Stack gap="xs" align="center">
          <Group
            justify="center"
            style={{
              width: 48,
              height: 48,
              borderRadius: theme.radius.md,
              backgroundColor: disabled || running ? tileBg('gray.1', theme.colors.gray[1]) : enabledBg,
              color: disabled || running ? theme.colors.gray[5] : enabledIcon,
            }}
          >
            {icon ? <Icon icon={icon} size={24} /> : null}
          </Group>

          <Stack gap={0} align="center">
            <Text size="sm" fw={600} ta="center" c={disabled || running ? 'dimmed' : 'inherit'}>
              {label}
            </Text>

            {description && (
              <Text size="xs" c="dimmed" ta="center" lineClamp={2} style={{ minHeight: '2.5em' }}>
                {description}
              </Text>
            )}
          </Stack>
        </Stack>
      </Paper>
    </UnstyledButton>
  )
}
