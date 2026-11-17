// SPDX-License-Identifier: Apache-2.0
// A draw.io diagram drawn as a thumbnail: a small landscape
// view of the drawing that opens it in full - a click on it - in a large
// modal, its title above.
//
// The thumbnail is the drawing's SKETCH (../../drawio/sketch.ts), a still SVG
// in the page's colours: its boxes in the Mantine hue nearest their draw.io
// colour, its arrows, on the card's raised surface - never the draw.io viewer
// with its fixed white sheet and its loading splash (a white
// portrait sheet on the dark theme, a dark splash on the light one). A drawing
// the sketch cannot read shows a quiet « Schéma » mark (its labels in the
// person's language); the full view is the
// viewer's, the host's own when it gives one (`full`).

import i18next from 'i18next'
import { dataAttrs } from './data-attrs.ts'
import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Box, Modal, Stack, Text, UnstyledButton } from '@mantine/core'
import { IconSchema } from '@tabler/icons-react'

import { DrawioView } from '../../drawio/index.ts'
import { inflateDiagram, sketchFromModel, sketchOf, type Sketch, type SketchBox } from '../../drawio/sketch.ts'
import type { UIDrawioNode } from '../ui-page.types.ts'

/** thumbnailHeight is a thumbnail's height in pixels when it gives one; with
 *  `true` it has none: it is the card's width, landscape (16:10). */
export function thumbnailHeight(thumbnail: UIDrawioNode['thumbnail']): number | undefined {
  return typeof thumbnail === 'number' && thumbnail > 0 ? thumbnail : undefined
}

/** drawioOpenLabel names the thumbnail's button in the person's language. */
export function drawioOpenLabel(language: string | undefined, title: string | undefined): string {
  const fr = (language ?? '').startsWith('fr')
  if (!title) return fr ? 'Ouvrir le schéma' : 'Open the diagram'
  return fr ? `Ouvrir le schéma « ${title} »` : `Open the diagram “${title}”`
}

/** drawioMarkLabel is the quiet mark of a drawing the sketch cannot read. */
export function drawioMarkLabel(language: string | undefined): string {
  return (language ?? '').startsWith('fr') ? 'Schéma' : 'Diagram'
}

/** The thumbnail's surface: raised a step from the card, in both schemes. */
const SURFACE = 'light-dark(var(--mantine-color-gray-0), var(--mantine-color-dark-6))'
const LINE = 'var(--mantine-color-dimmed)'

function boxShape(b: SketchBox): ReactNode {
  const fill = b.container || !b.tone ? 'none' : `var(--mantine-color-${b.tone}-light)`
  const stroke = b.shape === 'text' ? 'none' : b.tone ? `var(--mantine-color-${b.tone}-outline)` : LINE
  const common = { fill, stroke, strokeWidth: 1.5, vectorEffect: 'non-scaling-stroke' as const }
  if (b.shape === 'ellipse') return <ellipse cx={b.x + b.w / 2} cy={b.y + b.h / 2} rx={b.w / 2} ry={b.h / 2} {...common} />
  if (b.shape === 'rhombus') {
    const cx = b.x + b.w / 2, cy = b.y + b.h / 2
    return <polygon points={`${cx},${b.y} ${b.x + b.w},${cy} ${cx},${b.y + b.h} ${b.x},${cy}`} {...common} />
  }
  const r = b.shape === 'rounded' ? Math.min(b.w, b.h) * 0.15 : Math.min(b.w, b.h) * 0.06
  return <rect x={b.x} y={b.y} width={b.w} height={b.h} rx={r} {...common} />
}

