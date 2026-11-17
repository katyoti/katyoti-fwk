// SPDX-License-Identifier: Apache-2.0
// ConfluenceStatusDemoPage — the `confluence status` table-tree, GUI-native.
//
// Renders the confluence sync-status object via <NativeObjectView>: a tree of
// pages with Local (✓/✎ + version) · direction arrows · Remote (✓ + version) ·
// Page (tree) · Status columns — matching the confluence status TUI.

import { ScrollArea, Stack, Text } from '@mantine/core'
import { NativeObjectView } from '../../components/compositions'
import { CONFLUENCE_STATUS_SCHEMA, CONFLUENCE_STATUS_VALUE } from './demo-schemas'

export function ConfluenceStatusDemoPage() {
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Stack gap="md" p="md">
        <Text fw={600}>Confluence status (table-tree)</Text>
        <Text size="sm" c="dimmed">
          The `confluence status` output rendered natively — Local / direction / Remote / Page tree /
          Status, with version checks (✓/✎) and sync-direction arrows.
        </Text>
        <NativeObjectView schema={CONFLUENCE_STATUS_SCHEMA} value={CONFLUENCE_STATUS_VALUE} />
      </Stack>
    </ScrollArea>
  )
}
