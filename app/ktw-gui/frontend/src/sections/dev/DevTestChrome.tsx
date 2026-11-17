// SPDX-License-Identifier: Apache-2.0
import type { ReactNode } from 'react'
import {
  ActionIcon,
  Badge,
  Box,
  Code,
  Group,
  Stack,
  Tabs,
  Text,
  Tooltip,
} from '@mantine/core'
import { StateBadge } from '@ktw/display-lib/ui-core/components/other/index.ts'
import { IconRefresh } from '@tabler/icons-react'
import type { IframeWidgetStatus } from '../../components/iframe-widget'
import { useAppStore } from '../../store'

interface DevTestChromeProps {
  title: string
  status: IframeWidgetStatus
  statusDetail?: string
  controls?: ReactNode
  readyCount?: number
  eventCount?: number
  eventLabel?: string
  onReload: () => void
  content: ReactNode
  schema: ReactNode
  events: ReactNode
}

export function DevTestChrome({
  title,
  status,
  statusDetail,
  controls,
  readyCount,
  eventCount,
  eventLabel = 'events',
  onReload,
  content,
  schema,
  events,
}: DevTestChromeProps) {
  const displayedUniverse = useAppStore((s) => s.displayedUniverseName)

  return (
    <Stack gap={0} style={{ height: '100%', overflow: 'hidden' }}>
      {/* Mini ribbon */}
      <Group
        justify="space-between"
        wrap="nowrap"
        gap="sm"
        px="md"
        py={6}
        style={{ borderBottom: '1px solid var(--mantine-color-default-border)', flexShrink: 0 }}
      >
        <Group gap="xs" wrap="nowrap" style={{ minWidth: 0 }}>
          <Text fw={600} size="sm" truncate>{title}</Text>
          <StateBadge value={status} tones={WIDGET_TONES} label={status in WIDGET_TONES ? status : 'loading'} />
          {statusDetail && (
            <Text size="xs" c="dimmed" truncate>
              {statusDetail}
            </Text>
          )}
        </Group>
        <Group gap="xs" wrap="nowrap">
          {controls}
          <Tooltip label="Reload iframe">
            <ActionIcon variant="default" size="sm" onClick={onReload}>
              <IconRefresh size={14} />
            </ActionIcon>
          </Tooltip>
          <Text size="xs" c="dimmed" style={{ whiteSpace: 'nowrap' }}>
            <Code style={{ fontSize: 10 }}>{displayedUniverse ?? '(no universe)'}</Code>
            {readyCount !== undefined && <> · ready: <Code style={{ fontSize: 10 }}>{readyCount}</Code></>}
            {eventCount !== undefined && <> · {eventLabel}: <Code style={{ fontSize: 10 }}>{eventCount}</Code></>}
          </Text>
        </Group>
      </Group>

      {/* Tabs */}
      <Tabs defaultValue="content" style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }} keepMounted={false}>
        <Tabs.List style={{ flexShrink: 0 }}>
          <Tabs.Tab value="content">Content</Tabs.Tab>
          <Tabs.Tab value="schema">Schema</Tabs.Tab>
          <Tabs.Tab
            value="events"
            rightSection={
              eventCount && eventCount > 0 ? (
                <Badge size="xs" color="teal" variant="light" circle>{eventCount}</Badge>
              ) : null
            }
          >
            Events
          </Tabs.Tab>
        </Tabs.List>

        {/* Content + Schema panels are FULL-BLEED — no padding, no border —
            so the iframe fills the entire tab. Pages that need padding
            wrap their own content. */}
        <Tabs.Panel value="content" style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
          <Box style={{ flex: 1, minHeight: 0, display: 'flex' }}>
            {content}
          </Box>
        </Tabs.Panel>

        <Tabs.Panel value="schema" style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
          <Box style={{ flex: 1, minHeight: 0, display: 'flex' }}>
            {schema}
          </Box>
        </Tabs.Panel>

        <Tabs.Panel value="events" style={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
          <Box style={{ height: '100%', overflow: 'auto', padding: 8 }}>
            {events}
          </Box>
        </Tabs.Panel>
      </Tabs>
    </Stack>
  )
}

/** An iframe widget's health, as a state badge's tone. */
const WIDGET_TONES: Record<string, string> = { ok: 'green', fallback: 'yellow', down: 'red' }
