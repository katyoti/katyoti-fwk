// SPDX-License-Identifier: Apache-2.0
/**
 * DebugWorkspaceStatePanel — workspace.yaml + lockfile inspector
 * inside the existing Debug tab cluster (cwm-v3 UX restructure
 * 2026-06-06).
 */

import { Box, Code, Group, ScrollArea, Stack, Text, Title } from '@mantine/core'
import { IconBuildingWarehouse } from '@tabler/icons-react'

export function DebugWorkspaceStatePanel() {
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Stack gap="md" p="lg">
        <Group gap="xs">
          <IconBuildingWarehouse size={20} color="var(--mantine-color-grape-6)" />
          <Title order={4}>Workspace state</Title>
        </Group>
        <Text size="sm" c="dimmed">
          Lockfile holder + workspace.yaml contents + per-node mutex
          state. Useful when debugging concurrent writes or a
          workspace that refuses to open.
        </Text>
        <Box p="md" style={{ background: 'var(--mantine-color-dark-7)', borderRadius: 4 }}>
          <Text size="xs" c="dimmed" ff="monospace">
            # Workspace state not wired yet.
            #
            # Future surface:
            #   - .katyoti/.lock content + holder PID
            #   - workspace.yaml (live + on-disk diff)
            #   - per-node mutex state when a write is in flight
            #   - Recent NodeCreate / ItemCreate journal entries
            #
            # Existing surface: `ktw-agents sc dirs --out json` from the
            # CLI shows the resolved workspace paths.
          </Text>
        </Box>
      </Stack>
    </ScrollArea>
  )
}
