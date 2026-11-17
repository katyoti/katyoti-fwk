// SPDX-License-Identifier: Apache-2.0
/**
 * HistoryView — the ended runs (the live tab shows none): filtered by status,
 * group, who launched them and text; each with why it ended, how long it took,
 * its session, and a repeat. Below, the session files on disk.
 */

import { useMemo, useState } from 'react'
import { Accordion, ActionIcon, Badge, Group, ScrollArea, Select, Table, Text, TextInput, Tooltip } from '@mantine/core'
import { IconExternalLink, IconRepeat, IconSearch } from '@tabler/icons-react'
import { useT } from '../../i18n'
import { LiveSessionList } from '../shell/LiveSessionList'
import { STATUS_TONE, fmtDuration, history, originKind, type HistoryFilter, type RunInfo } from './model'

export interface HistoryProps {
  universeName: string
  runs: RunInfo[]
  onOpen: (r: RunInfo) => void
  onOpenSession: (r: RunInfo) => void
  onRepeat: (r: RunInfo) => Promise<void>
}

export function HistoryView({ universeName, runs, onOpen, onOpenSession, onRepeat }: HistoryProps) {
  const t = useT('runs')
  const [f, setF] = useState<HistoryFilter>({})
  const rows = useMemo(() => history(runs, f), [runs, f])
  const groups = useMemo(() => [...new Set(runs.map((r) => r.group).filter(Boolean) as string[])].sort(), [runs])
  return (
    <>
      <Group gap="xs" mb="sm" wrap="wrap">
        <Select size="xs" w={170} clearable placeholder={t('console.history.status')} value={f.status ?? null} onChange={(v) => setF({ ...f, status: v ?? undefined })}
          data={['completed', 'failed', 'cancelled', 'killed'].map((s) => ({ value: s, label: t(`status.${s}`) }))} />
        <Select size="xs" w={150} clearable placeholder={t('console.history.group')} value={f.group ?? null} onChange={(v) => setF({ ...f, group: v ?? undefined })} data={groups} />
        <Select size="xs" w={170} clearable placeholder={t('console.history.origin')} value={f.origin ?? null} onChange={(v) => setF({ ...f, origin: v ?? undefined })}
          data={['person', 'planner', 'agent', 'system'].map((o) => ({ value: o, label: t(`console.origin.${o}`) }))} />
        <TextInput size="xs" style={{ flex: '1 1 220px' }} leftSection={<IconSearch size={14} />} placeholder={t('console.history.search')}
          value={f.text ?? ''} onChange={(e) => setF({ ...f, text: e.currentTarget.value })} />
      </Group>
      <ScrollArea type="auto">
        <Table striped highlightOnHover verticalSpacing={6} miw={900}>
          <Table.Thead><Table.Tr>
            <Table.Th>{t('table.state')}</Table.Th><Table.Th>{t('table.task')}</Table.Th><Table.Th>{t('table.startedBy')}</Table.Th>
            <Table.Th>{t('table.for')}</Table.Th><Table.Th>{t('table.group')}</Table.Th><Table.Th>{t('console.history.ended')}</Table.Th>
            <Table.Th>{t('table.duration')}</Table.Th><Table.Th>{t('console.history.why')}</Table.Th><Table.Th />
          </Table.Tr></Table.Thead>
          <Table.Tbody>
            {rows.length === 0 && <Table.Tr><Table.Td colSpan={9}><Text size="sm" c="dimmed" ta="center" py="md">{t('console.history.none')}</Text></Table.Td></Table.Tr>}
            {rows.map((r) => (
              <Table.Tr key={r.runId} style={{ cursor: 'pointer' }} onClick={() => onOpen(r)}>
                <Table.Td><Badge size="sm" variant="light" color={STATUS_TONE[r.status] ?? 'gray'}>{t(`status.${r.status}`, { defaultValue: r.status })}</Badge></Table.Td>
                <Table.Td><Text size="sm" fw={600}>{r.taskName}</Text></Table.Td>
                <Table.Td><Text size="xs">{t(`console.origin.${originKind(r)}`, { defaultValue: originKind(r) })}</Text></Table.Td>
                <Table.Td><Text size="xs" ff="monospace" c="dimmed">{r.target ?? ''}</Text></Table.Td>
                <Table.Td><Text size="xs">{r.group ?? ''}</Text></Table.Td>
                <Table.Td><Text size="xs" className="sc-mono">{r.endedAt ? new Date(r.endedAt).toLocaleString() : ''}</Text></Table.Td>
                <Table.Td><Text size="xs" className="sc-mono">{r.endedAt ? fmtDuration(Date.parse(r.endedAt) - Date.parse(r.startedAt)) : ''}</Text></Table.Td>
                <Table.Td maw={380}><Text size="xs" c={r.error ? 'red' : 'dimmed'} lineClamp={2}>{r.error ?? ''}</Text></Table.Td>
                <Table.Td onClick={(e) => e.stopPropagation()}>
                  <Group gap={4} wrap="nowrap">
                    {r.sessionId && <Tooltip label={t('row.openSession')} withArrow><ActionIcon variant="subtle" size="sm" aria-label={t('row.openSession')} onClick={() => onOpenSession(r)}><IconExternalLink size={14} /></ActionIcon></Tooltip>}
                    <Tooltip label={t('console.drawer.repeat')} withArrow><ActionIcon variant="subtle" size="sm" aria-label={t('console.drawer.repeat')} onClick={() => onRepeat(r)}><IconRepeat size={14} /></ActionIcon></Tooltip>
                  </Group>
                </Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      </ScrollArea>
      <Text size="xs" c="dimmed" mt={6}>{t('console.history.kept')}</Text>
      <Accordion mt="md" variant="separated">
        <Accordion.Item value="files">
          <Accordion.Control>{t('history.title')}</Accordion.Control>
          <Accordion.Panel><LiveSessionList universeName={universeName} /></Accordion.Panel>
        </Accordion.Item>
      </Accordion>
    </>
  )
}
