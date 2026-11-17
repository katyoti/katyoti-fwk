// SPDX-License-Identifier: Apache-2.0
// NativeViewDemoPage — ov2 acceptance demo for the NATIVE (no-iframe) view host.
//
// The same SyncRun object as the "ui-view + tree" iframe demo, but rendered via
// <NativeObjectView> directly in the ktw-gui React tree (no iframe). Proves the
// native render path: {schema, value} → buildUIViews → UIViewView, with the
// array-of-object fields rendering as tree-table + flat table.

import { Badge, Group, ScrollArea, Stack, Text } from '@mantine/core'
import { NativeObjectView } from '../../components/compositions'
import { VIEW_TREE_DEMO_SCHEMA, VIEW_TREE_DEMO_VALUE } from './demo-schemas'

export function NativeViewDemoPage() {
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Stack gap="md" p="md">
        <Group gap="xs">
          <Text fw={600}>Native object view</Text>
          <Badge color="green" variant="light">
            no iframe
          </Badge>
        </Group>
        <Text size="sm" c="dimmed">
          The SyncRun object rendered via @ktw/display-lib directly in ktw-gui's React tree — same
          schema as the iframe "ui-view + tree" demo, with the tree-table and flat-table fields,
          but no iframe. This host powers live-state and custom node/item pages.
        </Text>
        <NativeObjectView schema={VIEW_TREE_DEMO_SCHEMA} value={VIEW_TREE_DEMO_VALUE} />
      </Stack>
    </ScrollArea>
  )
}
