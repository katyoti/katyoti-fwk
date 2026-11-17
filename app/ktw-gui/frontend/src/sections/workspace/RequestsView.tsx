// SPDX-License-Identifier: Apache-2.0
/**
 * RequestsView — the work asked of a module, drawn as a UI extension. The panel
 * is DECLARED by the planification extension (`object-panels/requests.yaml`,
 * filter: the `module` capability); this view calls the services itself —
 * `ktw-agents:requests/demands` for the module, the plan service's verbs from
 * its buttons — and reloads after each move, as What's Next does.
 *
 * Three views: Open, Mine (what the person at the screen asked) and History
 * (ended, the latest first). Layout follows the method panels: a dimmed
 * headline with the counts, the lifecycle as a strip of states, then one
 * section per state — a card per demand (a request, a feature, a goal), its
 * border the state's colour, its deliverables, what it still waits on, the step
 * it is on (who holds it; "needs you" when a person's step is up) and why it
 * waits — state and step read from its to-dos by the service, the moves its state allows, and its PARTS nested under it
 * (in any module), so a demand is followed to the modules it went to.
 */

import { useCallback, useMemo, useState } from 'react'
import {
  Badge, Box, Button, Collapse, Group, Progress, ScrollArea, SegmentedControl, Stack, Text,
  ThemeIcon, Timeline, Title, Tooltip,
} from '@mantine/core'
import { ItemCard, toneBorder } from '@ktw/display-lib/ui-core/components/other/index.ts'
import {
  IconCheck, IconChevronDown, IconChevronRight, IconCircleDashed, IconClock, IconCoin, IconHandStop,
  IconBox, IconHelpCircle, IconInbox, IconLink, IconMessageCircle, IconMessageReply, IconPlayerPause, IconPlayerPlay, IconPlus,
  IconRobot, IconTrash, IconUser,
} from '@tabler/icons-react'
import { Markdown } from '../../components/Markdown'
import { WorkspaceRunAction } from '../../../wailsjs/go/main/App'
import { main } from '../../../wailsjs/go/models'
import { ActionLaunchModal } from './ActionLaunchModal'
import { usePersonId } from '../../store/profileStore'
import { useAppStore } from '../../store'
import { useT, type TFn } from '../../i18n'
import { errorText } from '../../i18n/errors'
import { useInboxLauncher } from '../../hooks/useInboxLauncher'
import { runMethod } from '../../components/compositions/hostServices'
import { LoadedPart, useHostLoad } from '../../hooks/useHostLoad'

interface Actor { id: string; kind: string }
interface Deliverable { title: string; check?: string; verified?: boolean; verified_at?: string }
interface PlanStep { task?: string; phase: string; group?: string; doer?: string; interaction?: string; step?: string }
interface Plan { state?: string; next?: PlanStep; note?: string; run?: string }
/** The step a demand is on, read from its to-dos: who does it, who holds it,
 *  and whether a person must act (then where it was asked). */
interface DemandStep {
  step: string; phase?: string; who?: string; interaction?: string
  held_by?: Actor; until?: string; needs_you?: boolean; ask?: string
}
interface Answer { text: string; by: Actor; at?: string }
interface TimelineEntry {
  at: string; kind: 'step' | 'move'; title: string; note?: string; state?: string; by?: string
  run?: string; session?: string; ask?: string
}

interface Request {
  id: string
  type?: string
  module: string
  title: string
  state: string
  /** Why it is not moving: a waiting / blocked plan's note, or why the planner holds a ready one. */
  why?: string
  /** When it ended (history). */
  ended_at?: string
  /** The demands split from it, in any module. */
  parts?: Request[]
  depends_on?: string[]
  /** Its dependencies not ended yet. */
  waits_on?: string[]
  /** The step it is on (open demands). */
  step?: DemandStep
  requester?: Actor
  details?: string
  deliverables?: Deliverable[]
  budget_usd?: number
  /** Its plan: the next step and where it stands (its state is the plan's). */
  plan?: Plan
  kind?: string
  /** A question's answer, once given (requests Answer). */
  answer?: Answer
  /** The object it concerns. */
  about?: string
  /** Its activity, oldest first: steps (each with its own session) and plan moves. */
  timeline?: TimelineEntry[]
}

