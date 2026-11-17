// SPDX-License-Identifier: Apache-2.0
// Group renderer — lays out one view section's fields in a SimpleGrid
// with each field's value displayed read-only.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-view/layouts/vol-group.tsx
// during gui-view Subtask 1 (2026-05-04). Adapted for the new src/ui/
// layout: `@core/cpm-ui/ui-fields` → `'../../../../ui-core/fields/index.ts'`,
// `@core/types` → `'../../../../types/json-types.ts'`. Renamed group cols
// param to match DSL §9.2: `nbCols` → `cols`. Reuses gui-form's
// `FieldRendererWrapper` pattern via a local copy (the form's wrapper is
// not exported; the trivial 3-line component is duplicated here).

import { Box, SimpleGrid, Stack, Text } from '@mantine/core'
import type { UIViewFieldRenderer } from '../../../fields/index.ts'
import type { JsonObject } from '../../../../types/json-types.ts'
import type { UIViewFields, UIViewGroupRenderer } from '../../ui-view.types.ts'

// ============================================================================
// FieldRendererWrapper — proper React component for renderer functions
// ============================================================================
//
// UIViewFieldRenderer can use hooks internally (e.g., useTexts). Wrapping
// the call in a stable component lets React reconcile hook order across
// renders.

interface FieldRendererWrapperProps {
  field: UIViewFieldRenderer
  item: any
}

function FieldRendererWrapper({ field, item }: FieldRendererWrapperProps) {
  return <>{field(item)}</>
}

// ============================================================================
// vol_group_layout — produces a UIViewGroupRenderer from a UIViewFields def
// ============================================================================

export function vol_group_layout(fieldsDef: UIViewFields): UIViewGroupRenderer {
  return (props) => {
    const { item = {}, detailLevel = 2 } = props
    const { cols = 1, title } = fieldsDef
    const fields = fieldsDef.getFieldsToRender(item as JsonObject, detailLevel)

    // No outer Paper border — the host (NodeTab card, FormWidgetPanel
    // card, etc.) already provides one. Doubling it was the source of
    // the "view feels heavier than the form" feedback (cwm-v3 US-4
    // close-out, 2026-06-08). The group title becomes an underlined
    // section header so groups read as a clean definition list; fields
    // stack as aligned label/value rows directly underneath.
    return (
      <Stack gap="xs">
        {title && (
          <Text
            size="sm"
            fw={600}
            c="dimmed"
            style={{
              paddingBottom: 4,
              borderBottom: '1px solid var(--mantine-color-default-border)',
            }}
          >
            {title}
          </Text>
        )}
        <SimpleGrid cols={cols} spacing="md" verticalSpacing="xs">
          {fields.map((field, index) => (
            <Box key={`field-${index}`}>
              <FieldRendererWrapper field={field} item={item} />
            </Box>
          ))}
        </SimpleGrid>
      </Stack>
    )
  }
}
