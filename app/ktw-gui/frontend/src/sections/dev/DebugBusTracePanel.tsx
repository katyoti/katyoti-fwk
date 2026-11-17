// SPDX-License-Identifier: Apache-2.0
/**
 * DebugBusTracePanel — live view of NATS / RPC slot calls inside
 * the existing Debug tab cluster (cwm-v3 UX restructure 2026-06-06).
 * Operator-introspection surface for verifying cross-binary traffic.
 *
 * v1 scaffold; real subscription wires when the bus-trace Wails
 * binding ships.
 */

import { Box, Code, Group, ScrollArea, Stack, Text, Title } from '@mantine/core'
import { IconActivity } from '@tabler/icons-react'

export function DebugBusTracePanel() {
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Stack gap="md" p="lg">
        <Group gap="xs">
          <IconActivity size={20} color="var(--mantine-color-orange-6)" />
          <Title order={4}>Bus / RPC trace</Title>
        </Group>
        <Text size="sm" c="dimmed">
          Live tail of cross-binary slot calls + NATS subjects. Useful
          for verifying that <Code>cwm-items</Code> connection traffic
          flows, hub catalog refresh fires, sidecar identity probes
          land.
        </Text>
        <Box p="md" style={{ background: 'var(--mantine-color-dark-7)', borderRadius: 4 }}>
          <Text size="xs" c="dimmed" ff="monospace">
            # Live trace not wired yet.
            #
            # When the bus-trace Wails binding ships, this view will
            # subscribe to ktw.&gt;.event.&gt; (and friends) and render a
            # rolling window of decoded events with timestamp +
            # subject + payload size.
          </Text>
        </Box>
      </Stack>
    </ScrollArea>
  )
}
