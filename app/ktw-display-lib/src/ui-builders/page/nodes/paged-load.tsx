// SPDX-License-Identifier: Apache-2.0
// A table that pages from the server: on a progressive page
// (the host's flag) a table whose `data` is `{load: {method, args},
// pageSize}` asks its method one page at a time through the host's
// callMethod - render.PageRequest `{limit, offset | cursor}` in,
// render.PagedItems `{items, page}` out - drawing only the rows
// it has loaded, the next page asked when its end comes into view or on
// « Afficher plus ». Elsewhere such a table has no rows to draw.

import { Button, Group, Text } from '@mantine/core'
import i18next from 'i18next'
import { useEffect, useRef, useState } from 'react'

import { useHostServices, useProgressive } from '../../../ui-core/host/index.tsx'
import { DEFAULT_PAGE_SIZE, showMoreLabel } from './window.tsx'

/** A paged table's data: the method that answers its pages. */
export interface PagedSource {
  load: { method: string; args?: Record<string, unknown> }
  pageSize?: number
}

/** The page envelope a paged load answers (render.Pagination). */
export interface PageInfo {
  total?: number
  pageSize?: number
  offset?: number
  hasMore?: boolean
  nextCursor?: string
}

/** pagedSource reads a node's data as a paged source, or undefined. */
export function pagedSource(data: unknown): PagedSource | undefined {
  if (!data || typeof data !== 'object' || Array.isArray(data)) return undefined
  const load = (data as { load?: { method?: unknown } }).load
  return load && typeof load.method === 'string' && load.method !== '' ? (data as PagedSource) : undefined
}

/** pageRequest is the input of the next page's load: its args, then limit and
 *  where to start - the cursor the last page gave, else the rows loaded. */
export function pageRequest(src: PagedSource, loaded: number, cursor?: string): Record<string, unknown> {
  const limit = src.pageSize && src.pageSize > 0 ? Math.floor(src.pageSize) : DEFAULT_PAGE_SIZE
  return { ...(src.load.args ?? {}), limit, ...(cursor ? { cursor } : { offset: loaded }) }
}

/** The rows a paged table holds and where its paging stands. */
export interface PagedState {
  rows: unknown[]
  page?: PageInfo
  done: boolean
}

/** mergePage adds one answered page to what is held: its items after the
 *  rows, its envelope kept; an answer with no more page ends the paging. */
export function mergePage(prev: PagedState, answer: unknown): PagedState {
  const a = (answer ?? {}) as { items?: unknown[]; page?: PageInfo }
  const items = Array.isArray(a.items) ? a.items : []
  const page = a.page ?? {}
  const rows = [...prev.rows, ...items]
  const more = page.hasMore ?? (page.total !== undefined ? rows.length < page.total : false)
  return { rows, page, done: !more || items.length === 0 }
}

/** usePagedRows loads a paged source's pages on a progressive page whose host
 *  can call methods: the rows so far, a function asking the next page, and
 *  whether it can page at all. */
export function usePagedRows(data: unknown) {
  const progressive = useProgressive()
  const { callMethod } = useHostServices()
  const src = pagedSource(data)
  const on = !!(src && progressive && callMethod)
  const [state, setState] = useState<PagedState>({ rows: [], done: false })
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string>()
  const key = JSON.stringify(src ?? null)
  const asked = useRef(0)
  const more = () => {
    if (!on || loading || state.done || !src || !callMethod) return
    const ticket = ++asked.current
    setLoading(true)
    callMethod(src.load.method, pageRequest(src, state.rows.length, state.page?.nextCursor)).then(
      (answer) => { if (ticket === asked.current) { setState((s) => mergePage(s, answer)); setError(undefined) } },
      (e: unknown) => { if (ticket === asked.current) setError(e instanceof Error ? e.message : String(e)) },
    ).finally(() => { if (ticket === asked.current) setLoading(false) })
  }
  // A changed source starts again from its first page.
  useEffect(() => { asked.current++; setState({ rows: [], done: false }); setLoading(false); setError(undefined) }, [key])
  useEffect(() => { if (on && state.rows.length === 0 && !state.done && !loading && !error) more() })
  return { on, rows: state.rows, page: state.page, done: state.done, loading, error, more }
}

/** PagedFooter asks the next page when it comes into view, and offers
 *  « Afficher plus » for a surface that cannot tell. */
export function PagedFooter({ rest, done, loading, error, onMore, name }: {
  rest?: number; done: boolean; loading: boolean; error?: string; onMore: () => void; name?: string
}) {
  const box = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (done || loading || error || typeof IntersectionObserver === 'undefined' || !box.current) return
    const io = new IntersectionObserver((es) => { if (es.some((e) => e.isIntersecting)) onMore() })
    io.observe(box.current)
    return () => io.disconnect()
  })
  if (done && !error) return null
  return (
    <Group ref={box} justify="center" py={6} data-paged-more={name ?? ''}>
      {error ? <Text size="xs" c="red">{error}</Text> : null}
      <Button size="xs" variant="subtle" loading={loading} onClick={onMore}>
        {showMoreLabel(i18next.language, rest)}
      </Button>
    </Group>
  )
}