/** DrawioSketchSVG draws a sketch fitted to its box. */
export function DrawioSketchSVG({ sketch }: { sketch: Sketch }): ReactNode {
  const { view } = sketch
  // Labels at a size read at the thumbnail's scale: a share of the boxes' height.
  const heights = sketch.boxes.filter((b) => !b.container && b.shape !== 'text').map((b) => b.h).sort((a, b) => a - b)
  const font = Math.max(view.h / 22, (heights[Math.floor(heights.length / 2)] ?? 30) * 0.36)
  return (
    <svg viewBox={`${view.x} ${view.y} ${view.w} ${view.h}`} preserveAspectRatio="xMidYMid meet"
      style={{ width: '100%', height: '100%', display: 'block' }} data-guide="drawio.sketch" role="img">
      <defs>
        <marker id="ktw-sketch-arrow" viewBox="0 0 6 6" refX="5" refY="3" markerWidth="5" markerHeight="5" orient="auto">
          <path d="M0 0L6 3L0 6z" fill={LINE} />
        </marker>
      </defs>
      {sketch.boxes.map((b) => (
        <g key={b.id}>
          {boxShape(b)}
          {b.label && (
            <text x={b.x + b.w / 2} y={b.container ? b.y + font * 1.2 : b.y + b.h / 2} dominantBaseline={b.container ? 'auto' : 'central'}
              textAnchor="middle" fontSize={font} fill="var(--mantine-color-text)" style={{ fontFamily: 'inherit' }}>
              {b.label.length > 28 ? b.label.slice(0, 27) + '…' : b.label}
            </text>
          )}
        </g>
      ))}
      {sketch.edges.map((e, i) => (
        <line key={i} x1={e.x1} y1={e.y1} x2={e.x2} y2={e.y2} stroke={LINE} strokeWidth={1.2}
          vectorEffect="non-scaling-stroke" markerEnd="url(#ktw-sketch-arrow)" />
      ))}
    </svg>
  )
}

/** useSketch reads the node's drawing: at once when it is plain, after
 *  inflating its page when it is compressed; null when there is nothing to
 *  draw. */
function useSketch(xml: string): Sketch | null {
  const read = useMemo(() => sketchOf(xml), [xml])
  const [inflated, setInflated] = useState<Sketch | null>(null)
  useEffect(() => {
    setInflated(null)
    if (!('compressed' in read)) return
    let live = true
    inflateDiagram(read.compressed).then((model) => { if (live) setInflated(sketchFromModel(model)) }).catch(() => {})
    return () => { live = false }
  }, [read])
  return 'sketch' in read ? read.sketch : inflated
}

export interface DrawioThumbnailProps {
  node: UIDrawioNode
  /** The full view, the host's (the GUI draws draw.io in its viewer widget);
   *  the draw.io viewer by default. */
  full?: (node: UIDrawioNode) => ReactNode
}

export function DrawioThumbnail({ node, full }: DrawioThumbnailProps): ReactNode {
  const [open, setOpen] = useState(false)
  const sketch = useSketch(node.xml ?? '')
  const height = thumbnailHeight(node.thumbnail)
  const label = drawioOpenLabel(i18next.language, node.title)
  return (
    <>
      <UnstyledButton {...dataAttrs(node)} onClick={() => setOpen(true)} aria-label={label} title={label}
        data-guide="drawio.thumbnail" data-guide-name={node.id}
        style={{
          display: 'block', width: '100%', overflow: 'hidden', borderRadius: 'var(--mantine-radius-sm)',
          border: '1px solid var(--mantine-color-default-border)', background: SURFACE,
          ...(height ? { height } : { aspectRatio: '16 / 10' }),
        }}>
        <Box data-guide="drawio.open" style={{ width: '100%', height: '100%', padding: '4%', boxSizing: 'border-box' }}>
          {sketch ? <DrawioSketchSVG sketch={sketch} /> : (
            <Stack align="center" justify="center" gap={4} h="100%" c="dimmed" data-guide="drawio.mark">
              <IconSchema size={28} stroke={1.5} />
              <Text size="xs" c="dimmed">{drawioMarkLabel(i18next.language)}</Text>
            </Stack>
          )}
        </Box>
      </UnstyledButton>
      <Modal opened={open} onClose={() => setOpen(false)} size="90%" centered title={node.title} data-guide="drawio.full">
        {open && (full ? full(node) : <DrawioView value={{ xml: node.xml }} minHeight={node.height ?? 600} />)}
      </Modal>
    </>
  )
}
