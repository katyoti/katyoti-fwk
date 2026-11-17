// SPDX-License-Identifier: Apache-2.0
// A section: any page node carrying `id`, `load` and a
// `placeholder` ('s section props). With the host's progressive
// flag on and a loadSection host service, the node draws its
// placeholder, asks the host to load it when it comes into view (`when:
// visible`, the default), at once (`eager`) or when its tab or accordion item
// opens (`open` - it is mounted only then), and replaces itself, by its id,
// with what the load answers. With the flag off, or no loader, the node draws
// as it is declared - except a section with nothing of its own to draw (an
// empty box: the part of a page fed only by its own load), which loads
// whenever the host can and shows its `empty` text where it cannot.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, useRef, useState, type ComponentType, type ReactNode } from 'react'
import { Alert, Button, Card, Group, Skeleton, Stack, Text } from '@mantine/core'
import i18next from 'i18next'

import { useHostServices } from '../../../ui-core/host/index.tsx'
import { isNamedSpec, isSection, type SectionProps } from '../answer.ts'
import { SectionLoadContext, usePageContext, useSelection } from '../page-context.tsx'
import { resolveActionArgs } from '../ui-page.interactive.ts'
import type { UIPageActionEvent } from '../ui-page.interactive.ts'
import type { UIPageNode } from '../ui-page.types.ts'

/** loadOnly tells a section with nothing of its own to draw: an empty box,
 *  fed only by its load. */
export function loadOnly(node: unknown): boolean {
  if (!isSection(node)) return false
  const n = node as { kind?: unknown; children?: unknown }
  return n.kind === 'box' && !(Array.isArray(n.children) && n.children.length > 0)
}

/** sectionMode says how a node draws: `load` when it is a section the host
 *  can load on a progressive page - or a load-only section wherever the host
 *  can load -, `empty` for a load-only section the host cannot load, `as-is`
 *  otherwise. */
export function sectionMode(node: unknown, progressive: boolean, canLoad: boolean): 'load' | 'empty' | 'as-is' {
  if (!isSection(node)) return 'as-is'
  if (loadOnly(node)) return canLoad ? 'load' : 'empty'
  return progressive && canLoad ? 'load' : 'as-is'
}

/** SectionEmpty is a load-only section on a surface that loads none (a chat
 *  artifact): its empty text, dimmed. */
export function SectionEmpty({ id, empty, attrs }: { id: string; empty?: string; attrs?: Record<string, string> }): ReactNode {
  return <div {...attrs} data-section={id} data-state="unloaded">{empty ? <Text size="sm" c="dimmed" fs="italic">{empty}</Text> : null}</div>
}

/** sectionWhen is when a section loads: visible by default. */
export function sectionWhen(node: Partial<SectionProps>): 'visible' | 'open' | 'eager' {
  return node.when === 'eager' || node.when === 'open' ? node.when : 'visible'
}

/** sectionLoadsNow says whether a section asks for its load: an eager or an
 *  open one (mounted only once open) at once, a visible one once in view - or
 *  at once where the surface cannot tell (no IntersectionObserver). */
export function sectionLoadsNow(when: 'visible' | 'open' | 'eager', inView: boolean, canObserve: boolean): boolean {
  return when !== 'visible' || inView || !canObserve
}

/** sectionContent reads what a section's load answered as the node to draw
 *  in its place and that node's data: the one answer's inline tree, a page
 *  {dslTree, data}, or a node itself. A tree still named by URI is the
 *  host's to resolve: unresolved, it draws nothing. */
export function sectionContent(answer: unknown): { node: UIPageNode; data?: unknown } | undefined {
  if (!answer || typeof answer !== 'object') return undefined
  const a = answer as Record<string, unknown>
  const isNode = (v: unknown): v is UIPageNode => !!v && typeof v === 'object' && typeof (v as { kind?: unknown }).kind === 'string'
  if ('tree' in a) {
    if (isNamedSpec(a.tree) || !isNode(a.tree)) return undefined
    return { node: a.tree, data: a.data }
  }
  if ('dslTree' in a) return isNode(a.dslTree) ? { node: a.dslTree, data: a.data } : undefined
  return isNode(a) ? { node: a } : undefined
}

/** placeholderShape is what a placeholder draws: its kind and how many. */
export function placeholderShape(ph: SectionProps['placeholder']): { kind: 'skeleton' | 'cards' | 'rows'; count: number } {
  const kind = ph?.kind === 'cards' || ph?.kind === 'rows' ? ph.kind : 'skeleton'
  const count = Math.max(1, Math.min(12, ph?.count ?? (kind === 'skeleton' ? 3 : 3)))
  return { kind, count }
}

