// SPDX-License-Identifier: Apache-2.0
/**
 * ObjectBadges — an object's header badges, as CWM resolved them: its own
 * (set by a capability or a person) and the ones its type and capabilities
 * derive from its fields (the lifecycle's state, …). Each with its colour, its
 * icon and its tooltip; the capability that set it is named on hover.
 */
import { Badge, Group, Tooltip } from '@mantine/core'
import { IconPoint } from '@tabler/icons-react'
import { tablerIcon } from '@ktw/display-lib/ui-core/components/icons/index.ts'
import { main } from '../../../wailsjs/go/models'
import { useT } from '../../i18n'
import { ctBadge, useCT } from '../../i18n/contrib'

export function ObjectBadges({ badges, size = 'sm', objectType, guide }: {
  badges?: main.WorkspaceBadge[]; size?: 'xs' | 'sm' | 'md'
  /** The object's type: a badge its type declares is translated by it. */
  objectType?: string
  /** The group's element id (data-guide), where a walk reads it. */
  guide?: string
}) {
  const t = useT('workspace')
  const ct = useCT()
  if (!badges || badges.length === 0) return null
  return (
    <Group gap={6} wrap="nowrap" data-guide={guide}>
      {badges.map((b) => {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const Icon: any = b.icon ? tablerIcon(b.icon, IconPoint) : null
        // Its prefix and tooltip in the GUI language (its declarer's translations).
        const shown = ctBadge(ct, b, objectType)
        const tip = [shown.tooltip, b.capability && t('badges.from', { capability: b.capability })].filter(Boolean).join(' — ')
        const badge = (
          <Badge
            size={size}
            variant="light"
            color={b.color || 'gray'}
            leftSection={Icon ? <Icon size={12} /> : undefined}
            style={{ textTransform: 'none', flexShrink: 0 }}
          >
            {shown.label}
          </Badge>
        )
        return tip ? (
          <Tooltip key={b.id} label={tip} withArrow>{badge}</Tooltip>
        ) : (
          <span key={b.id}>{badge}</span>
        )
      })}
    </Group>
  )
}
