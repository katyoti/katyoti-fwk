// SPDX-License-Identifier: Apache-2.0
// <GraphFlow> — the GUI's renderer of the page DSL's `graph` node, on
// @xyflow/react (the library of the Teams tab): the same boxes and edges, laid
// out in layers by the display library's layerGraph, with pan, zoom, a minimap
// and controls; the critical path drawn accented and animated; a click on a
// box runs the node's `open` action through the page's host. A group's frame
// is drawn behind its members, titled, dotted when the box says so, and a box
// carries its status in the shared status colours and a line of badges.
// Registered by PageHost (registerPageNodeRenderer); the
// iframe keeps the SVG renderer.

import { useEffect, useMemo, useRef } from 'react'
import {
  Background, Controls, Handle, MarkerType, MiniMap, Position, ReactFlow, ReactFlowProvider, useReactFlow,
  type Edge, type Node, type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import { Box, Paper, Text } from '@mantine/core'
import { statusChip } from '@ktw/display-lib/ui-builders/fields/_planning/palette.ts'
import { useT } from '../../i18n'

import {
  dataAttrs, layerGraph, usePageContext, type UIGraphNode, type UIPageNode,
} from '@ktw/display-lib/ui-builders/page/index.ts'

const COL = 230
const ROW = 92
const BOX_W = 190
const PAD = 14

interface BoxData extends Record<string, unknown> {
  title: string
  meta?: string
  tone: string
  critical: boolean
  status?: string
  badges?: string[]
  dotted?: boolean
}

function GraphBox({ data }: NodeProps<Node<BoxData>>) {
  const { title, meta, tone, critical, status, badges, dotted } = data
  const look = status ? statusChip(status) : undefined
  return (
    <Paper withBorder radius="md" px="sm" py={8} w={BOX_W} shadow={critical ? 'md' : 'xs'} data-status={status} style={{
      borderColor: look ? look.border : `var(--mantine-color-${tone}-${critical ? 5 : 7})`,
      borderWidth: critical ? 2 : 1,
      borderStyle: dotted ? 'dotted' : 'solid',
      background: look ? look.bg : critical ? `var(--mantine-color-${tone}-light)` : 'var(--mantine-color-body)',
    }}>
      <Handle type="target" position={Position.Left} style={{ opacity: 0 }} />
      <Text size="sm" fw={600} truncate>{title}</Text>
      {meta && <Text size="xs" c="dimmed" truncate>{meta}</Text>}
      {badges?.length ? <Text size="xs" truncate style={{ color: look?.fg }} c={look ? undefined : 'dimmed'}>{badges.join(' · ')}</Text> : null}
      <Handle type="source" position={Position.Right} style={{ opacity: 0 }} />
    </Paper>
  )
}

// A group's frame: its title and line on top, its members inside.
function GraphFrame({ data }: NodeProps<Node<BoxData>>) {
  const { title, meta, status, badges, dotted } = data
  const look = status ? statusChip(status) : undefined
  return (
    <Box data-status={status} style={{
      width: '100%', height: '100%', borderRadius: 12, padding: '6px 10px',
      border: `1.5px ${dotted ? 'dotted' : 'solid'} ${look ? look.border : 'var(--mantine-color-gray-6)'}`,
    }}>
      <Handle type="target" position={Position.Left} style={{ opacity: 0 }} />
      <Text size="sm" fw={700} truncate>{title}</Text>
      {(meta || badges?.length) && (
        <Text size="xs" truncate style={{ color: look?.fg }} c={look ? undefined : 'dimmed'}>
          {[meta, ...(badges ?? [])].filter(Boolean).join(' · ')}
        </Text>
      )}
      <Handle type="source" position={Position.Right} style={{ opacity: 0 }} />
    </Box>
  )
}

const nodeTypes = { box: GraphBox, frame: GraphFrame }

function Flow({ node }: { node: UIGraphNode }) {
  const t = useT('components')
  const { onAction } = usePageContext()
  const flow = useReactFlow()
  const box = useRef<HTMLDivElement>(null)
  // Fit whenever the box gets (or changes) its size: a graph in a tab that was
  // hidden when it mounted measured 0x0 and fit at the minimum zoom.
  useEffect(() => {
    const el = box.current
    if (!el) return
    const ro = new ResizeObserver(() => {
      if (el.clientWidth > 0 && el.clientHeight > 0) void flow.fitView({ padding: 0.2, maxZoom: 1 })
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [flow])
  const boxesIn = node.data?.nodes ?? []
  const edgesIn = node.data?.edges ?? []
  const { nodes, edges, height } = useMemo(() => {
    const { boxes, frames } = layerGraph(boxesIn, edgesIn)
    const critical = new Set(boxes.filter((b) => b.critical).map((b) => b.id))
    const rows = Math.max(1, ...boxes.map((b) => b.slot + 1))
    const dataOf = (b: (typeof boxes)[number] | (typeof frames)[number]): BoxData => ({
      title: b.title, meta: b.meta, tone: b.tone ?? (b.critical ? 'blue' : 'gray'), critical: !!b.critical,
      status: b.status, badges: b.badges, dotted: b.frame === 'dotted',
    })
    return {
      height: node.height ?? Math.min(560, Math.max(220, rows * ROW + 80)),
      nodes: [
        ...frames.map((f): Node<BoxData> => ({
          id: f.id, type: 'frame', zIndex: -1, selectable: false,
          position: { x: f.layerFrom * COL - PAD, y: f.slotFrom * ROW - PAD / 2 },
          style: { width: (f.layerTo - f.layerFrom) * COL + BOX_W + 2 * PAD, height: (f.slotTo - f.slotFrom) * ROW + PAD },
          data: dataOf(f),
        })),
        ...boxes.map((b): Node<BoxData> => ({
          id: b.id, type: 'box', position: { x: b.layer * COL, y: b.slot * ROW },
          data: dataOf(b),
        })),
      ],
      edges: edgesIn.map((e, i): Edge => {
        const hot = critical.has(e.from) && critical.has(e.to)
        const color = hot ? 'var(--mantine-color-blue-5)' : 'var(--mantine-color-gray-6)'
        return {
          id: `e${i}`, source: e.from, target: e.to, type: 'smoothstep', animated: hot, label: e.label,
          style: { stroke: color, strokeWidth: hot ? 2 : 1.3, strokeDasharray: e.style === 'dashed' ? '5 4' : undefined },
          markerEnd: { type: MarkerType.ArrowClosed, color },
        }
      }),
    }
  }, [boxesIn, edgesIn, node.height])

  if (nodes.length === 0) return <Text {...dataAttrs(node)} size="sm" c="dimmed">{t('graphFlow.empty')}</Text>
  return (
    <Box {...dataAttrs(node)} ref={box} h={height} style={{ borderRadius: 8, overflow: 'hidden', border: '1px solid var(--mantine-color-default-border)' }}>
      <ReactFlow
        nodes={nodes} edges={edges} nodeTypes={nodeTypes}
        colorMode="dark" fitView fitViewOptions={{ padding: 0.2, maxZoom: 1 }} minZoom={0.3} maxZoom={1.8}

        nodesDraggable={false} nodesConnectable={false} proOptions={{ hideAttribution: true }}
        style={{ background: 'var(--mantine-color-dark-8)' }}
        onNodeClick={(_e, n) => {
          if (!node.open || !onAction) return
          const box = boxesIn.find((b) => b.id === n.id)
          onAction({ action: node.open, item: box })
        }}>
        <Background gap={18} size={1} color="var(--mantine-color-dark-5)" />
        {nodes.length > 6 && (
          <MiniMap pannable zoomable nodeColor={(n) => (n.data as BoxData).critical ? 'var(--mantine-color-blue-6)' : 'var(--mantine-color-gray-6)'}
            style={{ background: 'var(--mantine-color-dark-7)', width: 140, height: 90 }} />
        )}
        <Controls showInteractive={false} />
      </ReactFlow>
    </Box>
  )
}

export function GraphFlow({ node }: { node: UIPageNode }) {
  return (
    <ReactFlowProvider>
      <Flow node={node as UIGraphNode} />
    </ReactFlowProvider>
  )
}
