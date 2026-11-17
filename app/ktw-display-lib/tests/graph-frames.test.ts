// SPDX-License-Identifier: Apache-2.0
// A graph draws boxes inside frames and status cards: a box
// others name as their group is a frame around them, laid out in one band of
// rows with its title on the first; an edge from or to a frame orders all its
// members; a box keeps its status, its frame style and its badges.

import { describe, expect, test } from 'bun:test'

import { layerGraph, type UIGraphBox } from '../src/ui-builders/page/ui-page.interactive.ts'

const nodes: UIGraphBox[] = [
  { id: 'IN-1', title: 'Delivery overview', badges: ['running: claude-a'] },
  { id: 'RQ-1', title: 'Tabs', group: 'IN-1', status: 'done' },
  { id: 'RQ-2', title: 'Board', group: 'IN-1', status: 'running' },
  { id: 'IN-2', title: 'Increment pane', badges: ['waiting: IN-1'] },
  { id: 'RQ-3', title: 'Goal bars', group: 'IN-2', status: 'todo' },
  { id: 'EXT', title: 'm.RQ-9', frame: 'dotted', status: 'waiting', meta: 'IN-7 in 1.007' },
]
const edges = [
  { from: 'RQ-1', to: 'RQ-2' },
  { from: 'IN-1', to: 'IN-2' },
  { from: 'EXT', to: 'RQ-3' },
]

describe('frames and status cards', () => {
  const { boxes, frames, layers } = layerGraph(nodes, edges)
  const box = (id: string) => boxes.find((b) => b.id === id)!
  test('a group is a frame, not a box; its members keep their fields', () => {
    expect(boxes.map((b) => b.id).sort()).toEqual(['EXT', 'RQ-1', 'RQ-2', 'RQ-3'])
    expect(frames.map((f) => f.id)).toEqual(['IN-1', 'IN-2'])
    expect(box('RQ-2').status).toBe('running')
    expect(box('EXT').frame).toBe('dotted')
    expect(frames[0].badges).toEqual(['running: claude-a'])
  })
  test('an edge on a frame orders every member: IN-2 after IN-1', () => {
    expect(box('RQ-1').layer).toBe(0)
    expect(box('RQ-2').layer).toBe(1)
    expect(box('RQ-3').layer).toBeGreaterThan(box('RQ-2').layer)
    expect(layers).toBe(3)
  })
  test('each frame is one band: its title row, then its members; bands never overlap', () => {
    const [a, b] = frames
    expect(a.slotFrom).toBe(0)
    expect(box('RQ-1').slot).toBe(1)
    expect(box('RQ-2').slot).toBe(1)
    expect(a.slotTo).toBe(2)
    expect(b.slotFrom).toBeGreaterThanOrEqual(a.slotTo)
    expect(box('RQ-3').slot).toBe(b.slotFrom + 1)
    expect([a.layerFrom, a.layerTo]).toEqual([0, 1])
    expect(box('EXT').slot).toBeGreaterThanOrEqual(b.slotTo)
  })
  test('a graph with no group is laid out as before', () => {
    const plain = layerGraph([{ id: 'a', title: 'A' }, { id: 'b', title: 'B' }], [{ from: 'a', to: 'b' }])
    expect(plain.frames).toEqual([])
    expect(plain.boxes.map((b) => [b.id, b.layer, b.slot])).toEqual([['a', 0, 0], ['b', 1, 0]])
  })
})
