// SPDX-License-Identifier: Apache-2.0
// QuickActionsGrid — responsive grid of QuickActionButton.
//
// Ported from _ktw_workspace/.../cpm-core/actions/quick-actions/QuickActionsGrid.tsx
// during gui-action Subtask 2 (2026-05-04). No semantic changes.

import { SimpleGrid } from '@mantine/core'
import type { FC } from 'react'

import type { UITexts } from '../../i18n/index.ts'
import type { UIAction } from '../types.ts'
import { QuickActionButton } from './QuickActionButton.tsx'

interface QuickActionsGridProps {
  actions: UIAction[]
  onActionClick: (action: UIAction) => void
  texts: UITexts
}

export const QuickActionsGrid: FC<QuickActionsGridProps> = ({ actions, onActionClick, texts }) => {
  return (
    <SimpleGrid cols={{ base: 1, xs: 2, sm: 3, md: 4 }} spacing="sm">
      {actions.map((action) => (
        <QuickActionButton
          key={action.id}
          action={action}
          onClick={() => onActionClick(action)}
          texts={texts}
        />
      ))}
    </SimpleGrid>
  )
}
