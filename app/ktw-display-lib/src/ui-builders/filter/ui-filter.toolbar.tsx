// SPDX-License-Identifier: Apache-2.0
// UIFilterToolbar — the chrome around tab filter + toolbar filters +
// reset button. Renders inside a Mantine Paper above the table.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-filter/ui-filter.toolbar.tsx
// during gui-table Subtask 1 (2026-05-04). Adaptations:
//   - `@core/cpm-core/forms` → `'react-hook-form'`
//   - Default `filterResetButton` text passed in by the hook (i18n-aware)

import { Box, Button, Group, Paper } from '@mantine/core'
import { IconTrash } from '@tabler/icons-react'
import type { ReactNode } from 'react'

import { FormProvider, type UseFormReturn } from 'react-hook-form'
import type { UIFilter, UIFilterValues } from './ui-filter.types.ts'

type Props = {
  methods: UseFormReturn<UIFilterValues>
  tabFilter: UIFilter | undefined
  toolbarFilters: UIFilter[]
  isFiltered: boolean
  filterResetButton?: string
  onResetFilter?: () => void
  children?: ReactNode
}

export function UIFilterToolbar({
  isFiltered,
  tabFilter,
  toolbarFilters = [],
  methods,
  filterResetButton = 'Reset filters',
  onResetFilter,
  children,
}: Props) {
  if (!tabFilter && (!toolbarFilters || toolbarFilters.length === 0)) return null

  const handleResetClick = () => {
    if (onResetFilter) onResetFilter()
    else methods.reset()
  }

  // Tab + toolbar render with rounded bottom-only corners (visually merged).
  const borderRadius = tabFilter
    ? { borderTopLeftRadius: 0, borderTopRightRadius: 0 }
    : { borderRadius: 4 }

  return (
    <FormProvider {...methods}>
      {tabFilter && tabFilter.renderer()}
      {/* Background uses light-dark() so the filter row gets a subtle pale-blue
          tint in light mode and a slightly-elevated dark surface in dark mode.
          Hardcoding `bg="blue.0"` left dark mode unreadable. */}
      <Paper
        withBorder
        p="sm"
        style={{
          ...borderRadius,
          background: 'light-dark(var(--mantine-color-blue-0), var(--mantine-color-dark-6))',
        }}
      >
        <Group justify="space-between">
          <Group gap="sm">
            {toolbarFilters.map((filter) => (
              <Box key={filter.name}>{filter.renderer()}</Box>
            ))}
            {isFiltered && (
              <Button
                key="filter-reset-button"
                color="red"
                onClick={handleResetClick}
                leftSection={<IconTrash size={16} />}
                style={{ flexShrink: 0 }}
              >
                {filterResetButton}
              </Button>
            )}
          </Group>
          {children}
        </Group>
      </Paper>
    </FormProvider>
  )
}