const SERVICE = 'ktw-agents:requests/'
const PLANS = 'ktw-agents:plans/'


/** A request's states — its plan's — in order, each with its colour. The
 *  label is the English word the timeline's states carry (colorOfLabel); the
 *  person reads `requests.state.<name>`. */
const STATES: { name: string; label: string; color: string; open: boolean }[] = [
  { name: 'ready', label: 'Ready', color: 'teal', open: true },
  { name: 'running', label: 'Running', color: 'blue', open: true },
  { name: 'idle', label: 'Waiting', color: 'orange', open: true },
  { name: 'blocked', label: 'Blocked', color: 'red', open: true },
  { name: 'split', label: 'Split', color: 'violet', open: true },
  { name: 'done', label: 'Done', color: 'green', open: false },
  { name: 'dropped', label: 'Dropped', color: 'gray', open: false },
]
const colorOf = (state: string) => STATES.find((s) => s.name === state)?.color ?? 'gray'
/** A state's color by the label a person reads (`waiting` is the idle state). */
const colorOfLabel = (label?: string) => STATES.find((s) => s.label.toLowerCase() === label)?.color ?? 'gray'

function SectionTitle({ children }: { children: React.ReactNode }) {
  return <Title order={6} mb="xs" c="dimmed" tt="uppercase" style={{ letterSpacing: 0.5 }}>{children}</Title>
}

/** The moves a state allows, as the plan workflow declares them: re-evaluate
 *  plans its intake again; drop ends it. */
type Move = { verb: string; color: string; Icon: typeof IconCheck }
function movesOf(state: string): Move[] {
  const drop: Move = { verb: 'drop', color: 'gray', Icon: IconTrash }
  const again: Move = { verb: 'reevaluate', color: 'blue', Icon: IconCircleDashed }
  switch (state) {
    case 'idle':
    case 'blocked':
      return [again, drop]
    case 'ready':
    case 'running':
      return [drop]
    default:
      return []
  }
}

/** The step it is on, in words: the task or persona, or the person's ask. */
function stepLabel(s: DemandStep, t: TFn): string {
  const who = s.who === 'person' ? t('requests.personTo', { what: s.interaction ?? t('requests.answerVerb') }) : (s.who ?? '')
  return who ? `${s.step} · ${who}` : s.step
}

