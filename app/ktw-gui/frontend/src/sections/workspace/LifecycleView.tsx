// SPDX-License-Identifier: Apache-2.0
/**
 * LifecycleView — where an object stands and where it may go, for each
 * lifecycle it has (its own, and a capability's that follows a workflow):
 * the state on its workflow's path, the transitions from it — each with its
 * guard's verdict and, when it requires a named gate, the gate's checklist —
 * and the history (the journal's moves; the object stores only its state). The person using the GUI confirms a requirement and makes
 * a move from here; CWM checks both, so a refusal comes back with its reason.
 */
import { useCallback, useEffect, useState } from 'react'
import {
  Alert, Badge, Box, Button, Card, Center, Code, Group, Loader, Progress, Stack, Text, Textarea, ThemeIcon,
  Timeline, Tooltip,
} from '@mantine/core'
import { SectionCard } from '@ktw/display-lib/ui-core/components/other/index.ts'
import {
  IconAlertTriangle, IconArrowRight, IconCheck, IconCircleDashed, IconHandStop, IconHelpCircle, IconLock, IconRobot, IconUser, IconX,
} from '@tabler/icons-react'
import {
  WorkspaceObjectGateConfirm, WorkspaceObjectTransition, WorkspaceObjectTransitionOptions,
} from '../../../wailsjs/go/main/App'
import { api, main } from '../../../wailsjs/go/models'
import { usePersonId } from '../../store/profileStore'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'
import { errorText, type ErrorMsg } from '../../i18n/errors'

// A state's colour is the one its capability's badge gives it (the pane sends
// them), so a state reads the same here, in the header and in the lists.
const stateColorOf = (colors: Record<string, string> | undefined, s: string) =>
  colors?.[s] ?? colors?.['*'] ?? 'indigo'

export function LifecycleView({ universeName, objectId }: { universeName: string; objectId: string }) {
  const personId = usePersonId()
  const treeRev = useAppStore((s) => s.workspaceTreeRevision)
  const bumpTree = useAppStore((s) => s.bumpWorkspaceTree)
  const [data, setData] = useState<main.WorkspaceObjectLifecycles | null>(null)
  const [busy, setBusy] = useState('')
  const t = useT('workspace')

  const load = useCallback(async () => {
    setData(await WorkspaceObjectTransitionOptions(universeName, objectId, personId))
  }, [universeName, objectId, personId])
  useEffect(() => { void load() }, [load, treeRev])

  const after = (r: main.WorkspaceObjectLifecycles) => {
    setData(r)
    if (!r.message && !r.error) bumpTree()
  }
  const move = async (lc: api.LifecycleOptions, tr: api.TransitionOption, note: string, force: boolean) => {
    setBusy(`${lc.capability}:${tr.name}`)
    try {
      after(await WorkspaceObjectTransition(universeName, objectId, lc.capability ?? '', tr.to, tr.name, note, force, personId))
    } finally { setBusy('') }
  }
  const confirm = async (gate: string, requirement: string, note: string) => {
    setBusy(`${gate}/${requirement}`)
    try {
      after(await WorkspaceObjectGateConfirm(universeName, objectId, gate, requirement, note, personId))
    } finally { setBusy('') }
  }

  if (!data) return <Center p="xl"><Loader size="sm" /></Center>
  return (
    <Stack gap="md" p="md">
      {data.error && <Alert color="red" variant="light" icon={<IconAlertTriangle size={16} />}>{errorText(data)}</Alert>}
      {data.message && (
        <Alert color="orange" variant="light" icon={<IconHandStop size={16} />} title={t('lifecycle.refused')} withCloseButton
          onClose={() => setData({ ...data, message: '' } as main.WorkspaceObjectLifecycles)}>
          <Text size="sm" style={{ whiteSpace: 'pre-wrap' }}>{errorText({ error: data.message, errorMsg: (data as { messageMsg?: ErrorMsg }).messageMsg })}</Text>
        </Alert>
      )}
      {!data.error && (data.lifecycles ?? []).length === 0 && (
        <Text size="sm" c="dimmed">{t('lifecycle.noWorkflow')}</Text>
      )}
      {(data.lifecycles ?? []).map((lc) => (
        <LifecycleCard key={lc.capability || '-'} lc={lc} busy={busy} onMove={move} onConfirm={confirm} />
      ))}
    </Stack>
  )
}

