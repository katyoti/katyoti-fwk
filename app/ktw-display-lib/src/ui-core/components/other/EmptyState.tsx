// SPDX-License-Identifier: Apache-2.0
/**
 * EmptyState — what a view shows when it has nothing: a dimmed icon, a
 * message, an optional hint below, centred. The same in every view.
 */

import type { ReactNode } from 'react'
import { Stack, Text } from '@mantine/core'
import { tablerIcon } from '../icons/index.ts'
import { IconFile } from '@tabler/icons-react'

export interface EmptyStateProps {
  /** A Tabler icon name (kebab); none for a plain line. */
  icon?: string
  /** The icon's colour: dimmed by default. */
  color?: string
  /** The icon size (40 by default). */
  iconSize?: number
  message: ReactNode
  hint?: ReactNode
}

export function EmptyState({ icon, color, iconSize = 40, message, hint }: EmptyStateProps) {
  const Icon = icon ? tablerIcon(icon, IconFile) : undefined
  return (
    <Stack align="center" gap={Icon ? 'md' : 'xs'} py="xl">
      {Icon && <Icon size={iconSize} stroke={1.2} color={color ? `var(--mantine-color-${color}-6)` : 'var(--mantine-color-dimmed)'} />}
      <Text c="dimmed" size="sm" ta="center">{message}</Text>
      {hint && <Text c="dimmed" size="xs" ta="center">{hint}</Text>}
    </Stack>
  )
}
