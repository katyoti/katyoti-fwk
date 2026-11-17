// SPDX-License-Identifier: Apache-2.0
/**
 * KpiTile — the headline figure every view draws the same way: a label, the
 * value large in its colour, a caption, an icon badge; the colour tints the
 * tile and its top border. The page DSL's `kpi` node renders through it, and
 * a hand-written view uses it directly — no view draws its own tile.
 *
 * A sparkline (a series of numbers, oldest first) sits at the bottom right;
 * a changed value bumps once (none under prefers-reduced-motion). A selected
 * tile (the filter a page shows, its tile chosen) is ringed in its colour —
 * the primary colour when it has none —, in the light and the dark theme.
 * A footer (a page node's children: a button, a link) sits under the caption,
 * inside the tile.
 *
 * The `inline` variant is a call to act rather than a measure: the label bold
 * in the text colour with the count on its line, at the right, no icon; the
 * tile ringed with a soft line in its colour over its tint, no coloured top.
 */

import { useEffect, useRef, type CSSProperties, type ReactNode } from 'react'
import { Badge, Card, Group, Stack, Text, ThemeIcon } from '@mantine/core'
import { IconFile } from '@tabler/icons-react'
import { tablerIcon } from '../icons/index.ts'

export interface KpiTileProps {
  label: string
  value: string | number
  /** The caption under the value. */
  description?: string
  /** A Mantine colour name: the tint, the top border, the value, the icon. */
  color?: string
  /** A Tabler icon name (kebab, e.g. 'loader-2'). */
  icon?: string
  /** A trend chip beside the value ('+12%', '-3', 'stable'). */
  trend?: string
  /** The chip's colour; derived from the trend's sign when not given. */
  trendColor?: string
  /** A sparkline's points, oldest first. */
  spark?: number[]
  /** The tile is the one chosen (a filter's): ringed in its colour. */
  selected?: boolean
  /** Extra attributes on the tile's root (a page node's data-* keys); the tile's own win. */
  attrs?: Record<string, string>
  /** What sits under the caption, inside the tile (a button opening what the count counts). */
  footer?: ReactNode
  /** `inline`: the bold label and the count on one line, no icon, a soft ring. */
  variant?: 'inline'
}

export function KpiTile({ label, value, description, color: c, icon, trend, trendColor, spark, selected, attrs, footer, variant }: KpiTileProps) {
  const Icon = icon ? tablerIcon(icon, IconFile) : undefined
  const chip = trendColor ?? trendTone(trend)
  const ref = useRef<HTMLParagraphElement>(null)
  const last = useRef(value)
  useEffect(() => {
    const el = ref.current
    if (last.current !== value && el?.animate && !window.matchMedia?.('(prefers-reduced-motion: reduce)').matches) {
      el.animate([{ transform: 'scale(1)' }, { transform: 'scale(1.16)' }, { transform: 'scale(1)' }], { duration: 500, easing: 'ease' })
    }
    last.current = value
  }, [value])
  if (variant === 'inline') {
    return (
      <Card {...attrs} withBorder padding="md" radius="md" data-kpi-variant="inline" data-selected={selected || undefined} style={kpiTileStyle(c, selected, variant)}>
        <Stack gap={2}>
          <Group justify="space-between" wrap="nowrap" gap="xs" align="center">
            <Text fw={700} size="sm">{label}</Text>
            <Text ref={ref} fw={800} style={{ lineHeight: 1, fontSize: 26, transformOrigin: 'right center' }} c={c ? `${c}.3` : undefined}>
              {value}
            </Text>
          </Group>
          {description && <Text size="xs" c="dimmed">{description}</Text>}
          {footer && <Group gap="xs" mt={8} data-kpi-footer="">{footer}</Group>}
        </Stack>
      </Card>
    )
  }
  return (
    <Card {...attrs} withBorder padding="md" radius="md" data-selected={selected || undefined} style={kpiTileStyle(c, selected)}>
      <Stack gap={4}>
        <Group justify="space-between" wrap="nowrap" gap="xs">
          <Text size="xs" c="dimmed" tt="uppercase" fw={600}>{label}</Text>
          {Icon && (
            <ThemeIcon variant="light" color={c ?? 'gray'} radius="md" size="md">
              <Icon size={16} />
            </ThemeIcon>
          )}
        </Group>
        <Group gap="xs" align="flex-end">
          <Text ref={ref} fw={700} style={{ lineHeight: 1, fontSize: 30, transformOrigin: 'left bottom' }} c={c ? `${c}.4` : undefined}>
            {value}
          </Text>
          {trend && chip && <Badge color={chip} variant="light" size="sm">{trend}</Badge>}
        </Group>
        {description && <Text size="xs" c="dimmed">{description}</Text>}
        {footer && <Group gap="xs" mt={4} data-kpi-footer="">{footer}</Group>}
      </Stack>
      {spark && spark.length > 1 && <Sparkline points={spark} color={c ?? 'blue'} />}
    </Card>
  )
}

