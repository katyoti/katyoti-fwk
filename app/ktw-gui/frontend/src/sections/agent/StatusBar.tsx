// SPDX-License-Identifier: Apache-2.0
import { Badge, Group, Text, Tooltip } from '@mantine/core'
import { IconBolt, IconCpu, IconDatabase } from '@tabler/icons-react'
import { useSessionStats, useSessionStatus, useSessionName } from '../../store'
import { useT } from '../../i18n'

const STATUS_COLOR: Record<string, string> = {
  idle: 'gray', running: 'green', done: 'blue', error: 'red', stopped: 'orange',
}

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + 'M'
  if (n >= 1_000) return (n / 1_000).toFixed(1) + 'k'
  return String(n)
}

/** 24px bottom strip — status · task · cache · tokens · cost · model.
 *  Per-run display: reads THIS session's status/stats/name. */
export function StatusBar({ sessionId }: { sessionId?: string } = {}) {
  const sessionStatus = useSessionStatus(sessionId)
  const activeTask    = useSessionName(sessionId)
  const stats         = useSessionStats(sessionId)
  const t = useT('chat')

  // A delegating session can use several models (parent + child on a cheaper
  // model). Show "…" + a tooltip listing them when >1; the single name otherwise.
  const models = stats?.models ?? []
  const multiModel = models.length > 1
  const model = multiModel ? '…' : (stats?.model || models[0] || '—')
  const provider = multiModel ? undefined : stats?.provider
  const inTok = stats?.inputTokens ?? 0
  const outTok = stats?.outputTokens ?? 0
  const cacheRead = stats?.cacheReadTokens ?? 0
  const turns = stats?.turns ?? 0
  const ctxCur = stats?.currentInputTokens ?? 0
  const ctxWin = stats?.contextWindow ?? 0
  const ctxPct = ctxWin > 0 ? Math.round((ctxCur / ctxWin) * 100) : null
  const lastMs = stats?.lastDurationMs ?? 0
  const ttftMs = stats?.lastTtftMs ?? 0

  // Cache efficiency = cached input / total input seen (cached + fresh).
  const totalInputSeen = inTok + cacheRead
  const cachePct = totalInputSeen > 0 ? Math.round((cacheRead / totalInputSeen) * 100) : null

  // Cost is computed on the AGENT side from the model's configured
  // $/MTok rates (LLMProviders.yaml) and delivered in the stats event.
  // The GUI never hardcodes prices — it shows the value only when the
  // backend marks it known (the active model declared a rate).
  const costUSD = stats?.costKnown ? stats.costUSD : null

  const hasStats = !!stats && (inTok > 0 || outTok > 0)

  return (
    <Group
      gap="md"
      px="sm"
      style={{
        height: 24,
        flexShrink: 0,
        borderTop: '1px solid var(--mantine-color-default-border)',
        background: 'var(--mantine-color-body)',
      }}
    >
      <Badge size="xs" color={STATUS_COLOR[sessionStatus] ?? 'gray'} variant="dot" style={{ textTransform: 'none' }}>
        {t(`status.${sessionStatus}`, { defaultValue: sessionStatus })}
      </Badge>

      {activeTask && (
        <>
          <Text size="xs" c="dimmed">│</Text>
          <Group gap={4} align="center">
            <IconBolt size={11} color="var(--mantine-color-dimmed)" />
            <Text size="xs" c="dimmed">{activeTask}</Text>
          </Group>
        </>
      )}

      <div style={{ flex: 1 }} />

      {hasStats && (
        <>
          <Text size="xs" c="dimmed">│</Text>
          {turns > 0 && (
            <Text size="xs" c="dimmed">{t('stats.turn', { turns })}</Text>
          )}
          {lastMs > 0 && (
            <Tooltip label={t('stats.lastResponse', { last: (lastMs / 1000).toFixed(1), ttft: (ttftMs / 1000).toFixed(1) })} withArrow>
              <Text size="xs" c="dimmed">{(lastMs / 1000).toFixed(1)}s{ttftMs > 0 ? ` ⚡${(ttftMs / 1000).toFixed(1)}s` : ''}</Text>
            </Tooltip>
          )}
          {ctxPct !== null && (
            <Tooltip label={t('stats.contextTooltip', { cur: ctxCur.toLocaleString(), win: ctxWin.toLocaleString(), pct: ctxPct })} withArrow>
              <Text size="xs" c={ctxPct >= 80 ? 'orange' : 'dimmed'}>
                {t('stats.context', { cur: fmtTokens(ctxCur), win: fmtTokens(ctxWin), pct: ctxPct })}
              </Text>
            </Tooltip>
          )}
          {cachePct !== null && (
            <Tooltip label={t('stats.cacheTooltip', { cached: fmtTokens(cacheRead), total: fmtTokens(totalInputSeen) })} withArrow>
              <Group gap={3} align="center">
                <IconDatabase size={11} color="var(--mantine-color-dimmed)" />
                <Text size="xs" c="dimmed">{cachePct}%</Text>
              </Group>
            </Tooltip>
          )}
          <Tooltip label={t('stats.tokensTooltip', { input: inTok.toLocaleString(), cached: cacheRead.toLocaleString(), output: outTok.toLocaleString() })} withArrow>
            <Text size="xs" c="dimmed">{t('stats.tokens', { input: fmtTokens(inTok), output: fmtTokens(outTok) })}</Text>
          </Tooltip>
          {costUSD !== null && (
            <Tooltip label={t('stats.costTooltip')} withArrow>
              <Text size="xs" c="dimmed">${costUSD.toFixed(costUSD < 0.01 ? 4 : 2)}</Text>
            </Tooltip>
          )}
          <Text size="xs" c="dimmed">│</Text>
        </>
      )}

      <Tooltip
        label={multiModel ? models.join(', ') : (provider ? `${provider} · ${model}` : model)}
        withArrow
        openDelay={300}
        disabled={!multiModel && !provider}
      >
        <Group gap={4} align="center" style={{ cursor: multiModel ? 'help' : 'default' }}>
          <IconCpu size={11} color="var(--mantine-color-dimmed)" />
          <Text size="xs" c="dimmed">{provider ? `${provider} · ${model}` : model}</Text>
        </Group>
      </Tooltip>
    </Group>
  )
}