function RequestCard({ r, home, stateOf, busy, onMove, onSession, onAnswer }: {
  r: Request
  /** The module the panel is on: a part elsewhere shows its module. */
  home: string
  stateOf: (id: string) => string | undefined
  busy: boolean
  onMove: (r: Request, verb: string, extra?: Record<string, unknown>) => void
  onSession: (sessionId: string) => void
  onAnswer: (r: Request) => void
}) {
  const [open, setOpen] = useState(false)
  const [activity, setActivity] = useState(false)
  const color = colorOf(r.state)
  const deliverables = r.deliverables ?? []
  const verified = deliverables.filter((d) => d.verified).length
  const deps = r.waits_on ?? []
  const { launchInbox } = useInboxLauncher()
  const why = r.why || ((r.state === 'idle' || r.state === 'blocked') ? r.plan?.note : undefined)
  const parts = r.parts ?? []
  const question = r.kind === 'question'
  const ended = r.state === 'done' || r.state === 'dropped'
  const hasMore = !!r.details || deliverables.length > 0 || !!r.answer
  const t = useT('workspace')
  return (
    <ItemCard tone={color}>
      <Group justify="space-between" wrap="nowrap" gap="xs" align="flex-start">
        <Box style={{ minWidth: 0, flex: 1, cursor: hasMore ? 'pointer' : undefined }} onClick={() => hasMore && setOpen((o) => !o)}>
          <Group gap={6} wrap="nowrap">
            {hasMore && (open ? <IconChevronDown size={14} /> : <IconChevronRight size={14} />)}
            <Text size="sm" fw={600} truncate>{r.title}</Text>
          </Group>
          <Group gap={6} mt={4}>
            <Text size="xs" c="dimmed" ff="monospace">{r.id}</Text>
            {r.type && r.type !== 'request' && (
              <Badge size="xs" variant="light" color="indigo">{r.type}</Badge>
            )}
            {r.module !== home && (
              <Tooltip label={t('requests.inModule', { module: r.module })} withArrow>
                <Badge size="xs" variant="outline" color="indigo" style={{ textTransform: 'none' }}
                  leftSection={<IconBox size={10} />}>{r.module}</Badge>
              </Tooltip>
            )}
            {ended && r.ended_at && (
              <Text size="xs" c="dimmed">{t('requests.ended', { day: r.ended_at.slice(0, 10) })}</Text>
            )}
            {question && (
              <Badge size="xs" variant="light" color="cyan" leftSection={<IconHelpCircle size={10} />}>{t('requests.question')}</Badge>
            )}
            {r.requester && (
              <Tooltip label={r.requester.kind === 'module' ? t('requests.askedByModule', { id: r.requester.id }) : t('requests.askedBy', { id: r.requester.id })} withArrow>
                <Badge size="xs" variant="light" color="gray" style={{ textTransform: 'none' }}
                  leftSection={r.requester.kind === 'agent' ? <IconRobot size={10} />
                    : r.requester.kind === 'module' ? <IconBox size={10} /> : <IconUser size={10} />}>
                  {r.requester.id}
                </Badge>
              </Tooltip>
            )}
            {r.about && (
              <Tooltip label={t('requests.about', { about: r.about })} withArrow>
                <Badge size="xs" variant="outline" color="gray" style={{ textTransform: 'none' }}
                  leftSection={<IconLink size={10} />}>{r.about}</Badge>
              </Tooltip>
            )}
            {!!r.budget_usd && (
              <Badge size="xs" variant="light" color="yellow" leftSection={<IconCoin size={10} />}>${r.budget_usd}</Badge>
            )}
            {deliverables.length > 0 && (
              <Tooltip label={t('requests.deliverablesVerified', { verified, count: deliverables.length })} withArrow>
                <Badge size="xs" variant="light" color={verified === deliverables.length ? 'green' : 'gray'}
                  leftSection={<IconCheck size={10} />}>{verified}/{deliverables.length}</Badge>
              </Tooltip>
            )}
          </Group>
        </Box>
        <Group gap={4} wrap="nowrap">
          {(r.timeline?.length ?? 0) > 0 && (
            <Tooltip label={t('requests.activityTip')} withArrow>
              <Button size="compact-xs" variant={activity ? 'light' : 'subtle'} color="violet"
                leftSection={<IconMessageCircle size={12} />} onClick={() => setActivity((a) => !a)}>
                {t('requests.activity', { count: r.timeline!.length })}
              </Button>
            </Tooltip>
          )}
          {question && !ended && (
            <Button size="compact-xs" variant="light" color="cyan" disabled={busy}
              leftSection={<IconMessageReply size={12} />} onClick={() => onAnswer(r)}>{t('requests.answer')}</Button>
          )}
          {movesOf(r.state).map((m) => (
            <Button key={m.verb} size="compact-xs" variant="light" color={m.color} disabled={busy}
              leftSection={<m.Icon size={12} />} onClick={() => onMove(r, m.verb)}>{t(`requests.move.${m.verb}`)}</Button>
          ))}

        </Group>
      </Group>

      {r.step && !ended && (r.step.needs_you ? (
        <Group gap={6} mt={6} wrap="nowrap">
          <Badge size="sm" variant="filled" color="orange" leftSection={<IconUser size={12} />}>
            {t('requests.needsYou', { what: r.step.interaction ?? t('requests.answerVerb'), step: r.step.step })}
          </Badge>
          <Tooltip label={r.step.ask ? t('requests.inInboxTip', { ask: r.step.ask }) : t('requests.inInbox')} withArrow>
            <Button size="compact-xs" variant="light" color="orange" leftSection={<IconInbox size={12} />}
              onClick={launchInbox}>{t('requests.inInbox')}</Button>
          </Tooltip>
        </Group>
      ) : (
        <Group gap={6} mt={6} wrap="nowrap">
          <IconPlayerPlay size={14} color="var(--mantine-color-dimmed)" />
          <Text size="xs" c="dimmed" truncate>
            {t('requests.onStep')} <b>{stepLabel(r.step, t)}</b>{r.step.phase ? ` · ${r.step.phase}` : ''}
            {r.step.held_by && <> · {t('requests.heldBy', { id: r.step.held_by.id, until: r.step.until ? new Date(r.step.until).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '' })}</>}
          </Text>
        </Group>
      ))}
      {why && (
        <Group gap={6} mt={6} wrap="nowrap">
          <IconHandStop size={14} color={`var(--mantine-color-${color}-6)`} />
          <Text size="xs" c={ended ? 'dimmed' : (r.state === 'ready' ? 'teal' : color)} truncate>{why}</Text>
        </Group>
      )}

      {deps.length > 0 && (
        <Group gap={6} mt={6}>
          <Text size="xs" c="dimmed">{t('requests.waitsOn')}</Text>
          {deps.map((d) => {
            const st = stateOf(d)
            return (
              <Tooltip key={d} label={st ? t('requests.depIs', { dep: d, state: t(`requests.state.${st}`, { defaultValue: st }) }) : t('requests.depElsewhere', { dep: d })} withArrow>
                <Badge size="xs" variant={st === 'done' ? 'light' : 'outline'} color={st ? colorOf(st) : 'gray'}
                  style={{ textTransform: 'none' }} leftSection={<IconLink size={10} />}>{d}</Badge>
              </Tooltip>
            )
          })}
        </Group>
      )}

      <Collapse in={open}>
        <Stack gap="xs" mt="sm">
          {r.answer && (
            <Box p="xs" style={{ ...toneBorder('cyan'), background: 'var(--mantine-color-default-hover)' }}>
              <Text size="xs" c="dimmed" mb={4}>{t('requests.answeredBy', { id: r.answer.by.id })}{r.answer.at ? ` · ${r.answer.at.slice(0, 10)}` : ''}</Text>
              <Markdown text={r.answer.text} />
            </Box>
          )}
          {deliverables.length > 0 && (
            <Stack gap={4}>
              {deliverables.map((d, i) => (
                <Group key={`${d.title}:${i}`} gap={8} wrap="nowrap">
                  <ThemeIcon size="sm" variant="light" color={d.verified ? 'green' : 'gray'}>
                    {d.verified ? <IconCheck size={12} /> : <IconCircleDashed size={12} />}
                  </ThemeIcon>
                  <Text size="sm" style={{ flex: 1, minWidth: 0 }} truncate>{d.title}</Text>
                  {d.check && <Text size="xs" c="dimmed" ff="monospace" truncate maw="45%">{d.check}</Text>}
                </Group>
              ))}
            </Stack>
          )}
          {r.details && (
            <Box style={{ fontSize: 'var(--mantine-font-size-sm)' }}><Markdown text={r.details} /></Box>
          )}
        </Stack>
      </Collapse>

      <Collapse in={activity}>
        <Box mt="sm" pl={4}>
          <Timeline bulletSize={18} lineWidth={2} active={(r.timeline?.length ?? 1) - 1} color="violet">
            {(r.timeline ?? []).map((e, i) => (
              <Timeline.Item key={`${e.at}:${i}`}
                bullet={e.kind === 'step' ? <IconPlayerPlay size={10} /> : undefined}
                color={e.kind === 'step' ? 'violet' : colorOfLabel(e.state)}
                title={<Group gap={6} wrap="nowrap">
                  <Text size="sm" fw={e.kind === 'step' ? 600 : 500}>{e.title}</Text>
                  {e.state && <Badge size="xs" variant="light" color={colorOfLabel(e.state)}>{e.state}</Badge>}
                  {e.session && (
                    <Button size="compact-xs" variant="subtle" color="violet" onClick={() => onSession(e.session!)}>
                      {t('requests.openSession')}
                    </Button>
                  )}
                  {e.ask && <Text size="xs" c="orange">{t('requests.answerInInbox')}</Text>}
                </Group>}>
                <Text size="xs" c="dimmed">
                  {e.at.slice(0, 16).replace('T', ' ')}{e.by ? ` · ${e.by}` : ''}{e.note ? ` — ${e.note}` : ''}
                </Text>
              </Timeline.Item>
            ))}
          </Timeline>
        </Box>
      </Collapse>

      {parts.length > 0 && (
        <Stack gap="xs" mt="sm" pl="sm" style={{ borderLeft: '2px solid var(--mantine-color-default-border)' }}>
          <Text size="xs" c="dimmed">{t('requests.parts', { count: parts.length })}</Text>
          {parts.map((p) => (
            <RequestCard key={p.id} r={p} home={home} stateOf={stateOf} busy={busy} onMove={onMove}
              onSession={onSession} onAnswer={onAnswer} />
          ))}
        </Stack>
      )}
    </ItemCard>
  )
}

