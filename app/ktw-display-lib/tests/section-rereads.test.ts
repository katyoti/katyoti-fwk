// SPDX-License-Identifier: Apache-2.0
// A section asked again while its read runs: on a
// busy universe the page around a section is read again every few seconds
// (its panel refreshes on its module), each re-read asking the section again.
// A read cancelled and restarted at each ask never answered when the asks came
// faster than the read lasted - the increment's Blockers tab stayed on its
// placeholder. The section's reads run one at a time: an ask during a read is
// held, never cancels it.

import { describe, expect, test } from 'bun:test'

import { sectionReads } from '../src/ui-builders/page/nodes/section-load.tsx'

const sleep = (ms: number) => new Promise((go) => setTimeout(go, ms))

describe('a section read again while it reads', () => {
  test('asked faster than its read lasts, it still answers - one read at a time', async () => {
    let started = 0, answered = 0, cancelled = 0, inFlight = 0, most = 0
    const part = sectionReads(async (signal) => {
      started++
      inFlight++
      most = Math.max(most, inFlight)
      await sleep(60)
      inFlight--
      if (signal.aborted) { cancelled++; return }
      answered++
    })
    part.ask()
    // The page around it read again every 20 ms, three times per read.
    for (let i = 0; i < 15; i++) { await sleep(20); part.ask() }
    await sleep(200)
    expect(cancelled).toBe(0)
    expect(answered).toBeGreaterThanOrEqual(3)
    expect(most).toBe(1)
    // The asks held during a read collapse into one read after it.
    expect(started).toBeLessThan(16)
    expect(part.holding()).toBe(false)
  })

  test('an ask during a read holds one more read, run once it ends', async () => {
    const signals: AbortSignal[] = []
    const part = sectionReads(async (signal) => { signals.push(signal); await sleep(30) })
    part.ask()
    part.ask()
    part.ask()
    expect(signals.length).toBe(1)
    expect(part.holding()).toBe(true)
    await sleep(45)
    expect(signals.length).toBe(2)
    expect(signals[0].aborted).toBe(false)
    await sleep(45)
    expect(signals.length).toBe(2)
  })

  test('stop - another part, the section gone - cancels the read in flight and holds nothing', async () => {
    const signals: AbortSignal[] = []
    const part = sectionReads(async (signal) => { signals.push(signal); await sleep(30) })
    part.ask()
    part.ask()
    part.stop()
    expect(signals[0].aborted).toBe(true)
    expect(part.holding()).toBe(false)
    await sleep(45)
    part.ask()
    expect(signals.length).toBe(1)
  })

  test('a failed read does not stop the reads that follow', async () => {
    let n = 0
    const part = sectionReads(async () => { n++; await sleep(10); if (n === 1) throw new Error('the hub restarts') })
    part.ask()
    part.ask()
    await sleep(40)
    expect(n).toBe(2)
  })
})
