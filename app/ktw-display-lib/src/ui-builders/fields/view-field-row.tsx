// SPDX-License-Identifier: Apache-2.0
// ViewFieldRow — central label + description framing for view-mode
// fields. Mirrors what Mantine's `<TextInput label= description=>`
// emits on the form side so view/form density and visual hierarchy
// match.
//
// Applied centrally by `createViewRenderer` in `./orchestrator.ts` —
// per-content-type view renderers stay focused on rendering the bare
// value; this wrapper adds the label + optional description above it.
//
// cwm-v3 US-4 close-out (2026-06-08): view fields previously rendered
// as naked text with no context. Labels make multi-field records
// readable without forcing each renderer to repeat label-emission
// boilerplate.

import { Group, Text, Tooltip } from '@mantine/core'
import type { ReactNode } from 'react'
import type { JsonObject } from '../../types/json-types.ts'
import type { UIViewFieldRenderer } from './types.ts'

interface ViewFieldRowProps {
  label: string
  description?: string
  children: ReactNode
}

/** Horizontal label/value row. Label sits on the left at a fixed
 *  inline-size so multiple rows align as a definition list; the value
 *  fills the rest. When `description` is set the label gets a Tooltip
 *  (descriptions are an input-side affordance, so the view surface
 *  hides them by default to keep the read-only display dense). */
export function ViewFieldRow({ label, description, children }: ViewFieldRowProps) {
  const labelNode = (
    <Text
      size="sm"
      fw={500}
      c="dimmed"
      style={{
        flex: '0 0 160px',
        cursor: description ? 'help' : undefined,
      }}
    >
      {label}
    </Text>
  )
  return (
    <Group gap="md" align="flex-start" wrap="nowrap">
      {description ? (
        <Tooltip label={description} withArrow multiline w={320}>
          {labelNode}
        </Tooltip>
      ) : (
        labelNode
      )}
      <div style={{ flex: '1 1 auto', minWidth: 0 }}>{children}</div>
    </Group>
  )
}

/** Wrap a bare view-field renderer with the central label + description
 *  framing. Called by `createViewRenderer` in `./orchestrator.ts`
 *  (which is `.ts`, not `.tsx`, so the JSX wrap has to live here). */
export function wrapWithViewFieldRow<TData extends JsonObject>(
  inner: UIViewFieldRenderer<TData>,
  label: string,
  description?: string,
): UIViewFieldRenderer<TData> {
  return (item: TData) => (
    <ViewFieldRow label={label} description={description}>
      {inner(item)}
    </ViewFieldRow>
  )
}
