// SPDX-License-Identifier: Apache-2.0
/**
 * useHostLoad — a view's loaded part through the host's one load.
 *
 * A view that reads its own content (a source panel, a list, a tree) loads it
 * as a page section is: through the universe's one gate and shared loads
 * (loadPart) - at most a few at once across every page and view, a load asked
 * twice at once run once -, an ended signal in place of a cancelled flag, and
 * drawn with the section's placeholder and its error card with « Réessayer »
 * (LoadedPart) instead of red text.
 *
 * The cold start is kept here, in the one place: right after a universe
 * starts, its core's catalog hydrates for a few seconds, and a first read can
 * answer empty (`isEmpty`) or fail (`retryErrors`, a core still starting).
 * Such an answer is asked again with a growing backoff (~6.5 s in all; a part
 * that must outlast a hub restart takes RESTART_BACKOFF_MS, ~30 s), the last
 * answer taken once the backoff is spent.
 *
 * A re-read (`revision` changed, `reread()`) keeps the shown content until its
 * answer; a new part (`identity` changed) shows the placeholder. A `reread()`
 * asked while a read runs never restarts it: it is held, and run once when
 * that read ends - however many were asked meanwhile (under a stream of
 * changes a restarted read never answered).
 */
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { Center, Loader } from '@mantine/core'

import { SectionFailed } from '@ktw/display-lib/ui-builders/page/nodes/section-load.tsx'
import { isReadTimeout, loadPart } from '../components/compositions/hostServices'
import { errorText } from '../i18n/errors'

/** The cold-start backoff: the waits between tries of an empty or failed answer. */
export const COLD_START_BACKOFF_MS = [200, 300, 500, 800, 1200, 1500, 2000]

/** One part's load. */
export interface PartLoad<T> {
  universe: string
  /** What is loaded: a load of the same key in flight is shared. */
  key: string
  load: (signal: AbortSignal) => Promise<T>
  /** An answer that counts as empty is asked again while the core starts. */
  isEmpty?: (value: T) => boolean
  /** A rejected answer is asked again too (a core still starting); a
   *  predicate asks again only the failures it accepts (a refusal that will
   *  not change is shown at once). */
  retryErrors?: boolean | ((err: unknown) => boolean)
  /** The waits between tries, when the part must outlast more than a core
   *  starting (the hub restarting: RESTART_BACKOFF_MS). */
  backoff?: number[]
}

/** A longer backoff (~30 s in all) for a part that must outlast a hub
 *  restart (a deploy), not only a core's catalog hydrating. */
export const RESTART_BACKOFF_MS = [...COLD_START_BACKOFF_MS, 3000, 5000, 8000, 10000]

export function isAbort(e: unknown): boolean {
  return (e as { name?: string } | null)?.name === 'AbortError'
}

const aborted = () => new DOMException('The part no longer needs its load', 'AbortError')

/** wait resolves after ms, or rejects when the signal ends. */
export function wait(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((go, stop) => {
    if (signal.aborted) { stop(aborted()); return }
    const timer = setTimeout(() => { signal.removeEventListener('abort', end); go() }, ms)
    const end = () => { clearTimeout(timer); stop(aborted()) }
    signal.addEventListener('abort', end, { once: true })
  })
}

/** runPart loads one part through the universe's one load, the cold-start
 *  backoff applied: it answers the data, rejects with the failure, and an
 *  ended signal stops it (AbortError). */
export async function runPart<T>(p: PartLoad<T>, signal: AbortSignal, backoff: number[] = p.backoff ?? COLD_START_BACKOFF_MS): Promise<T> {
  for (let n = 0; ; n++) {
    let value: T
    try {
      value = await loadPart(p.universe, p.key, p.load, signal)
    } catch (e) {
      const again = typeof p.retryErrors === 'function' ? p.retryErrors(e) : !!p.retryErrors
      // A read past its deadline is shown, never waited again.
      if (isAbort(e) || isReadTimeout(e) || signal.aborted || !again || n >= backoff.length) throw e
      await wait(backoff[n], signal)
      continue
    }
    if (p.isEmpty?.(value) && n < backoff.length) { await wait(backoff[n], signal); continue }
    return value
  }
}

/** A loaded part, as a view draws it. */
export interface HostLoad<T> {
  /** The answer; kept during a re-read, undefined before the first. */
  data: T | undefined
  /** True until the first answer of the part (a re-read keeps it false). */
  loading: boolean
  /** The load's failure, in the GUI language. */
  error: string | null
  /** « Réessayer »: the placeholder, then the load again. */
  retry: () => void
  /** Read again, the shown content kept until the answer. */
  reread: () => void
}

export interface UseHostLoad<T> extends Omit<PartLoad<T>, 'universe'> {
  universe: string | null | undefined
  /** Which part is shown: a change shows the placeholder. Defaults to the key
   *  (a key change under the same identity keeps the content, as a query). */
  identity?: string
  /** Off: nothing is loaded (no data, not loading). */
  enabled?: boolean
  /** A change re-reads, the content kept. */
  revision?: unknown
}

/** What a part holds: the part it is (`at`), its answer or failure. */
export interface Got<T> { at: string; data?: T; error?: string; done: boolean }

/** partOnLoad is a part as a load starts: a re-read of the part shown keeps
 *  its content; a new part, or one that failed, shows the placeholder. */
