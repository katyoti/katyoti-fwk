// SPDX-License-Identifier: Apache-2.0
/**
 * Why an export failed, said where it was asked: the « export failed » badge
 * and the reason itself beside it - the converter's message (a diagram that
 * cannot be drawn, Pandoc missing, a file that cannot be read), never only a
 * tooltip.
 */
import { Badge, Group, Text } from '@mantine/core'
import { IconAlertTriangle } from '@tabler/icons-react'

export function ExportFailure({ label, reason }: { label: string; reason: string }) {
  return (
    <Group gap={6} wrap="nowrap" style={{ minWidth: 0 }} data-testid="document-export-failure">
      <Badge size="xs" color="red" variant="light" leftSection={<IconAlertTriangle size={10} />} style={{ flexShrink: 0 }}>{label}</Badge>
      <Text size="xs" c="red" lineClamp={2} title={reason} data-testid="document-export-failure-reason">{reason}</Text>
    </Group>
  )
}
