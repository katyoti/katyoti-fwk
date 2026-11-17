// SPDX-License-Identifier: Apache-2.0
/**
 * ObjectErrorsPanel — every error of the universe's project, in the Global
 * tab under Actions. CWM computes an object's errors when it reads it (an
 * invalid object, a state outside its workflow, a link to an object that does
 * not exist); this lists the objects carrying any, each with its errors, and
 * the files CWM could not read at all. A click opens the object, where its
 * Information tab shows the same errors.
 */

import { Alert, Badge, Box, Group, ScrollArea, Stack, Text, ThemeIcon, Title } from '@mantine/core'
import { ItemCard } from '@ktw/display-lib/ui-core/components/other/index.ts'
import { IconAlertTriangle, IconCircleCheck, IconFileAlert } from '@tabler/icons-react'
import type { main } from '../../../wailsjs/go/models'
import { openObjectTab } from '../workspace/openObjectTab'
import { colorForType, iconForType } from '../workspace/type-style'
import { useT } from '../../i18n'

function SectionTitle({ children }: { children: React.ReactNode }) {
  return (
    <Title order={6} mb="xs" c="dimmed" tt="uppercase" style={{ letterSpacing: 0.5 }}>{children}</Title>
  )
}

export function ObjectErrorsPanel({ errors, universeName }: { errors: main.WorkspaceObjectErrors | null; universeName: string }) {
  const t = useT('home')
  if (!errors) return null
  if (errors.error) {
    return (
      <Box p="md">
        <Alert color="red" variant="light" icon={<IconAlertTriangle size={16} />} title={t('errors.unreadable')}>
          {errors.error}
        </Alert>
      </Box>
    )
  }
  const objects = errors.objects ?? []
  const unreadable = errors.unreadable ?? []
  if (objects.length + unreadable.length === 0) {
    return (
      <Group p="md" gap="xs">
        <IconCircleCheck size={16} color="var(--mantine-color-teal-6)" />
        <Text size="sm" c="dimmed">{t('errors.none')}</Text>
      </Group>
    )
  }
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Stack gap="lg" p="md">
        {objects.length > 0 && (
          <Box>
            <SectionTitle>{t('errors.objects', { count: objects.length })}</SectionTitle>
            <Stack gap={6}>
              {objects.map((o) => {
                const Icon = iconForType(o.type ?? '')
                return (
                  <ItemCard key={o.id} tone="red" padding="xs"
                    onClick={() => openObjectTab(universeName, { id: o.id, title: o.title || o.id })}>
                    <Group gap={8} wrap="nowrap" mb={4}>
                      <ThemeIcon size="sm" variant="light" color="red"><Icon size={12} /></ThemeIcon>
                      <Text size="sm" fw={500} c="red" style={{ flex: 1, minWidth: 0 }} truncate>{o.title || o.id}</Text>
                      {o.module && <Badge size="xs" variant="light" color="gray">{o.module}</Badge>}
                      {o.type && (
                        <Badge size="xs" variant="light" color={colorForType(o.type)} style={{ textTransform: 'none' }}>{o.type}</Badge>
                      )}
                    </Group>
                    <Stack gap={2} pl={32}>
                      {(o.errors ?? []).map((d, i) => (
                        <Text key={`${d.code}-${i}`} size="xs">
                          <Text span size="xs" c="dimmed" ff="monospace">{d.code}</Text> {d.label}
                        </Text>
                      ))}
                      <Text size="xs" c="dimmed" ff="monospace" truncate>{o.path}</Text>
                    </Stack>
                  </ItemCard>
                )
              })}
            </Stack>
          </Box>
        )}
        {unreadable.length > 0 && (
          <Box>
            <SectionTitle>{t('errors.files', { count: unreadable.length })}</SectionTitle>
            <Stack gap={6}>
              {unreadable.map((u) => (
                <ItemCard key={u.path} tone="red" padding="xs">
                  <Group gap={8} wrap="nowrap" mb={4}>
                    <ThemeIcon size="sm" variant="light" color="red"><IconFileAlert size={12} /></ThemeIcon>
                    <Text size="sm" ff="monospace" style={{ flex: 1, minWidth: 0 }} truncate>{u.path}</Text>
                  </Group>
                  <Stack gap={2} pl={32}>
                    {(u.problems ?? []).map((p, i) => <Text key={i} size="xs">{p}</Text>)}
                  </Stack>
                </ItemCard>
              ))}
            </Stack>
          </Box>
        )}
      </Stack>
    </ScrollArea>
  )
}