export function partOnLoad<T>(was: Got<T>, at: string): Got<T> {
  return was.at === at && was.done && was.error === undefined ? was : { at, done: false }
}

/** partOnFail is a part whose load failed: the error card, its content kept
 *  for the part it was. */
export function partOnFail<T>(was: Got<T>, at: string, error: string): Got<T> {
  return { at, data: was.at === at ? was.data : undefined, error, done: true }
}

/** partShown is what a view draws of a part (`on`: it loads at all). */
export function partShown<T>(got: Got<T>, at: string, on: boolean): Pick<HostLoad<T>, 'data' | 'loading' | 'error'> {
  const mine = on && got.at === at
  return { data: mine ? got.data : undefined, loading: on && !(mine && got.done), error: mine ? got.error ?? null : null }
}

/** rereadQueue holds the re-reads asked while a read runs (`ask` during
 *  `began`..`ended`) and starts one, once, when it ended; a read given up
 *  (`dropped`: another part, the view gone) holds nothing - what follows reads
 *  anew. */
export function rereadQueue(start: () => void) {
  let running = false
  let held = false
  return {
    ask() { if (running) held = true; else start() },
    began() { running = true; held = false },
    ended() { running = false; if (held) { held = false; start() } },
    dropped() { running = false; held = false },
    /** Whether a re-read is held (tests). */
    holding: () => held,
  }
}

/** leftLoads keeps a view's load given up by its effect for one turn:
 * an effect that ends and runs again in the same
 *  turn with the same ask - the view mounted again, React's development
 *  double effect - joins the call in flight (the shared loads) instead of
 *  cancelling it and computing it a second time, `fresh`. An effect running
 *  another ask (another part, a re-read, a new revision) gives the left load
 *  up at once; a load no one asks again is given up once the turn ends. */
export function leftLoads() {
  let left: { ask: string; ctl: AbortController } | null = null
  return {
    /** The effect ended: its load is given up once the turn ends. */
    leave(ask: string, ctl: AbortController) {
      left?.ctl.abort()
      left = { ask, ctl }
      setTimeout(() => { ctl.abort(); if (left?.ctl === ctl) left = null }, 0)
    },
    /** An effect begins: a load left by another ask is given up now; one
     *  left by the same ask stays until the turn ends, for this one to join. */
    begin(ask: string) {
      if (left && left.ask !== ask) left.ctl.abort()
      left = null
    },
  }
}

export function useHostLoad<T>(opts: UseHostLoad<T>): HostLoad<T> {
  const { universe, key, isEmpty, retryErrors, backoff, enabled = true, revision } = opts
  const at = `${universe ?? ''}|${opts.identity ?? key}`
  const on = enabled && !!universe
  // The load and the predicate by reference: the key says when to load.
  const load = useRef(opts.load)
  load.current = opts.load
  const empty = useRef(isEmpty)
  empty.current = isEmpty
  const [round, setRound] = useState(0)
  const [got, setGot] = useState<Got<T>>({ at: '', done: false })
  const rereads = useRef<ReturnType<typeof rereadQueue> | null>(null)
  if (!rereads.current) rereads.current = rereadQueue(() => setRound((r) => r + 1))
  const left = useRef<ReturnType<typeof leftLoads> | null>(null)
  if (!left.current) left.current = leftLoads()
  useEffect(() => {
    if (!on) return
    const ask = JSON.stringify([at, key, revision ?? null, round])
    left.current!.begin(ask)
    const ctl = new AbortController()
    // Ended with the effect, at once: its answer is no longer this view's,
    // though the call it shares may end a turn later (leftLoads).
    let live = true
    const queue = rereads.current!
    queue.began()
    // A new part shows the placeholder; a re-read keeps what is shown.
    setGot((was) => partOnLoad(was, at))
    runPart<T>({
      universe: universe!, key, retryErrors, backoff,
      load: (signal) => load.current(signal),
      isEmpty: (v) => empty.current?.(v) ?? false,
    }, ctl.signal)
      .then((data) => { if (live) setGot({ at, data, done: true }) })
      .catch((e: unknown) => {
        if (!live || isAbort(e)) return
        setGot((was) => partOnFail(was, at, errorText(e)))
      })
      // The read ended: a re-read asked meanwhile runs now.
      .finally(() => { if (live) queue.ended() })
    return () => { live = false; left.current!.leave(ask, ctl); queue.dropped() }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- the key and the revision say when to load
  }, [at, key, on, revision, round])
  const retry = useCallback(() => { setGot({ at, done: false }); setRound((r) => r + 1) }, [at])
  const reread = useCallback(() => rereads.current!.ask(), [])
  return { ...partShown(got, at, on), retry, reread }
}

/** LoadedPart draws a loaded part: its placeholder until the first answer,
 *  the error card with « Réessayer » when it failed, else its content. */
export function LoadedPart<T>({ id, part, placeholder, children }: {
  id: string
  part: HostLoad<T>
  placeholder?: ReactNode
  children: (data: T) => ReactNode
}): ReactNode {
  if (part.error) return <SectionFailed id={id} message={part.error} onRetry={part.retry} />
  if (part.data === undefined) {
    return placeholder ?? <Center p="md" data-section={id} data-state="loading"><Loader size="sm" /></Center>
  }
  return <>{children(part.data)}</>
}
