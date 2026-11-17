// SPDX-License-Identifier: Apache-2.0
/**
 * SectionCard — a titled panel, the same in every view: a bordered card, its
 * heading with an icon badge in the section's colour, a dimmed caption under
 * it, actions on the right of the heading, then its content. The planification
 * deck's sections and the hand-written consoles share this look.
 */

import type { ReactNode } from 'react'
import { Card, Group, Stack, Text, ThemeIcon, Title } from '@mantine/core'
import { IconFile } from '@tabler/icons-react'
import { tablerIcon } from '../icons/index.ts'

export interface SectionCardProps {
  title: ReactNode
  /** A Tabler icon name (kebab). */
  icon?: string
  /** A Mantine colour name for the icon badge. */
  color?: string
  /** The dimmed line under the heading. */
  description?: ReactNode
  /** Controls on the right of the heading. */
  actions?: ReactNode
  /** Dims the whole card (a paused group, a section that is off). */
  dimmed?: boolean
  children?: ReactNode
  'aria-label'?: string
}

export function SectionCard({ title, icon, color, description, actions, dimmed, children, ...rest }: SectionCardProps) {
  const Icon = icon ? tablerIcon(icon, IconFile) : undefined
  return (
    <Card withBorder padding="lg" radius="md" style={{ minWidth: 0, opacity: dimmed ? 0.72 : undefined }} aria-label={rest['aria-label']}>
      <Stack gap="sm">
        <Group justify="space-between" wrap="wrap" gap="xs">
          <Group gap="sm" wrap="nowrap" style={{ minWidth: 0 }}>
            {Icon && (
              <ThemeIcon variant="light" color={color ?? 'blue'} radius="md" size="md">
                <Icon size={16} />
              </ThemeIcon>
            )}
            <Title order={5}>{title}</Title>
          </Group>
          {actions && <Group gap="xs" wrap="nowrap">{actions}</Group>}
        </Group>
        {description && <Text size="xs" c="dimmed">{description}</Text>}
        {children}
      </Stack>
    </Card>
  )
}