function Placeholder({ ph, id }: { ph: SectionProps['placeholder']; id: string }): ReactNode {
  const { kind, count } = placeholderShape(ph)
  const n = Array.from({ length: count }, (_, i) => i)
  if (kind === 'cards') {
    return (
      <Group gap="sm" data-section={id} data-state="loading">
        {n.map((i) => <Card key={i} withBorder padding="sm" w={220}><Skeleton height={12} width="60%" mb={8} /><Skeleton height={10} /></Card>)}
      </Group>
    )
  }
  if (kind === 'rows') {
    return <Stack gap={6} data-section={id} data-state="loading">{n.map((i) => <Skeleton key={i} height={18} />)}</Stack>
  }
  return (
    <Stack gap="xs" data-section={id} data-state="loading">
      {n.map((i) => <Skeleton key={i} height={i === 0 ? 14 : 10} width={i === 0 ? '40%' : i % 2 ? '100%' : '80%'} />)}
    </Stack>
  )
}

/** PushedSection is a section the producer of a page panel pushes:
 *  its placeholder until the producer's replace swaps it,
 *  by its id, for what it built. */
export function PushedSection({ id, placeholder, attrs }: { id: string; placeholder: SectionProps['placeholder']; attrs?: Record<string, string> }): ReactNode {
  return <div {...attrs} data-section={id} data-state="pushed"><Placeholder ph={placeholder} id={id} /></div>
}

/** The nested page a section draws its answer with (UIPageView, passed in to
 *  break the import cycle). */
export type SectionPageView = ComponentType<{ node: UIPageNode; data?: unknown; onAction?: (e: UIPageActionEvent) => void }>

/** What a section's load answered; `loads` counts the answers read, so the
 *  nodes of its page tell a section read again. */
interface Got { content?: { node: UIPageNode; data?: unknown }; error?: string; done: boolean; loads?: number }

/** resolveSelectionArgs fills a section's load args against the page's
 *  selection (`${selection.<key>}`). */
export function resolveSelectionArgs(args: Record<string, unknown> | undefined, selection: Record<string, unknown>): Record<string, unknown> {
  return resolveActionArgs(args ?? {}, undefined, { selection })
}

const SELECTION_REF = /\$\{\s*selection\.([^.}\s]+)[^}]*\}/g

/** selectionKeys are the selection keys a section's load args read. */
export function selectionKeys(args: Record<string, unknown> | undefined): string[] {
  const keys = new Set<string>()
  const walk = (v: unknown): void => {
    if (typeof v === 'string') for (const m of v.matchAll(SELECTION_REF)) keys.add(m[1])
    else if (Array.isArray(v)) v.forEach(walk)
    else if (v && typeof v === 'object') Object.values(v).forEach(walk)
  }
  walk(args)
  return [...keys].sort()
}

/** selectionReady reports whether every selection key a section's load args
 *  read is set: a section whose key is unset shows its empty text and loads
 *  nothing. */
export function selectionReady(args: Record<string, unknown> | undefined, selection: Record<string, unknown>): boolean {
  return selectionKeys(args).every((k) => selection[k] !== undefined && selection[k] !== null && selection[k] !== '')
}

/** sectionLoad is what a section loads for the page's selection:
 *  its load with its args read against the selection
 *  (`${selection.<key>}`), and `at`, the identity of the read - the section
 *  loads again only when `at` changes (or the outer page's data is re-read),
 *  so a change of a key its args do not read leaves it as it is. Undefined
 *  while a key it reads is unset. */
export function sectionLoad(load: SectionProps['load'], selection: Record<string, unknown>): { at: string; load: SectionProps['load'] } | undefined {
  if (!selectionReady(load.args, selection)) return undefined
  const args = resolveSelectionArgs(load.args, selection)
  return { at: `${load.method}|${JSON.stringify(args)}`, load: { ...load, args } }
}

/** retryLabel is the error card's button in the person's language. */
export function retryLabel(language: string | undefined): string {
  return (language ?? '').startsWith('fr') ? 'Réessayer' : 'Try again'
}

/** sectionWatches says whether a section re-reads on object changes: it
 *  declares a refresh whose scope is not none. */
export function sectionWatches(refresh: SectionProps['refresh']): boolean {
  return !!refresh && !!refresh.scope && refresh.scope !== 'none'
}

/** SectionFailed is a failed section's error card: its message
 *  and « Réessayer », only that section - the rest of the page stays. */
export function SectionFailed({ id, message, onRetry, attrs }: { id: string; message: string; onRetry: () => void; attrs?: Record<string, string> }): ReactNode {
  return (
    <Alert {...attrs} color="red" variant="light" data-section={id} data-state="failed">
      <Group justify="space-between" wrap="nowrap" gap="sm">
        <Text size="sm" style={{ minWidth: 0 }}>{message}</Text>
        <Button size="xs" variant="light" color="red" onClick={onRetry} data-section-retry={id}>{retryLabel(i18next.language)}</Button>
      </Group>
    </Alert>
  )
}

