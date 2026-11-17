// SPDX-License-Identifier: Apache-2.0
// UIViewView — React component that renders a built UIView.
//
// Subtask 1 of gui-view (2026-05-04). Differences vs UIFormView:
//   - No react-hook-form (read-only; nothing to register)
//   - No submit/cancel/reset action wiring
//   - NEW: detail-level toggle in the header (cycles 1→2→3 on click;
//     clamps which fields render via the per-field `info.fieldOptions.detail`)
//   - NEW: empty-value guard wraps each rendered field per the
//     `defaultEmptyStrategy` prop (omit / em-dash / explicit-empty)
//
// The legacy `vol-view.tsx` was effectively a single layout — the new
// 5-layout system means each layout file owns its visual shape; this
// component owns ONLY the chrome (title + detail-level toggle + buttons
// row + empty-value strategy plumbing).

import { ActionIcon, Box, Group, Stack, Title, Tooltip } from '@mantine/core'
import { IconAdjustments } from '@tabler/icons-react'
import { useState } from 'react'

import type { UIActionProcessor } from '../../ui-core/action/index.ts'
import type { UIView, UIViewEmptyStrategy } from './ui-view.types.ts'

export type UIViewViewProps<T = any> = {
  view: UIView<T>
  item?: T
  /** Initial detail level. User can cycle via the header toggle. Default: 2. */
  detailLevel?: 1 | 2 | 3
  /** Show the detail-level toggle button in the header. Default: true. */
  enableDetailLevelToggle?: boolean
  /** Strategy for fields whose value is null/undefined/empty.
   *  Default: `'em-dash'`. */
  defaultEmptyStrategy?: UIViewEmptyStrategy
  /** Optional action processor — fires on item-scope action clicks
   *  (edit/delete/etc.). */
  actionProcessor?: UIActionProcessor
  /** Suppress the form-level title row (and detail-level toggle along
   *  with it). Hosts that provide their own outer chrome (e.g. NodeTab
   *  with its own page header) pass `hideTitle` to avoid the redundant
   *  "View : <row>" heading. Matches UIFormView's `hideTitle` semantics
   *  (cwm-v3 US-4 close-out, 2026-06-08). Default: false. */
  hideTitle?: boolean
  /** Extra attributes on the root element (a page node's data-* keys); the view's own win. */
  attrs?: Record<string, string>
}

/**
 * UIViewView — renders a `UIView` (built via `buildUIView()`) into a
 * complete read-only display. Detail-level toggle + empty-value strategy
 * are owned here; the layout factory just renders the groups.
 */
export function UIViewView<T = any>({
  view,
  item,
  detailLevel: initialDetailLevel = 2,
  enableDetailLevelToggle = true,
  defaultEmptyStrategy = 'em-dash',
  actionProcessor,
  hideTitle = false,
  attrs,
}: UIViewViewProps<T>) {
  const [detailLevel, setDetailLevel] = useState<1 | 2 | 3>(initialDetailLevel)
  // A `slots` view is a compact card or header: it carries no view heading.
  const bare = hideTitle || view.definition.layout === 'slots'

  const title = (() => {
    const detailTitle = view.texts.t('viewTitle', { defaultValue: 'View' })
    if (item && view.rowId(item)) return `${detailTitle} : ${view.rowTitle(item)}`
    return detailTitle
  })()

  const cycleDetailLevel = () => {
    setDetailLevel((cur) => (cur === 3 ? 1 : ((cur + 1) as 1 | 2 | 3)))
  }

  return (
    <Stack {...attrs} gap="md">
      {!bare && (
        <Group justify="space-between">
          <Title order={1}>{title}</Title>
          {enableDetailLevelToggle && (
            <Tooltip label={`Detail level ${detailLevel} of 3 — click to cycle`}>
              <ActionIcon variant="default" size="lg" onClick={cycleDetailLevel} aria-label="Cycle detail level">
                <IconAdjustments size={18} />
                <Box component="span" ml={4} style={{ fontSize: 11, fontWeight: 600 }}>
                  L{detailLevel}
                </Box>
              </ActionIcon>
            </Tooltip>
          )}
        </Group>
      )}
      {view.renderer({
        item,
        actionProcessor,
        detailLevel,
        defaultEmptyStrategy,
      })}
    </Stack>
  )
}
