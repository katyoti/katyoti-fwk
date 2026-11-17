// SPDX-License-Identifier: Apache-2.0
/**
 * DisplayPanel — one display panel drawn by its id: it reads the
 * panel's snapshot from the hub's panel server, follows its events live and
 * draws its type's page over its data (PageHost), its progress, its question
 * and its end, as the display-panels mock-up shows; the person's answer, a
 * chat turn or a cancel goes back to the run owning it. In a tab, inline
 * where the action that started it ran, in the run drawer, and as the
 * selected panel of the panels screen (PanelsScreen) - drawn as
 * the approved mock-up's panel: its title with `<id> · type · owner · seq`,
 * its work, the answer sent, « ✓ <what it said> » once done. Only its own
 * controls ever wait: it never disables the rest of the screen. Its element
 * address is `panel:<id>`.
 */

import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { Badge, Box, Button, Group, Paper, Progress, Radio, Stack, Text, TextInput, Textarea } from '@mantine/core'
import { UIPageView, type UIPageNode } from '@ktw/display-lib/ui-builders/page/index.ts'
import { panelTemplateFetch, panelView, reducePanel, type PanelView } from '@ktw/display-lib/ui-builders/page/panel.ts'

import { PageHost } from '../../components/compositions/PageHost'
import { GuiHostServices } from '../../components/compositions/hostServices'
import { useT } from '../../i18n'
import { usePersonId } from '../../store/profileStore'
import { specCacheOf, templateFetchOf, templateFor } from '../../lib/specCache'
import { followPanelWith, sendPanelInput, wailsPanelTransport, type FollowState, type PanelInput } from '../../lib/displayPanels'
import { answerInput, askFormNode, cancelInput, isChatPanel, messageInput, panelControls, progressShare } from './panelInput'
import { openPanelTab } from './openPanelTab'
import { answerLabel, completionMessage } from './panelList'
import { nextFormIdentity, panelTreeResolve, useDrawnForm, useDrawnTree, type FormIdentity } from './panelTrees'

const STATE_COLOR: Record<string, string> = { open: 'orange', asking: 'violet', complete: 'teal', closed: 'gray' }

export interface DisplayPanelProps {
  universeName: string
  panelId: string
  /** `tab`: the panel alone in its tab; `inline`: a card where it was
   *  started (the run drawer, under an action), with « Ouvrir dans un onglet »;
   *  `board`: the selected panel of the panels screen, a card with
   *  « Ouvrir dans un onglet » beside Cancel. */
  variant?: 'tab' | 'inline' | 'board'
  /** The title shown until the panel's open says its own. */
  title?: string
  /** Called once when the panel completes, with its status (ok, failed,
   *  cancelled) - where it was started reacts to its end. */
  onComplete?: (status: string) => void
}

