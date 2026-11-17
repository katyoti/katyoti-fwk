// SPDX-License-Identifier: Apache-2.0
/**
 * ProjectHome — landing view shown when a project is selected but no node
 * is picked. Minimal v1: project header + "pick a node" CTA.
 *
 * Empty state when no project is selected at all (rare — only on first run
 * before MOCK_PROJECTS sets a default activeProjectId).
 */
import { Avatar, Center, Group, Stack, Text, Title } from '@mantine/core'
import { EmptyState, ItemCard } from '@ktw/display-lib/ui-core/components/other/index.ts'
import { IconArrowLeft } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'

// A sentence carrying an inline element is one key: the element's slot is
// interpolated with a marker, and the translated text is cut around it.
const SLOT = '\u0001'

export function ProjectHome() {
  const t = useT('shell')
  const activeProjectId = useAppStore((s) => s.activeProjectId)
  const projects        = useAppStore((s) => s.projects)
  const project         = projects.find((p) => p.id === activeProjectId)

  if (!project) {
    return (
      <Center style={{ height: '100%' }}>
        <Stack align="center" gap="md">
          <IconArrowLeft size={48} stroke={1.2} color="var(--mantine-color-dimmed)" />
          <Text c="dimmed" size="sm">{t('projectHome.pickProject')}</Text>
        </Stack>
      </Center>
    )
  }

  const [nodeBefore, nodeAfter = ''] = t('projectHome.pickNode', { tree: SLOT }).split(SLOT)

  return (
    <Stack p="xl" gap="lg" style={{ height: '100%', overflow: 'auto' }}>
      <Group gap="md" align="center" wrap="nowrap">
        <Avatar size={56} radius="md" color={project.color} variant="light" style={{ fontWeight: 700, fontSize: 16 }}>
          {project.icon}
        </Avatar>
        <Stack gap={2} style={{ flex: 1, minWidth: 0 }}>
          <Title order={3}>{project.name}</Title>
          <Text size="xs" c="dimmed" truncate>{project.path}</Text>
        </Stack>
      </Group>

      {project.contextTree.description && (
        <Text size="sm" c="dimmed">{project.contextTree.description}</Text>
      )}

      <ItemCard padding="lg">
        <EmptyState icon="hand-finger" color="blue"
          message={<>{nodeBefore}<Text component="span" fw={600}>{t('projectHome.contextTree')}</Text>{nodeAfter}</>} />
      </ItemCard>
    </Stack>
  )
}