/** sectionReads runs a section's reads of one part one at a time:
 *  `ask` starts a read, or - while one runs -
 *  holds one, run once when it ends however many were asked meanwhile. A
 *  re-read never cancels the read in flight: a part asked again faster than
 *  its read lasts (a busy module re-reading the page around it) still
 *  answers. `stop` (another part, the section gone) ends the read in flight
 *  through its signal and holds nothing. */
export interface SectionReads { ask(): void; stop(): void; holding(): boolean }
export function sectionReads(read: (signal: AbortSignal) => Promise<unknown>): SectionReads {
  const ctl = new AbortController()
  let running = false
  let held = false
  const go = () => {
    running = true
    held = false
    read(ctl.signal).catch(() => {}).finally(() => {
      running = false
      if (held && !ctl.signal.aborted) go()
    })
  }
  return {
    ask() { if (ctl.signal.aborted) return; if (running) held = true; else go() },
    stop() { held = false; ctl.abort() },
    holding: () => held,
  }
}

export function SectionRendererLoad({ node, Page }: { node: UIPageNode & SectionProps; Page: SectionPageView }): ReactNode {
  const { loadSection, watchChanges } = useHostServices()
  // Each round loads the section again: a change its refresh matches, or
  // « Réessayer »; the content shown stays until the new answer.
  const [round, setRound] = useState(0)
  const outer = usePageContext()
  const when = sectionWhen(node)
  const canObserve = typeof IntersectionObserver !== 'undefined'
  const box = useRef<HTMLDivElement>(null)
  const [inView, setInView] = useState(false)
  const [got, setGot] = useState<Got>({ done: false })
  useEffect(() => {
    if (when !== 'visible' || !canObserve || inView || !box.current) return
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting)) { setInView(true); io.disconnect() }
    })
    io.observe(box.current)
    return () => io.disconnect()
  }, [when, canObserve, inView])
  const now = sectionLoadsNow(when, inView, canObserve)
  // Its args read the page's selection: it waits while a key is unset and
  // loads again when what it reads changes.
  // It re-reads too when the outer page's data is re-read (revision).
  const { selection, revision } = useSelection()
  const resolved = sectionLoad(node.load, selection)
  const loadKey = resolved?.at ?? ''
  // One read of the part at a time: a re-read asked
  // while one runs - the outer page read again, a change its refresh
  // matches, « Réessayer » - is held and run once when it ends; only another
  // part, or the section gone, cancels the read in flight.
  const read = useRef<(signal: AbortSignal) => Promise<unknown>>(() => Promise.resolve())
  read.current = (signal) => (loadSection && resolved ? loadSection(resolved.load, signal) : Promise.resolve(undefined)).then(
    (answer) => { if (!signal.aborted) setGot((was) => ({ content: sectionContent(answer), done: true, loads: (was.loads ?? 0) + 1 })) },
    (e: unknown) => { if (!signal.aborted) setGot((was) => ({ error: e instanceof Error ? e.message : String(e), done: true, loads: was.loads })) },
  )
  const reads = useRef<SectionReads | null>(null)
  const asked = useRef('')
  const askKey = `${round}|${revision}`
  useEffect(() => {
    if (!now || !loadSection || !resolved) return
    const part = sectionReads((signal) => read.current(signal))
    reads.current = part
    asked.current = askKey
    part.ask()
    return () => { part.stop(); if (reads.current === part) reads.current = null }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [now, loadSection, loadKey])
  useEffect(() => {
    if (asked.current === askKey) return
    asked.current = askKey
    reads.current?.ask()
  }, [askKey])
  // Once loaded, a section declaring a refresh re-reads on its own when an
  // object change it matches is over; a newer round aborts the older load.
  const refreshKey = JSON.stringify(node.refresh ?? null)
  useEffect(() => {
    if (!got.done || !watchChanges || !sectionWatches(node.refresh)) return
    return watchChanges(node.refresh!, () => setRound((r) => r + 1))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [got.done, watchChanges, refreshKey])
  const own = dataAttrs(node)
  if (!resolved) {
    const empty = (node as { empty?: string }).empty
    return <div {...own} ref={box} data-section={node.id} data-state="waiting">{empty ? <Text size="sm" c="dimmed">{empty}</Text> : null}</div>
  }
  if (!got.done) {
    return <div {...own} ref={box} data-section={node.id}><Placeholder ph={node.placeholder} id={node.id} /></div>
  }
  if (got.error) {
    return <SectionFailed attrs={own} id={node.id} message={got.error} onRetry={() => { setGot((was) => ({ done: false, loads: was.loads })); setRound((r) => r + 1) }} />
  }
  if (!got.content) return <div {...own} data-section={node.id} data-state="empty" />
  const content = { ...got.content.node, id: node.id } as UIPageNode
  return (
    <div {...own} data-section={node.id} data-state="loaded">
      <SectionLoadContext.Provider value={got.loads ?? 0}>
        <Page node={content} data={got.content.data} onAction={outer.hasHost ? outer.onAction : undefined} />
      </SectionLoadContext.Provider>
    </div>
  )
}
