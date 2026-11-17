// SPDX-License-Identifier: Apache-2.0
/**
 * ItemCard — one item of a collection, the same in every view: a bordered
 * card whose tone is a 3px left border (pulsing while the item is live), a
 * pointer when it opens. The page DSL's list and lane cards render through it
 * and the hand-written views use it — a request card is one card everywhere.
 */

import type { CSSProperties, ReactNode } from 'react'
import { Card, type CardProps } from '@mantine/core'

export interface ItemCardProps extends Omit<CardProps, 'children'> {
  /** A Mantine colour name: the left border. */
  tone?: string
  /** The border pulses (a running item). */
  pulse?: boolean
  onClick?: () => void
  children?: ReactNode
  /** Passed to the card (a lane card is draggable). */
  draggable?: boolean
  onDragStart?: React.DragEventHandler<HTMLDivElement>
  'data-guide'?: string
  'data-guide-name'?: string
}

/** The pulse keyframes, written once per page that pulses. */
export const PULSE_CSS = '@keyframes ktw-pulse{0%,100%{box-shadow:0 0 0 0 transparent}50%{box-shadow:0 0 0 3px color-mix(in srgb, var(--ktw-pulse) 35%, transparent)}}'

/** A tone's border style (and its pulse), for the rare frame that is not a card. */
export function toneBorder(tone?: string, pulse = false): CSSProperties | undefined {
  if (!tone) return undefined
  const s: CSSProperties = { borderLeft: `3px solid var(--mantine-color-${tone}-6)` }
  if (pulse) {
    s.animation = 'ktw-pulse 1.8s ease-in-out infinite'
    ;(s as Record<string, string>)['--ktw-pulse'] = `var(--mantine-color-${tone}-6)`
  }
  return s
}

/** An opening card lightens under the pointer - its border and background:
 *  the dark theme draws request-rows-mockup.html's row hover
 *  exactly (#4a4d55 / #2a2b31), the light theme a lighter gray; a tone keeps
 *  its left border. */
export const OPENS_CSS = '.ktw-item-opens:hover{border-color:light-dark(var(--mantine-color-gray-5),#4a4d55);background:light-dark(var(--mantine-color-gray-0),#2a2b31)}'

export function ItemCard({ tone, pulse, onClick, children, style, padding = 'sm', radius = 'md', className, ...rest }: ItemCardProps) {
  return (
    <Card withBorder padding={padding} radius={radius} onClick={onClick} {...rest}
      className={onClick ? [className, 'ktw-item-opens'].filter(Boolean).join(' ') : className}
      style={{ ...toneBorder(tone, pulse), cursor: onClick ? 'pointer' : undefined, ...(style as CSSProperties | undefined) }}>
      {onClick && <style>{OPENS_CSS}</style>}
      {pulse && <style>{PULSE_CSS}</style>}
      {children}
    </Card>
  )
}