function LifecycleCard({ lc, busy, onMove, onConfirm }: {
  lc: api.LifecycleOptions
  busy: string
  onMove: (lc: api.LifecycleOptions, tr: api.TransitionOption, note: string, force: boolean) => Promise<void>
  onConfirm: (gate: string, requirement: string, note: string) => Promise<void>
}) {
  const visited = new Set((lc.history ?? []).flatMap((h) => [h.from, h.to]).filter(Boolean))
  const stateColor = (s: string) => stateColorOf(lc.state_colors, s)
  const t = useT('workspace')
  return (
    <SectionCard icon="progress" color={stateColor(lc.state)}
      title={lc.capability ? `${lc.capability} — ${lc.workflow_label || lc.workflow}` : (lc.workflow_label || lc.workflow)}
      description={lc.capability ? t('lifecycle.capabilityWorkflow', { capability: lc.capability }) : t('lifecycle.objectLifecycle')}
      actions={<Badge size="lg" variant="filled" color={stateColor(lc.state)}>{lc.state}</Badge>}>

      {/* The workflow's path: where it has been, where it is, where it can go. */}
      <Group gap={4} mb="md" wrap="wrap">
        {(lc.states ?? []).map((st, i) => {
          const here = st.name === lc.state
          const reachable = (lc.transitions ?? []).some((x) => x.to === st.name)
          return (
            <Group key={st.name} gap={4} wrap="nowrap">
              {i > 0 && <IconArrowRight size={12} color="var(--mantine-color-dimmed)" />}
              <Tooltip label={st.description || st.name} disabled={!st.description} withArrow>
                <Badge size="sm" radius="sm"
                  variant={here ? 'filled' : reachable ? 'outline' : 'light'}
                  color={here ? stateColor(st.name) : visited.has(st.name) ? 'gray' : reachable ? stateColor(st.name) : 'gray'}
                  style={{ opacity: here || reachable || visited.has(st.name) ? 1 : 0.55 }}>
                  {st.name}{st.terminal ? ' ■' : ''}
                </Badge>
              </Tooltip>
            </Group>
          )
        })}
      </Group>

      {lc.terminal && <Text size="sm" c="dimmed" mb="sm">{t('lifecycle.terminal')}</Text>}
      {!lc.terminal && (lc.transitions ?? []).length === 0 && (
        <Text size="sm" c="dimmed" mb="sm">{t('lifecycle.noTransition', { state: lc.state })}</Text>
      )}
      <Stack gap="sm">
        {(lc.transitions ?? []).map((tr) => (
          <TransitionCard key={tr.name} lc={lc} tr={tr} busy={busy} onMove={onMove} onConfirm={onConfirm} />
        ))}
      </Stack>

      {(lc.history ?? []).length > 0 && (
        <Box mt="md">
          <Text size="xs" fw={700} tt="uppercase" c="dimmed" mb={6}>{t('lifecycle.history')}</Text>
          <Timeline active={(lc.history ?? []).length} bulletSize={20} lineWidth={2}>
            {[...(lc.history ?? [])].reverse().map((h, i) => (
              <Timeline.Item key={i} color={stateColor(h.to)}
                bullet={h.by?.kind === 'agent' ? <IconRobot size={12} /> : <IconUser size={12} />}
                title={<Group gap={6}><Text size="sm" fw={600}>{h.from ? `${h.from} → ${h.to}` : h.to}</Text>
                  {h.transition && <Code>{h.transition}</Code>}
                  {h.forced && <Badge size="xs" color="red" variant="light">{t('lifecycle.forced')}</Badge>}</Group>}>
                <Text size="xs" c="dimmed">{h.by?.id} · {h.at ? new Date(h.at).toLocaleString() : ''}</Text>
                {h.note && <Text size="sm" mt={2}>{h.note}</Text>}
                {passedUnmet(h.calls).length > 0 && (
                  <Text size="xs" c="red" mt={2}>{t('lifecycle.passedUnmet', { names: passedUnmet(h.calls).join(', ') })}</Text>
                )}
              </Timeline.Item>
            ))}
          </Timeline>
        </Box>
      )}
    </SectionCard>
  )
}

