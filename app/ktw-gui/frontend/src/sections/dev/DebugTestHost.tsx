// SPDX-License-Identifier: Apache-2.0
/**
 * DebugTestHost — dispatches the selected `test` id to its demo page
 * component. Lifted out of BaseLayout so DebugRoot can mount it from
 * the new tabbed shell without pulling layout-level imports.
 */
import { Box, Stack, Text } from '@mantine/core'
import { LoadWidgetDemoPage } from './LoadWidgetDemoPage'
import { NativeViewDemoPage } from './NativeViewDemoPage'
import { ConfluenceStatusDemoPage } from './ConfluenceStatusDemoPage'
import { DocumentsDemoPage } from './DocumentsDemoPage'

export function DebugTestHost({ test }: { test: string | null }) {
  if (test === null) {
    return (
      <Box style={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Stack align="center" gap={4}>
          <Text size="sm" c="dimmed" fw={500}>No test selected</Text>
          <Text size="xs" c="dimmed">Pick a test from the tree on the left.</Text>
        </Stack>
      </Box>
    )
  }
  switch (test) {
    case 'load-widget':      return <LoadWidgetDemoPage />
    case 'native-view':      return <NativeViewDemoPage />
    case 'confluence-status': return <ConfluenceStatusDemoPage />
    case 'documents':        return <DocumentsDemoPage />
    default:
      return (
        <Box style={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <Text size="sm" c="dimmed">Unknown test: {test}</Text>
        </Box>
      )
  }
}
