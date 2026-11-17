// SPDX-License-Identifier: Apache-2.0
/**
 * SessionCostPanel — the Cost tab for a session.
 *
 * A session can interleave runs on DIFFERENT models (a parent delegating to a
 * child on a cheaper model), so cost is reported PER MODEL. Every figure comes
 * from the per-call cost the agent loop computes at source (Usage.Cost), using
 * each model's own pricing — the GUI never replicates provider cache formulas.
 *
 * Reuses GetSessionJournal (which aggregates conv.models + conv.totalUsd) so
 * there's no second parse of llm.jsonl.
 */

import { useEffect, useState } from 'react'
import {
  Alert,
  Badge,
  Box,
  Center,
  Group,
  Loader,
  ScrollArea,
  Table,
  Text,
} from '@mantine/core'
import { IconAlertCircle } from '@tabler/icons-react'

import { GetSessionJournal } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'
import type { main } from '../../../wailsjs/go/models'

interface Props {
  universeName: string
  sessionId:    string
}

export function SessionCostPanel({ universeName, sessionId }: Props) {
  const t = useT('shell')
  const [conv, setConv]       = useState<main.SessionJournalConversation | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError]     = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    setConv(null)
    GetSessionJournal(universeName, sessionId)
      .then((r) => {
        if (cancelled) return
        if (r.error) setError(r.error)
        else setConv(r.conversation ?? null)
        setLoading(false)
      })
      .catch((e) => {
        if (cancelled) return
        setError(String(e))
        setLoading(false)
      })
    return () => { cancelled = true }
  }, [universeName, sessionId])

  if (loading) return <Center py="xl"><Loader size="sm" /></Center>
  if (error) {
    return (
      <Box p="md">
        <Alert icon={<IconAlertCircle size={16} />} color="red" variant="light" title={t('sessionCost.unavailable')}>
          {error}
        </Alert>
      </Box>
    )
  }
  const models = conv?.models ?? []
  if (!conv || models.length === 0) {
    return <Center py="xl"><Text size="sm" c="dimmed">{t('sessionCost.empty')}</Text></Center>
  }

  return (
    <ScrollArea style={{ height: '100%' }} scrollbarSize={6}>
      <Box p="md">
        <Group justify="space-between" mb="sm" align="flex-end">
          <Text fw={600} size="sm">{t('sessionCost.byModel')}</Text>
          <Group gap={4} align="baseline">
            <Text size="xs" c="dimmed">{t('sessionCost.sessionTotal')}</Text>
            <Text fw={700} ff="monospace">{usd(conv.totalUsd)}</Text>
          </Group>
        </Group>

        <Table striped highlightOnHover withTableBorder fz="xs" ff="monospace">
          <Table.Thead>
            <Table.Tr>
              <Table.Th>{t('sessionCost.model')}</Table.Th>
              <Table.Th>{t('sessionCost.provider')}</Table.Th>
              <Table.Th ta="right">{t('sessionCost.runs')}</Table.Th>
              <Table.Th ta="right">{t('sessionCost.input')}</Table.Th>
              <Table.Th ta="right">{t('sessionCost.output')}</Table.Th>
              <Table.Th ta="right">{t('sessionCost.cacheRead')}</Table.Th>
              <Table.Th ta="right">{t('sessionCost.cacheWrite')}</Table.Th>
              <Table.Th ta="right">{t('sessionCost.cost')}</Table.Th>
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {models.map((m) => (
              <Table.Tr key={m.model}>
                <Table.Td>
                  <Text size="xs" fw={600} ff="monospace">{m.model}</Text>
                </Table.Td>
                <Table.Td>
                  {m.provider && <Badge size="xs" variant="light" color="gray">{m.provider}</Badge>}
                </Table.Td>
                <Table.Td ta="right">{m.runs}</Table.Td>
                <Table.Td ta="right">{num(m.inputTokens)}</Table.Td>
                <Table.Td ta="right">{num(m.outputTokens)}</Table.Td>
                <Table.Td ta="right" c={m.cacheReadTokens ? undefined : 'dimmed'}>{num(m.cacheReadTokens)}</Table.Td>
                <Table.Td ta="right" c={m.cacheWriteTokens ? undefined : 'dimmed'}>{num(m.cacheWriteTokens)}</Table.Td>
                <Table.Td ta="right" fw={600}>{usd(m.totalUsd)}</Table.Td>
              </Table.Tr>
            ))}
          </Table.Tbody>
          {models.length > 1 && (
            <Table.Tfoot>
              <Table.Tr>
                <Table.Td colSpan={3}><Text size="xs" fw={700}>{t('sessionCost.total')}</Text></Table.Td>
                <Table.Td ta="right" fw={700}>{num(sum(models, (m) => m.inputTokens))}</Table.Td>
                <Table.Td ta="right" fw={700}>{num(sum(models, (m) => m.outputTokens))}</Table.Td>
                <Table.Td ta="right" fw={700}>{num(sum(models, (m) => m.cacheReadTokens))}</Table.Td>
                <Table.Td ta="right" fw={700}>{num(sum(models, (m) => m.cacheWriteTokens))}</Table.Td>
                <Table.Td ta="right" fw={700}>{usd(conv.totalUsd)}</Table.Td>
              </Table.Tr>
            </Table.Tfoot>
          )}
        </Table>

        <Text size="xs" c="dimmed" mt="sm">
          {t('sessionCost.note')}
        </Text>
      </Box>
    </ScrollArea>
  )
}

function usd(v?: number): string {
  const n = v ?? 0
  if (n === 0) return '$0.00'
  if (n < 0.01) return `$${n.toFixed(4)}`
  return `$${n.toFixed(2)}`
}

function num(v?: number): string {
  return (v ?? 0).toLocaleString()
}

function sum(rows: main.SessionModelCost[], pick: (m: main.SessionModelCost) => number | undefined): number {
  return rows.reduce((acc, m) => acc + (pick(m) ?? 0), 0)
}
