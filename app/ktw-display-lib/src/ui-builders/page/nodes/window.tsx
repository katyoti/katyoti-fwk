// SPDX-License-Identifier: Apache-2.0
// Drawing only what is seen: on a progressive page (the
// host's flag) a list, a tree, a lane or a grid cell draws its
// first page of items and an « Afficher plus » button that adds the next one;
// tabs and accordions mount only what is open. With the flag off every item
// draws, as before. Paging rather than measuring the screen keeps it the same
// on every surface: nothing depends on a layout the host may not have.

import { Button } from '@mantine/core'
import i18next from 'i18next'
import { useState } from 'react'

import { useProgressive } from '../../../ui-core/host/index.tsx'

/** How many items a page shows when its node names no pageSize. */
export const DEFAULT_PAGE_SIZE = 50

/** windowSize is how many of total items are drawn after `pages` pages of
 *  pageSize: all of them when not progressive. */
export function windowSize(total: number, progressive: boolean, pageSize: number | undefined, pages: number): number {
  if (!progressive) return total
  const size = pageSize && pageSize > 0 ? Math.floor(pageSize) : DEFAULT_PAGE_SIZE
  return Math.min(total, size * Math.max(1, pages))
}

/** showMoreLabel is the button's label in the person's language, with how
 *  many items are left when that is known. */
export function showMoreLabel(language: string | undefined, rest?: number): string {
  const n = rest !== undefined && rest > 0 ? ` (${rest})` : ''
  return (language ?? '').startsWith('fr') ? `Afficher plus${n}` : `Show more${n}`
}

/** showLessLabel is the label of the toggle that folds back what « Afficher
 *  plus » opened, in the person's language. */
export function showLessLabel(language: string | undefined): string {
  return (language ?? '').startsWith('fr') ? 'Afficher moins' : 'Show less'
}

/** panelMounted says whether a tab panel or an accordion item draws its
 *  content: always as before, only when open on a progressive page. */
export function panelMounted(progressive: boolean, open: boolean): boolean {
  return !progressive || open
}

/** useWindow is one list's window: how many to draw and the next page.
 *  `always` windows on every host, progressive or not (a table: its pager
 *  windowed hostless too). */
export function useWindow(total: number, pageSize: number | undefined, always = false): { shown: number; more: () => void } {
  const progressive = useProgressive()
  const [pages, setPages] = useState(1)
  return { shown: windowSize(total, progressive || always, pageSize, pages), more: () => setPages((p) => p + 1) }
}

/** useWindows is the windows of several lists by key (a board's lanes or
 *  cells): how many each draws and its next page. */
export function useWindows(pageSize: number | undefined): { shown: (key: string, total: number) => number; more: (key: string) => void } {
  const progressive = useProgressive()
  const [pages, setPages] = useState<Record<string, number>>({})
  return {
    shown: (key, total) => windowSize(total, progressive, pageSize, pages[key] ?? 1),
    more: (key) => setPages((p) => ({ ...p, [key]: (p[key] ?? 1) + 1 })),
  }
}

/** ShowMore is the « Afficher plus » button under a window, when items are left. */
export function ShowMore({ rest, onMore, name }: { rest: number; onMore: () => void; name?: string }) {
  if (rest <= 0) return null
  return (
    <Button size="xs" variant="subtle" onClick={onMore} data-show-more={name ?? ""}>
      {showMoreLabel(i18next.language, rest)}
    </Button>
  )
}