export function DisplayPanel({ universeName, panelId, variant = 'tab', title, onComplete }: DisplayPanelProps) {
  const t = useT('components')
  const personId = usePersonId()
  const [view, setView] = useState<PanelView>(() => panelView(panelId))
  const [follow, setFollow] = useState<FollowState>('live')
  const [sending, setSending] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [choice, setChoice] = useState<string | undefined>(undefined)
  const [text, setText] = useState('')
  const [message, setMessage] = useState('')
  const [answered, setAnswered] = useState<string | undefined>(undefined)

  useEffect(() => {
    setView(panelView(panelId))
    setFollow('live')
    setAnswered(undefined)
    return followPanelWith(wailsPanelTransport(universeName), panelId, (e) => setView((v) => reducePanel(v, e)), setFollow)
  }, [universeName, panelId])

  // An open naming its tree by {uri, hash} (a page template):
  // the template drawn from the universe's spec cache, the panel server's
  // Template asked once on a miss, with the ops the view kept
  // meanwhile laid over it (panelTemplateFetch).
  const spec = view.page ? undefined : view.treeSpec
  const needsTemplate = !!spec
  useEffect(() => {
    if (!needsTemplate) return
    let gone = false
    panelTemplateFetch(view, (named) => templateFor(specCacheOf(universeName), named, templateFetchOf(universeName)))?.then(
      (step) => { if (!gone) setView(step) },
      (err: unknown) => { if (!gone) setError(String((err as Error)?.message ?? err)) },
    )
    return () => { gone = true }
  }, [needsTemplate, spec?.uri, spec?.hash, universeName]) // eslint-disable-line react-hooks/exhaustive-deps

  // Its end, told once to where it was started.
  const completed = view.complete?.status
  useEffect(() => { if (completed) onComplete?.(completed) }, [completed]) // eslint-disable-line react-hooks/exhaustive-deps

  // A new question starts with its first answer chosen and nothing written.
  useEffect(() => { setChoice(view.ask?.actions?.[0]?.name); setText('') }, [view.ask])

  const live = follow === 'live'
  const controls = panelControls(view, live, sending)
  const by = personId || undefined
  const send = async (input: PanelInput | null, after?: () => void) => {
    if (!input) return
    setSending(true)
    setError(null)
    try {
      await sendPanelInput(universeName, input)
      after?.()
    } catch (err) {
      setError(String((err as Error)?.message ?? err))
    } finally {
      setSending(false)
    }
  }
  const onAnswer = (e: FormEvent) => {
    e.preventDefault()
    const said = answerLabel(view.ask?.actions, { action: choice, text })
    void send(answerInput(view, { action: choice, text }, by), () => setAnswered(said))
  }
  // A question carrying a form: the form drawn, prefilled with its value;
  // its submit sends the form's object as the answer.
  const form = view.state === 'asking' ? askFormNode(view.ask) : undefined
  // The drawn form is keyed by its question: a refusal asks
  // again over the same form with the value just sent, and keeps the key - the
  // form stays mounted and keeps what was typed; a new
  // question or another data type takes a new key and starts fresh from its
  // own value. Worked out once per ask, the value sent used once.
  const sentRef = useRef<unknown>(undefined)
  const formIdentity = useRef<{ ask?: PanelView['ask']; id?: FormIdentity }>({})
  if (form && formIdentity.current.ask !== view.ask) {
    formIdentity.current = { ask: view.ask, id: nextFormIdentity(formIdentity.current.id, form, sentRef.current) }
    sentRef.current = undefined
  }
  const formKey = formIdentity.current.id?.key
  const onFormAnswer = (value: unknown) => {
    if (!controls.answer) return
    sentRef.current = value ?? {}
    void send(answerInput(view, { value: value ?? {} }, by), () => setAnswered(t('displayPanel.ask.formSent')))
  }
  const onMessage = (e: FormEvent) => { e.preventDefault(); void send(messageInput(panelId, message, by), () => setMessage('')) }

  // Every tree drawn resolved first: the page tree - kept as the
  // panel's page once resolved, so ops apply by node id over it - and the
  // question's form - its schema resolved once, its value laid over it, so a
  // question asked again after a refusal draws at once; an
  // error shows in the panel.
  const resolve = useMemo(() => panelTreeResolve(universeName), [universeName])
  const keepResolved = (from: unknown, to: unknown) =>
    setView((v) => (v.page && v.page.tree === from ? { ...v, page: { ...v.page, tree: to } } : v))
  const page = useDrawnTree(view.page?.tree, `${universeName}|${panelId}|page`, resolve, keepResolved)
  const drawnForm = useDrawnForm(form, `${universeName}|${panelId}|form`, resolve)
  const tree = page.tree as UIPageNode | undefined
  const formNode = drawnForm.tree as UIPageNode | undefined
  const p = view.progress
  const meta = useMemo(() => {
    const parts = [view.id]
    if (view.type) parts.push(t('displayPanel.meta.type', { type: view.type.replace(/^.*\/panel-type\//, '') }))
    if (view.owner) parts.push(t('displayPanel.meta.owner', { owner: view.owner }))
    if (view.seq > 0) parts.push(t('displayPanel.meta.seq', { seq: view.seq }))
    return parts.join(' · ')
  }, [view.id, view.type, view.owner, view.seq, t])
  const actions = view.ask?.actions ?? []
  const carded = variant !== 'tab'
  const done = completionMessage(view.complete)

  return (
    <Paper withBorder={carded} p="md" radius="md" mih={variant === 'board' ? 360 : undefined} data-element={`panel:${panelId}`} data-state={view.state ?? 'waiting'} data-follow={follow}>
      <Stack gap="sm">
        <Group justify="space-between" align="baseline" gap="xs" wrap="wrap">
          <Group gap="xs" align="baseline" wrap="wrap">
            <Text fw={700} size={variant === 'inline' ? 'md' : 'lg'}>{view.title ?? title ?? t('displayPanel.untitled')}</Text>
            {variant === 'inline' && view.state && <Badge variant="light" color={STATE_COLOR[view.state] ?? 'gray'} size="sm">{t(`displayPanel.state.${view.state}`)}</Badge>}
          </Group>
          <Text size="xs" c="dimmed" ff="monospace" data-guide="panel.meta">{meta}</Text>
        </Group>

        {!live && (
          <Box p={6} style={{ borderRadius: 6, border: '1px solid var(--mantine-color-orange-6)', background: 'var(--mantine-color-orange-light)' }} data-guide="panel.lost">
            <Text size="sm">{t('displayPanel.lost', { seq: view.seq })}</Text>
          </Box>
        )}

        {p && (
          <Stack gap={4} data-guide="panel.progress">
            <Progress value={progressShare(p)} size="sm" radius="xl" animated={view.state === 'open'} />
            <Text size="sm" c="dimmed">
              {(p.total ?? 0) > 0 ? t('displayPanel.progress', { current: p.current, total: p.total }) : p.label ?? ''}
              {(p.total ?? 0) > 0 && p.label ? ` ${p.label}` : ''}
            </Text>
          </Stack>
        )}

        {page.error && <Text size="sm" c="red" data-guide="panel.page-error">{page.error}</Text>}
        {tree && <PageHost node={tree} data={view.data} universeName={universeName} />}

        {view.state === 'asking' && view.ask && (
          <Box component={form ? 'div' : 'form'} onSubmit={form ? undefined : onAnswer} data-guide="panel.ask">
            <Stack gap="xs" p="sm" style={{ border: '1.5px solid var(--mantine-color-violet-6)', background: 'var(--mantine-color-violet-light)', borderRadius: 8 }}>
              {view.ask.question && <Text fw={700} size="sm">{view.ask.question}</Text>}
              {form && (
                <Box data-guide="panel.ask-form">
                  {drawnForm.error && <Text size="sm" c="red" data-guide="panel.ask-form-error">{drawnForm.error}</Text>}
                  {formNode && (
                    <GuiHostServices universeName={universeName}>
                      <UIPageView key={formKey} node={formNode} onAction={(e) => onFormAnswer(e.extra?.value)} />
                    </GuiHostServices>
                  )}
                </Box>
              )}
              {form ? null : actions.length > 0
                ? (
                  <Radio.Group value={choice} onChange={setChoice}>
                    <Stack gap={6}>
                      {actions.map((a) => <Radio key={a.name} value={a.name} label={a.label ?? a.name} disabled={!controls.answer} />)}
                    </Stack>
                  </Radio.Group>
                )
                : <Textarea autosize minRows={2} value={text} onChange={(e) => setText(e.currentTarget.value)} disabled={!controls.answer} placeholder={t('displayPanel.ask.placeholder')} />}
              {!form && (
                <Group gap="xs">
                  <Button type="submit" size="xs" disabled={!controls.answer} loading={sending} data-guide="panel.answer">{t('displayPanel.ask.send')}</Button>
                </Group>
              )}
              <Text size="xs" c="dimmed">{t('displayPanel.ask.note')}</Text>
            </Stack>
          </Box>
        )}

        {isChatPanel(view) && view.state !== 'complete' && view.state !== 'closed' && (
          <form onSubmit={onMessage} data-guide="panel.chat">
            <Group gap="xs" wrap="nowrap">
              <TextInput style={{ flex: 1 }} value={message} onChange={(e) => setMessage(e.currentTarget.value)} disabled={!controls.message} placeholder={t('displayPanel.chat.placeholder')} />
              <Button type="submit" size="xs" disabled={!controls.message || message.trim() === ''}>{t('displayPanel.chat.send')}</Button>
            </Group>
          </form>
        )}

        {answered && view.state !== 'asking' && (
          <Text size="sm" c="dimmed" data-guide="panel.answered">{t('displayPanel.ask.yours')} <b>{answered}</b></Text>
        )}

        {view.complete && (
          <Text size="sm" c={view.complete.status === 'ok' ? 'teal' : view.complete.status === 'cancelled' ? 'dimmed' : 'red'} data-guide="panel.complete">
            {view.complete.status === 'ok' && done
              ? t('displayPanel.done.okSaid', { message: done })
              : t(`displayPanel.done.${view.complete.status === 'ok' ? 'ok' : view.complete.status === 'cancelled' ? 'cancelled' : 'failed'}`)}
            {view.complete.status !== 'ok' && done ? ` - ${done}` : ''}
          </Text>
        )}
        {error && <Text size="sm" c="red">{error}</Text>}

        {(controls.cancel || carded) && view.state !== 'complete' && view.state !== 'closed' && (
          <Group gap="xs">
            <Button size="xs" variant="default" disabled={!controls.cancel} onClick={() => void send(cancelInput(panelId, by))} data-guide="panel.cancel">
              {t('displayPanel.cancel')}
            </Button>
            {carded && (
              <Button size="xs" variant="default" onClick={() => openPanelTab(universeName, panelId, view.title ?? title)} data-guide="panel.open-tab">
                {t('displayPanel.openTab')}
              </Button>
            )}
          </Group>
        )}
      </Stack>
    </Paper>
  )
}