function TransitionCard({ lc, tr, busy, onMove, onConfirm }: {
  lc: api.LifecycleOptions
  tr: api.TransitionOption
  busy: string
  onMove: (lc: api.LifecycleOptions, tr: api.TransitionOption, note: string, force: boolean) => Promise<void>
  onConfirm: (gate: string, requirement: string, note: string) => Promise<void>
}) {
  const [note, setNote] = useState('')
  const [open, setOpen] = useState(false)
  const t = useT('workspace')
  const cl = gateChecklistOf(tr.calls)
  const items = cl?.items ?? []
  const met = items.filter((it) => it.status === 'met').length
  const blockedByGate = !!cl && !cl.passes
  const canMove = tr.allowed || tr.guard_status === 'at-move'
  const canForce = !canMove && !!tr.force
  const status = tr.allowed
    ? { color: 'green', label: t('lifecycle.ready') }
    : tr.guard_status === 'at-move' && !blockedByGate
      ? { color: 'blue', label: t('lifecycle.checkedAtMove') }
      : { color: 'red', label: t('lifecycle.blocked') }

  return (
    <Card withBorder radius="sm" padding="sm" bg="var(--mantine-color-default-hover)">
      <Group justify="space-between" wrap="nowrap">
        <Group gap="xs" wrap="nowrap">
          <IconArrowRight size={16} />
          <Text fw={600}>{tr.to}</Text>
          <Code>{tr.name}</Code>
          {cl && (
            <Tooltip label={t('lifecycle.requiresGate', { gate: cl.label || cl.gate })} withArrow>
              <Badge size="sm" variant="light" leftSection={<IconLock size={10} />}>{cl.label || cl.gate}</Badge>
            </Tooltip>
          )}
        </Group>
        <Badge color={status.color} variant="light">{status.label}</Badge>
      </Group>

      {tr.guard && (
        <Group gap={6} mt={6} wrap="nowrap">
          <Text size="xs" c="dimmed">{t('lifecycle.guard')}</Text>
          <Code style={{ fontSize: 11 }}>{tr.guard}</Code>
          {tr.guard_status === 'holds' && <IconCheck size={14} color="var(--mantine-color-green-6)" />}
          {tr.guard_status === 'fails' && <IconX size={14} color="var(--mantine-color-red-6)" />}
        </Group>
      )}
      {tr.reason && !tr.allowed && <Text size="xs" c="red" mt={4}>{tr.reason}</Text>}

      {cl && items.length > 0 && (
        <Box mt="sm">
          <Group justify="space-between" mb={4}>
            <Text size="xs" fw={700}>{cl.label || cl.gate}</Text>
            <Text size="xs" c="dimmed">{t('lifecycle.metCount', { met, total: items.length })}</Text>
          </Group>
          <Progress value={(met / items.length) * 100} color={cl.passes ? 'green' : 'orange'} size="sm" mb="xs" />
          <Stack gap={6}>
            {items.map((it) => (
              <RequirementRow key={it.name} gate={cl.gate} it={it} busy={busy} onConfirm={onConfirm} />
            ))}
          </Stack>
        </Box>
      )}

      <Group justify="flex-end" mt="sm" gap="xs">
        {!open && (canMove || canForce) && (
          <Button size="xs" variant={canMove ? 'filled' : 'light'} color={canMove ? undefined : 'red'}
            onClick={() => setOpen(true)}
            data-guide="lifecycle.move" data-guide-name={tr.to}
            data-guide-label={canMove ? t('lifecycle.moveToEllipsis', { to: tr.to }) : t('lifecycle.forceEllipsis')}>
            {canMove ? t('lifecycle.moveToEllipsis', { to: tr.to }) : t('lifecycle.forceEllipsis')}
          </Button>
        )}
      </Group>
      {open && (
        <Box mt="xs">
          <Textarea autosize minRows={2} value={note} onChange={(e) => setNote(e.currentTarget.value)}
            placeholder={canMove ? t('lifecycle.whyOptional') : t('lifecycle.whyForced')} />
          <Group justify="flex-end" mt={6} gap="xs">
            <Button size="xs" variant="subtle" onClick={() => setOpen(false)}>{t('common.cancel')}</Button>
            <Button size="xs" color={canMove ? undefined : 'red'} loading={busy === `${lc.capability}:${tr.name}`}
              disabled={!canMove && !note.trim()}
              onClick={() => void onMove(lc, tr, note, !canMove).then(() => { setOpen(false); setNote('') })}>
              {canMove ? t('lifecycle.moveTo', { to: tr.to }) : t('lifecycle.forceTo', { to: tr.to })}
            </Button>
          </Group>
        </Box>
      )}
    </Card>
  )
}

