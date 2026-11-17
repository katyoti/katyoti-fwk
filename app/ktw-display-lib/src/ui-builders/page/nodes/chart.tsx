// SPDX-License-Identifier: Apache-2.0
// ChartRenderer — renders a UIChartNode via @mantine/charts.
//
// Per dsl-chart-node (todo/ms06, 2026-05-07): wraps Mantine's chart
// components (LineChart / BarChart / AreaChart / PieChart / DonutChart),
// each backed by recharts. Theme-consistent (Mantine color tokens) so
// dark mode + theme overrides propagate via the iframe's HostThemeBridge.
//
// Reactive: Mantine charts re-render on prop change. When the host
// pushes a `value` postMessage updating the artifact's tree, the new
// `data` array flows in and the chart updates without special wiring.
//
// Empty-data: renders a dimmed "No data" placeholder card so the LLM's
// in-progress emission (or a stream that mounts before patches arrive)
// shows something non-broken.

import { dataAttrs } from './data-attrs.ts'
import { AreaChart, BarChart, DonutChart, LineChart, PieChart } from '@mantine/charts'
import { Box, Card, Stack, Text } from '@mantine/core'

import type { UIChartNode, UIChartSeries } from '../ui-page.types.ts'

const DEFAULT_HEIGHT = 240

export function ChartRenderer({ node }: { node: UIChartNode }) {
  const data = Array.isArray(node.data) ? node.data : []

  if (data.length === 0) {
    return (
      <Card {...dataAttrs(node)} withBorder padding="md" radius="md">
        {node.title && (
          <Text fw={600} size="sm" mb="xs">{node.title}</Text>
        )}
        <Text size="xs" c="dimmed">No data</Text>
      </Card>
    )
  }

  const h = node.h ?? DEFAULT_HEIGHT
  const body = renderChart(node, data, h, node.title ? {} : dataAttrs(node))

  if (!node.title) return body

  return (
    <Stack {...dataAttrs(node)} gap={6}>
      <Text fw={600} size="sm">{node.title}</Text>
      {body}
    </Stack>
  )
}

function renderChart(
  node: UIChartNode,
  data: Array<Record<string, unknown>>,
  h: number,
  attrs: Record<string, string>,
) {
  switch (node.chartKind) {
    case 'line':
      return (
        <LineChart
          {...attrs}
          h={h}
          data={data}
          dataKey={node.dataKey ?? 'date'}
          series={normalizeSeries(node.series)}
          curveType={node.curveType ?? 'monotone'}
          withTooltip={node.withTooltip ?? true}
          withLegend={resolveLegend(node)}
          withXAxis={node.withXAxis ?? true}
          withYAxis={node.withYAxis ?? true}
        />
      )
    case 'bar':
      return (
        <BarChart
          {...attrs}
          h={h}
          data={data}
          dataKey={node.dataKey ?? 'category'}
          series={normalizeSeries(node.series)}
          withTooltip={node.withTooltip ?? true}
          withLegend={resolveLegend(node)}
          withXAxis={node.withXAxis ?? true}
          withYAxis={node.withYAxis ?? true}
        />
      )
    case 'area':
      return (
        <AreaChart
          {...attrs}
          h={h}
          data={data}
          dataKey={node.dataKey ?? 'date'}
          series={normalizeSeries(node.series)}
          curveType={node.curveType ?? 'monotone'}
          withTooltip={node.withTooltip ?? true}
          withLegend={resolveLegend(node)}
          withXAxis={node.withXAxis ?? true}
          withYAxis={node.withYAxis ?? true}
        />
      )
    case 'pie': {
      // Pie/Donut size themselves to a square SVG of `size` px. We center
      // them in their card via a flex row WITHOUT a fixed height — the
      // chart's intrinsic size controls layout. The earlier
      // `Box{height: h, display: flex}` wrapper collapsed the chart to
      // zero on some flex layouts (the chart never received a width).
      const pieSize = Math.min(h, 280)
      return (
        <Box {...attrs} style={{ display: 'flex', justifyContent: 'center', padding: 8 }}>
          <PieChart
            data={normalizePieData(data, node)}
            withTooltip={node.withTooltip ?? true}
            withLabels={node.withLabels ?? false}
            size={pieSize}
          />
        </Box>
      )
    }
    case 'donut': {
      const donutSize = Math.min(h, 280)
      return (
        <Box {...attrs} style={{ display: 'flex', justifyContent: 'center', padding: 8 }}>
          <DonutChart
            data={normalizePieData(data, node)}
            withTooltip={node.withTooltip ?? true}
            withLabels={node.withLabels ?? false}
            thickness={node.thickness ?? 20}
            size={donutSize}
          />
        </Box>
      )
    }
    default: {
      const _exhaustive: never = node.chartKind
      void _exhaustive
      return (
        <Card {...attrs} withBorder padding="md" radius="md">
          <Text size="xs" c="red">Unknown chartKind: {String((node as { chartKind?: string }).chartKind)}</Text>
        </Card>
      )
    }
  }
}

// Mantine line/bar/area chart series require a `color` (it's not optional in
// their types). When the LLM omits color, we cycle through this palette so
// the chart still renders without forcing the LLM to pick colors.
const DEFAULT_PALETTE = [
  'indigo.6', 'teal.6', 'orange.6', 'grape.6', 'cyan.6',
  'lime.6', 'pink.6', 'yellow.6', 'blue.6', 'red.6',
]

/** Mantine line/bar/area expect `series: [{name, color, label?}]` with
 *  `color` REQUIRED. We pass through user-provided color when set,
 *  else cycle through DEFAULT_PALETTE so the chart renders without the
 *  LLM having to enumerate colors. */
function normalizeSeries(
  series: UIChartSeries[] | undefined,
): Array<{ name: string; color: string; label?: string }> {
  if (!series || series.length === 0) return []
  return series.map((s, i) => ({
    name: s.name,
    color: s.color ?? DEFAULT_PALETTE[i % DEFAULT_PALETTE.length],
    label: s.label,
  }))
}

/** Pie/donut data shape: `[{name, value, color}]` — Mantine requires color
 *  on each cell. Map from the user-supplied `nameKey` / `valueKey` (defaults:
 *  'name', 'value'). When the row doesn't carry a `color`, cycle through
 *  the default palette so the chart renders. */
function normalizePieData(
  data: Array<Record<string, unknown>>,
  node: UIChartNode,
): Array<{ name: string; value: number; color: string }> {
  const nameKey = node.nameKey ?? 'name'
  const valueKey = node.valueKey ?? 'value'
  return data.map((row, i) => {
    const name = String(row[nameKey] ?? `Item ${i + 1}`)
    const rawValue = row[valueKey]
    const value = typeof rawValue === 'number' ? rawValue : Number(rawValue) || 0
    const color = typeof row.color === 'string'
      ? row.color
      : DEFAULT_PALETTE[i % DEFAULT_PALETTE.length]
    return { name, value, color }
  })
}

/** Default legend behavior: show when more than one series. Honor explicit
 *  `withLegend` when set. */
function resolveLegend(node: UIChartNode): boolean {
  if (typeof node.withLegend === 'boolean') return node.withLegend
  return (node.series?.length ?? 0) > 1
}
