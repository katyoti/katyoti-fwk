// SPDX-License-Identifier: Apache-2.0
/**
 * StateBadge — a state or status as a pill, the same in every view: light,
 * small, its colour from the view's value-to-tone map (gray when the value is
 * not in it), an optional spinner while it is in flight.
 */

import type { ReactNode } from 'react'
import { Badge, Loader } from '@mantine/core'

export interface StateBadgeProps {
  value: string
  /** value → Mantine colour name. */
  tones?: Record<string, string>
  /** What is shown; the value when not given. */
  label?: ReactNode
  /** A spinner on the left (in flight). */
  loading?: boolean
  size?: 'xs' | 'sm' | 'md'
}

export function stateTone(value: string, tones?: Record<string, string>): string {
  return tones?.[value] ?? 'gray'
}

export function StateBadge({ value, tones, label, loading, size = 'xs' }: StateBadgeProps) {
  const c = stateTone(value, tones)
  return (
    <Badge size={size} color={c} variant="light" leftSection={loading ? <Loader size={8} color={c} /> : undefined}>
      {label ?? value}
    </Badge>
  )
}