// A gate's checklist, as gate('…') answers it (the gates service's GateChecklist).
interface GateItem {
  name: string
  label?: string
  status: string
  reason?: string
  kind: string
  resolve?: Record<string, unknown>
  confirmed_by?: { by?: { id: string; kind: string }; at?: string; note?: string }
}
interface GateChecklist {
  gate: string
  label?: string
  items: GateItem[]
  passes: boolean
}

// passedUnmet names the requirements a forced move passed unmet, from the
// gate checklist its guard's gate('…') answered.
function passedUnmet(calls?: api.FunctionAnswer[]): string[] {
  return (gateChecklistOf(calls)?.items ?? []).filter((it) => it.status !== 'met').map((it) => it.name)
}

// gateChecklistOf is the checklist a guard's gate('…') call answered, read
// from the option's function answers.
function gateChecklistOf(calls?: api.FunctionAnswer[]): GateChecklist | undefined {
  const a = calls?.find((c) => c.function === 'gate' && !c.error && c.answer && Array.isArray(c.answer.items))
  return a ? (a.answer as GateChecklist) : undefined
}

function RequirementRow({ gate, it, busy, onConfirm }: {
  gate: string
  it: GateItem
  busy: string
  onConfirm: (gate: string, requirement: string, note: string) => Promise<void>
}) {
  const [open, setOpen] = useState(false)
  const [note, setNote] = useState('')
  const t = useT('workspace')
  const icon = it.status === 'met'
    ? <ThemeIcon size={20} radius="xl" color="green" variant="light"><IconCheck size={12} /></ThemeIcon>
    : it.status === 'unmet'
      ? <ThemeIcon size={20} radius="xl" color="red" variant="light"><IconX size={12} /></ThemeIcon>
      : <ThemeIcon size={20} radius="xl" color="orange" variant="light"><IconHelpCircle size={12} /></ThemeIcon>
  const kindLabel = it.kind === 'person' ? t('lifecycle.kindPerson') : it.kind === 'uri' ? t('lifecycle.kindUri') : t('lifecycle.kindObject')
  return (
    <Box>
      <Group gap="xs" wrap="nowrap" align="flex-start">
        {icon}
        <Box style={{ flex: 1 }}>
          <Group gap={6}>
            <Text size="sm" fw={500}>{it.label || it.name}</Text>
            <Text size="xs" c="dimmed">· {kindLabel}</Text>
          </Group>
          {it.reason && it.status !== 'met' && <Text size="xs" c="dimmed">{it.reason}</Text>}
          {it.confirmed_by && (
            <Text size="xs" c="dimmed">
              {t('lifecycle.confirmedBy', { by: it.confirmed_by.by?.id })}{it.confirmed_by.at ? `, ${new Date(it.confirmed_by.at).toLocaleString()}` : ''}
              {it.confirmed_by.note ? ` — ${it.confirmed_by.note}` : ''}
            </Text>
          )}
        </Box>
        {it.kind === 'person' && it.status !== 'met' && !open && (
          <Button size="compact-xs" variant="light" leftSection={<IconCircleDashed size={12} />} onClick={() => setOpen(true)}>
            {t('lifecycle.confirm')}
          </Button>
        )}
      </Group>
      {open && (
        <Box ml={28} mt={4}>
          <Textarea autosize minRows={1} value={note} onChange={(e) => setNote(e.currentTarget.value)}
            placeholder={t('lifecycle.noteOptional')} />
          <Group justify="flex-end" mt={4} gap="xs">
            <Button size="compact-xs" variant="subtle" onClick={() => setOpen(false)}>{t('common.cancel')}</Button>
            <Button size="compact-xs" loading={busy === `${gate}/${it.name}`}
              onClick={() => void onConfirm(gate, it.name, note).then(() => { setOpen(false); setNote('') })}>
              {t('lifecycle.confirm')}
            </Button>
          </Group>
        </Box>
      )}
    </Box>
  )
}
