// SPDX-License-Identifier: Apache-2.0
// Icon — thin wrapper over Tabler React icons with name-based dispatch.
//
// Ported from _ktw_workspace/old_ui_main/src/modules/core/cpm-core/icons/Icon.tsx
// during gui-form Subtask 1 (2026-05-03). The original cpm-core/icons folder
// also shipped IconSelector.tsx (admin form picker) — that's NOT ported here
// because the form composition doesn't need it. Re-port if a future
// "icon-picker" content type lands.

import type React from 'react'
import type { IconName } from './types.ts'
import { tablerComponentName, useTablerIconSet } from './byName.tsx'

interface IconProps extends React.ComponentProps<'svg'> {
  /** Tabler icon name, e.g., 'IconPlus' / 'IconTrash' / 'IconPencil'. */
  icon: IconName
  /** Edge length in px. Default 24. */
  size?: number
  /** CSS color string. Default = inherit. */
  color?: string
}

/**
 * Render a Tabler icon by name. Renders nothing for an unknown name, and
 * nothing while the icon set loads (byName.tsx) — callers that want a
 * fallback use tablerIcon(name, fallback) instead.
 */
export const Icon: React.FC<IconProps> = ({ icon, size = 24, color, ...rest }) => {
  const IconComponent = useTablerIconSet()?.[tablerComponentName(icon)] as React.ElementType | undefined
  if (!IconComponent) return null
  return <IconComponent size={size} color={color} {...rest} />
}
