// SPDX-License-Identifier: Apache-2.0
// Group renderer — lays out one form section's fields in a SimpleGrid.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-form/layouts/fol-group.tsx
// during gui-form Subtask 3 (2026-05-03). Adapted for the new src/ui/
// layout: `@core/cpm-ui/ui-fields` → `'../../../../ui-core/fields/index.ts'`,
// `@core/types` → `'../../../../types/json-types.ts'`. Renamed group cols
// param to match DSL §9.1: `nbCols` → `cols`.

import { useState, type ReactNode } from 'react'
import { Box, Collapse, Group, Paper, SimpleGrid, Stack, Text, UnstyledButton } from '@mantine/core'
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import type { UIFormFieldRenderer } from '../../../fields/index.ts'
import type { JsonObject } from '../../../../types/json-types.ts'
import type { UIFormFields, UIFormGroupRenderer } from '../../ui-form.types.ts'
import { FormReadRow } from './read-row.tsx'

// ============================================================================
// FieldRendererWrapper — proper React component for renderer functions
// ============================================================================
//
// UIFormFieldRenderer can use hooks internally (e.g., useTexts, useFormContext).
// Wrapping the call in a stable component lets React reconcile the hook order
// across renders. Passing the renderer directly as a child wouldn't work with
// hook-using renderers.

interface FieldRendererWrapperProps {
  field: UIFormFieldRenderer
  item: any
  isNew: boolean
  isSubmitting: boolean
}

function FieldRendererWrapper({ field, item, isNew, isSubmitting }: FieldRendererWrapperProps) {
  return <>{field(item, isNew, isSubmitting)}</>
}

// ============================================================================
// Folding — a group declared collapsible folds under its title
// ============================================================================

/** groupFold says how a group is drawn: foldable (a collapsible group with a
 *  title - without a title there is nothing to click, so it draws open) and
 *  open at first (a foldable group declared collapsed starts folded). */
export function groupFold(fieldsDef: Pick<UIFormFields, 'title' | 'collapsible' | 'collapsed'>): { foldable: boolean; open: boolean } {
  const foldable = !!fieldsDef.collapsible && !!fieldsDef.title
  return { foldable, open: !(foldable && fieldsDef.collapsed) }
}

interface GroupTitleProps {
  title: string
  foldable: boolean
  open: boolean
  onToggle: () => void
  flush: boolean
}

/** GroupTitle draws a group's title: a plain caption, or - foldable - a
 *  button with a chevron that shows / hides the group's fields. */
function GroupTitle({ title, foldable, open, onToggle, flush }: GroupTitleProps) {
  const style = flush
    ? { paddingBottom: 4, borderBottom: '1px solid var(--mantine-color-default-border)' }
    : undefined
  const caption = (
    <Text size="sm" fw={600} c="dimmed" mb={flush ? undefined : open ? 'sm' : undefined} style={foldable ? undefined : style}>
      {title}
    </Text>
  )
  if (!foldable) return caption
  const Chevron = open ? IconChevronDown : IconChevronRight
  return (
    <UnstyledButton
      type="button"
      onClick={onToggle}
      aria-expanded={open}
      data-form-group-toggle=""
      style={{ display: 'block', width: '100%', ...style }}
    >
      <Group gap={4} wrap="nowrap">
        <Chevron size={14} color="var(--mantine-color-dimmed)" />
        {caption}
      </Group>
    </UnstyledButton>
  )
}

interface FormGroupSectionProps {
  fieldsDef: UIFormFields
  framed: boolean
  grid: ReactNode
}

/** FormGroupSection draws one group: its title and its fields, folded under
 *  the title when the group is collapsible. The fields stay mounted while
 *  folded (Collapse hides them), so a folded field keeps its value. */
function FormGroupSection({ fieldsDef, framed, grid }: FormGroupSectionProps) {
  const fold = groupFold(fieldsDef)
  const [open, setOpen] = useState(fold.open)
  const { title } = fieldsDef
  const body = fold.foldable ? <Collapse in={open}>{grid}</Collapse> : grid
  const heading = title ? (
    <GroupTitle title={title} foldable={fold.foldable} open={open} onToggle={() => setOpen((o) => !o)} flush={!framed} />
  ) : null
  const attrs = {
    'data-form-group': title ?? '',
    ...(fold.foldable ? { 'data-folded': open ? 'false' : 'true' } : {}),
  }

  // Multiple groups → bordered Paper per group separates the sections.
  if (framed) {
    return (
      <Paper p="md" withBorder {...attrs}>
        {heading}
        {body}
      </Paper>
    )
  }

  // Single group → flush. Title (if any) is an underlined section header.
  return (
    <Stack gap="sm" {...attrs}>
      {heading}
      {body}
    </Stack>
  )
}

// ============================================================================
// fol_group_layout — produces a UIFormGroupRenderer from a UIFormFields def
// ============================================================================

/**
 * @param framed When true (multiple groups in the form), each group is
 *   wrapped in a bordered Paper so the sections read as visually distinct.
 *   When false (a single group — the common type-specific-fields case),
 *   the group renders flush: the host (NodeEditorTab / ItemEditorTab /
 *   FormWidgetPanel) already provides framing, so a per-group border just
 *   produced a "box in a box".
 *
 * A group declared `collapsible` (with a title) folds under its title, drawn
 * folded at first when declared `collapsed`.
 */
export function fol_group_layout(fieldsDef: UIFormFields, framed = false): UIFormGroupRenderer {
  return (props) => {
    const { isNew, item = {}, isSubmitting } = props
    const { cols = 1 } = fieldsDef
    // `edit: row` - the fields read as rows, each edited behind its pencil.
    if (fieldsDef.edit === 'row' && fieldsDef.getRowsToRender) {
      const rows = fieldsDef.getRowsToRender(isNew, item as JsonObject)
      const list = (
        <Stack gap={0} data-form-rows="">
          {rows.map((row) => (
            <FormReadRow key={row.name} row={row} item={item} isNew={isNew} isSubmitting={isSubmitting} texts={fieldsDef.texts} />
          ))}
        </Stack>
      )
      return <FormGroupSection fieldsDef={fieldsDef} framed={framed} grid={list} />
    }

    const fields = fieldsDef.getFieldsToRender(isNew, item as JsonObject)

    const grid = (
      <SimpleGrid cols={cols} spacing="md">
        {fields.map((field, index) => (
          <Box key={`field-${index}`}>
            <FieldRendererWrapper field={field} item={item} isNew={isNew} isSubmitting={isSubmitting} />
          </Box>
        ))}
      </SimpleGrid>
    )

    return <FormGroupSection fieldsDef={fieldsDef} framed={framed} grid={grid} />
  }
}