/** Every demand of a tree, the root first. */
function flatten(rs: Request[]): Request[] {
  return rs.flatMap((r) => [r, ...flatten(r.parts ?? [])])
}

type View = 'open' | 'mine' | 'history'

function StateSection({ state, list, children }: {
  state: (typeof STATES)[number]; list: Request[]; children: (r: Request) => React.ReactNode
}) {
  const [open, setOpen] = useState(state.open)
  const t = useT('workspace')
  return (
    <Box>
      <Group gap={6} onClick={() => setOpen((o) => !o)} style={{ cursor: 'pointer' }} mb={open ? 0 : 'xs'}>
        {open ? <IconChevronDown size={12} color="var(--mantine-color-dimmed)" /> : <IconChevronRight size={12} color="var(--mantine-color-dimmed)" />}
        <SectionTitle>{t(`requests.state.${state.name}`)} ({list.length})</SectionTitle>
      </Group>
      <Collapse in={open}>
        <Stack gap="sm">{list.map((r) => children(r))}</Stack>
      </Collapse>
    </Box>
  )
}

export function RequestsView({ universeName, moduleId }: { universeName: string; moduleId: string }) {
  const setMainPaneRoute = useAppStore((s) => s.setMainPaneRoute)
  const [view, setView] = useState<View>('open')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState<{ text: string; color: string } | null>(null)
  const [filing, setFiling] = useState(false)
  const [answering, setAnswering] = useState<Request | null>(null)
  // Who moves a request from the GUI: the person at the screen (their profile).
  const personId = usePersonId()
  const t = useT('workspace')
  const person: Actor = useMemo(() => ({ id: personId, kind: 'person' }), [personId])

  // The requests, read through the host's one load: its
  // placeholder, its error card with « Réessayer »; a move re-reads them, the
  // list kept on screen until the answer.
  const demands = JSON.stringify({ module: moduleId, view, person: personId })
  const listed = useHostLoad<Request[]>({
    universe: universeName, key: `demands|${demands}`,
    load: async (signal) => ((await runMethod(universeName, SERVICE + 'demands', demands, signal)) as { demands?: Request[] } | null)?.demands ?? [],
  })
  const requests = listed.data
  const load = listed.reread

  const move = useCallback(async (r: Request, verb: string) => {
    setBusy(true); setMsg(null)
    try {
      const [uri, input] = verb === 'drop'
        ? [PLANS + 'finish', { id: r.id, outcome: 'dropped', by: person }]
        : [PLANS + 'plan', { id: r.id, next: { task: 'intake', phase: 'requests' }, by: person, note: 're-evaluate' }]
      const res = await WorkspaceRunAction(universeName, uri, JSON.stringify(input))
      setMsg(res.error ? { text: errorText(res), color: 'red' } : { text: t('requests.moveDone', { id: r.id, verb: t(`requests.move.${verb}`) }), color: 'teal' })
    } catch (e) { setMsg({ text: String(e), color: 'red' }) }
    load()
    setBusy(false)
  }, [universeName, load, person, t])

  const byState = useMemo(() => {
    const m: Record<string, Request[]> = {}
    for (const r of requests ?? []) (m[r.state] ??= []).push(r)
    return m
  }, [requests])
  const stateById = useMemo(() => new Map(flatten(requests ?? []).map((r) => [r.id, r.state])), [requests])

  const answerAction = useMemo(() => answering && main.WorkspaceQuickAction.createFrom({
    uri: SERVICE + 'answer', label: t('requests.answerTitle', { title: answering.title }), kind: 'service', source: 'planification',
    boundInput: { id: answering.id, by: person },
  }), [answering, person, t])

  const fileAction = useMemo(() => main.WorkspaceQuickAction.createFrom({
    uri: SERVICE + 'file', label: t('requests.file'), kind: 'service', source: 'planification',
    boundInput: { module: moduleId, by: person },
  }), [moduleId, person, t])

  if (!requests || listed.error) return <LoadedPart id="requests" part={listed}>{() => null}</LoadedPart>

  const total = requests.length
  const done = byState['done']?.length ?? 0
  const live = total - done - (byState['dropped']?.length ?? 0)
  const share = total > 0 ? (done / Math.max(1, total - (byState['dropped']?.length ?? 0))) * 100 : 0

  return (
    <ScrollArea style={{ height: '100%' }}>
      <Stack gap="lg" p="md">
        {/* Headline — how much is asked, and how much of it is done. */}
        <Group justify="space-between" wrap="nowrap" align="flex-start">
          <Box style={{ flex: 1 }}>
            <Text size="sm" c="dimmed">
              <b>{total}</b> {t('requests.requestWord', { count: total })} · <b>{live}</b> {t('requests.openWord', { count: live })}
              {(byState['ready']?.length ?? 0) > 0 && <> · <Text span c="teal">{t('requests.nReady', { count: byState['ready'].length })}</Text></>}
              {(byState['running']?.length ?? 0) > 0 && <> · <Text span c="blue">{t('requests.nRunning', { count: byState['running'].length })}</Text></>}
              {(byState['idle']?.length ?? 0) > 0 && <> · <Text span c="orange">{t('requests.nWaiting', { count: byState['idle'].length })}</Text></>}
              {(byState['blocked']?.length ?? 0) > 0 && <> · <Text span c="red">{t('requests.nBlocked', { count: byState['blocked'].length })}</Text></>}
            </Text>
            <Tooltip label={t('requests.nDone', { count: done })} withArrow>
              <Progress value={share} size="sm" mt={6} color="green" />
            </Tooltip>
          </Box>
          <Button size="compact-sm" variant="light" color="violet" leftSection={<IconPlus size={14} />}
            onClick={() => setFiling(true)}>{t('requests.file')}</Button>
        </Group>

        <SegmentedControl size="xs" value={view} onChange={(v) => setView(v as View)} data={[
          { value: 'open', label: t('requests.view.open') },
          { value: 'mine', label: t('requests.view.mine') },
          { value: 'history', label: t('requests.view.history') },
        ]} />

        {/* The lifecycle, each state with how many requests are in it. */}
        <Group gap={6}>
          {STATES.map((s) => {
            const n = byState[s.name]?.length ?? 0
            return (
              <Badge key={s.name} size="sm" variant={n > 0 ? 'light' : 'outline'} color={n > 0 ? s.color : 'gray'}>
                {t(`requests.state.${s.name}`)} {n}
              </Badge>
            )
          })}
        </Group>

        {msg && <Text size="xs" c={msg.color}>{msg.text}</Text>}

        {total === 0 ? (
          <Text size="sm" c="dimmed">{view === 'history' ? t('requests.emptyHistory')
            : view === 'mine' ? t('requests.emptyMine')
              : t('requests.emptyOpen')}</Text>
        ) : (
          STATES.filter((s) => (byState[s.name]?.length ?? 0) > 0).map((s) => (
            <StateSection key={s.name} state={s} list={byState[s.name]}>
              {(r) => <RequestCard key={r.id} r={r} home={moduleId} busy={busy} onMove={move} stateOf={(id) => stateById.get(id)}
                onAnswer={setAnswering}
                onSession={(sessionId) => setMainPaneRoute({ kind: 'session', universeName, sessionId })} />}
            </StateSection>
          ))
        )}
      </Stack>

      {answerAction && (
        <ActionLaunchModal action={answerAction} universeName={universeName} nodeId={moduleId}
          onClose={() => setAnswering(null)}
          onResult={(r) => {
            setAnswering(null)
            setMsg(r.error ? { text: errorText(r), color: 'red' } : { text: t('requests.answered'), color: 'teal' })
            load()
          }} />
      )}

      {filing && (
        <ActionLaunchModal action={fileAction} universeName={universeName} nodeId={moduleId}
          onClose={() => setFiling(false)}
          onResult={(r) => {
            setFiling(false)
            setMsg(r.error ? { text: errorText(r), color: 'red' } : { text: t('requests.filed'), color: 'teal' })
            load()
          }} />
      )}
    </ScrollArea>
  )
}