/** A tile's frame: tinted and topped in its colour; a selected tile ringed
 *  with a 2px border in its colour (the primary colour when it has none) —
 *  theme variables, so the mark holds in light and dark. The `inline` variant
 *  has no coloured top: a 1px ring in its colour, softened (softLine). */
export function kpiTileStyle(color?: string, selected?: boolean, variant?: 'inline'): CSSProperties {
  const ring = color ? `var(--mantine-color-${color}-6)` : 'var(--mantine-primary-color-filled)'
  const inline = variant === 'inline'
  return {
    position: 'relative', overflow: 'hidden', height: '100%',
    ...(color ? { background: `linear-gradient(135deg, var(--mantine-color-${color}-light) 0%, transparent 70%)` } : {}),
    ...(inline && color ? { border: `1px solid ${softLine(color)}` } : {}),
    ...(selected ? { border: `2px solid ${ring}` } : {}),
    ...(color && !inline ? { borderTop: `3px solid var(--mantine-color-${color}-6)` } : {}),
  }
}

/** A line in a colour, softened into the surface under it: the colour's
 *  filled shade at 45% (an inline tile's ring, an outlined button's border). */
export function softLine(color: string): string {
  return `color-mix(in srgb, var(--mantine-color-${color}-6) 45%, transparent)`
}

/** A trend's chip colour from its sign: + green, - red, otherwise gray. */
export function trendTone(trend: string | undefined): string | undefined {
  if (!trend) return undefined
  const t = trend.trim()
  if (t.startsWith('+')) return 'green'
  if (t.startsWith('-')) return 'red'
  return 'gray'
}

const W = 110
const H = 34

/** The sparkline's points in a W×H box: x spread evenly, y scaled to the
 *  series' maximum (a flat zero series lies on the bottom). */
export function sparkPath(points: number[]): [number, number][] {
  const max = Math.max(1, ...points)
  const step = points.length > 1 ? W / (points.length - 1) : 0
  return points.map((v, i) => [Math.round(i * step * 10) / 10, Math.round((H - 2 - (v / max) * (H - 6)) * 10) / 10])
}

function Sparkline({ points, color }: { points: number[]; color: string }) {
  const pts = sparkPath(points)
  const line = pts.map((p) => p.join(',')).join(' ')
  const [lx, ly] = pts[pts.length - 1]
  return (
    <svg viewBox={`0 0 ${W} ${H}`} aria-hidden="true" style={{ position: 'absolute', right: 12, bottom: 10, width: W, height: H }}>
      <polygon fill={`var(--mantine-color-${color}-light)`} points={`0,${H} ${line} ${W},${H}`} />
      <polyline fill="none" stroke={`var(--mantine-color-${color}-5)`} strokeWidth="1.6" points={line} />
      <circle r="2.6" fill={`var(--mantine-color-${color}-5)`} cx={lx} cy={ly} />
    </svg>
  )
}
