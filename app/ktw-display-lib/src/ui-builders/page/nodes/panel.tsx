// SPDX-License-Identifier: Apache-2.0
// Renderer of the page DSL's `panel` node: a display panel
// embedded by its id, where the action that started it ran - a card under the
// button, a row of the welcome screen. It follows the panel through the host
// (HostServices.followPanel), drawing it as its events build it (panel.ts):
// its page over its data, its progress, its question, its end; « Ouvrir dans
// un onglet » opens it in a tab of its own (HostServices.openPanel). Without
// a host that follows panels, it shows its title and its id. A tree its open
// names by `{uri, hash}` is drawn from the host's panelTemplate.
//
// `bare` draws its page alone: a question's card placed in a
// conversation, with none of the panel's own frame or controls.
//
// Its question is inline: the choices it offers as radios,
// its form drawn by the page's form node, else a text to write - « Envoyer
// ma réponse » sends the answer back to the run owning the panel through the
// host (HostServices.sendPanelInput), as « Annuler » sends a cancel while the
// work runs. Only the panel's own controls wait for their input.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { Badge, Button, Group, Paper, Progress, Radio, Stack, Text, Textarea } from '@mantine/core'

import { useHostServices } from '../../../ui-core/host/index.tsx'
import { panelAnswer, panelAskForm, panelCancel, panelView, panelTemplateFetch, reducePanel, type PanelInput, type PanelView } from '../panel.ts'
import type { UIPanelNode } from '../ui-page.interactive.ts'
import type { SectionPageView } from './section-load.tsx'
import { usePageContext } from '../page-context.tsx'
import type { UIPageNode } from '../ui-page.types.ts'

const STATE_LABEL: Record<string, { label: string; color: string }> = {
  open: { label: 'en cours', color: 'orange' },
  asking: { label: 'attend votre réponse', color: 'violet' },
  complete: { label: 'terminé', color: 'teal' },
  closed: { label: 'fermé', color: 'gray' },
}

/** panelMeta is the panel's line under its title: its id, its type, its
 *  owner and its sequence, as far as they are known. */
export function panelMeta(v: PanelView): string {
  const parts = [v.id]
  if (v.type) parts.push(`type ${v.type.replace(/^.*\/panel-type\//, '')}`)
  if (v.owner) parts.push(`owner ${v.owner}`)
  if (v.seq > 0) parts.push(`seq ${v.seq}`)
  return parts.join(' · ')
}

/** `initial` is the view a screen already holds (a snapshot it read): drawn
 *  until the panel's next events. */
export function PanelRenderer({ node, Page, initial }: { node: UIPanelNode; Page: SectionPageView; initial?: PanelView }): ReactNode {
  const { followPanel, openPanel, sendPanelInput, panelTemplate } = useHostServices()
  const outer = usePageContext()
  const [view, setView] = useState<PanelView>(() => initial ?? panelView(node.panelId))
  useEffect(() => {
    if (!followPanel) return
    setView((v) => (v.id === node.panelId ? v : panelView(node.panelId)))
    return followPanel(node.panelId, (e) => setView((v) => reducePanel(v, e)))
  }, [followPanel, node.panelId])

  const [choice, setChoice] = useState<string | undefined>(undefined)
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  // A tree named by {uri, hash} (a page template): fetched
  // through the host, its page built with the ops taken meanwhile.
  const spec = view.page ? undefined : view.treeSpec
  useEffect(() => {
    let gone = false
    panelTemplateFetch(view, panelTemplate)?.then(
      (step) => { if (!gone) setView(step) },
      (e: unknown) => { if (!gone) setError(e instanceof Error ? e.message : String(e)) },
    )
    return () => { gone = true }
  }, [panelTemplate, spec?.uri, spec?.hash]) // eslint-disable-line react-hooks/exhaustive-deps
  // A new question starts with its first answer chosen and nothing written.
  useEffect(() => { setChoice(view.ask?.actions?.[0]?.name); setText('') }, [view.ask])
  const send = async (input: PanelInput | null) => {
    if (!input || !sendPanelInput) return
    setSending(true)
    setError('')
    try { await sendPanelInput(input) } catch (e) { setError(e instanceof Error ? e.message : String(e)) } finally { setSending(false) }
  }
  const canAnswer = !!sendPanelInput && !sending && view.state === 'asking'
  const running = view.state === 'open' || view.state === 'asking'
  const onAnswer = (e: FormEvent) => { e.preventDefault(); void send(panelAnswer(view, { action: choice, text })) }
  const ask = view.state === 'asking' ? view.ask : undefined
  const actions = ask?.actions ?? []
  const form = panelAskForm(ask)

  const state = view.state ? STATE_LABEL[view.state] : undefined
  const tree = view.page?.tree as UIPageNode | undefined
  const p = view.progress
  // Bare: its page alone, where it was placed - a question's
  // card in a conversation, its own buttons answering it; no frame, title,
  // id line or panel controls of its own.
  if (node.bare) {
    return (
      <div {...dataAttrs(node)} data-element={`panel:${node.panelId}`} data-state={view.state ?? 'waiting'} data-bare="true">
        {tree ? <Page node={tree} data={view.data} onAction={outer.hasHost ? outer.onAction : undefined} />
          : <Text size="sm" c="dimmed">{view.title ?? node.title ?? 'Panneau'}</Text>}
        {error && <Text size="sm" c="red">{error}</Text>}
      </div>
    )
  }
  return (
    <Paper {...dataAttrs(node)} withBorder p="md" radius="md" data-element={`panel:${node.panelId}`} data-state={view.state ?? 'waiting'}>
      <Stack gap="sm">
        <Group justify="space-between" align="baseline" gap="xs">
          <Text fw={700}>{view.title ?? node.title ?? 'Panneau'}</Text>
          {state && <Badge variant="light" color={state.color} size="sm">{state.label}</Badge>}
        </Group>
        <Text size="xs" c="dimmed" ff="monospace">{panelMeta(view)}</Text>
        {p && (
          <Stack gap={4}>
            <Group justify="space-between" gap="xs">
              {p.label && <Text size="sm">{p.label}</Text>}
              {(p.total ?? 0) > 0 && <Text size="xs" c="dimmed">{p.current} / {p.total}</Text>}
            </Group>
            <Progress value={(p.total ?? 0) > 0 ? Math.min(100, Math.round((p.current / p.total!) * 100)) : 0} size="sm" animated={view.state === 'open'} />
          </Stack>
        )}
        {tree && <Page node={tree} data={view.data} onAction={outer.hasHost ? outer.onAction : undefined} />}
        {ask && (
          <form onSubmit={onAnswer} data-guide="panel.ask">
            <Stack gap="xs" p="sm" style={{ border: '1.5px solid var(--mantine-color-violet-6)', background: 'var(--mantine-color-violet-light)', borderRadius: 8 }}>
              {ask.question && <Text fw={700} size="sm">{ask.question}</Text>}
              {actions.length > 0 && (
                <Radio.Group value={choice} onChange={setChoice}>
                  <Stack gap={6}>
                    {actions.map((a) => <Radio key={a.name} value={a.name} label={a.label ?? a.name} disabled={!canAnswer} />)}
                  </Stack>
                </Radio.Group>
              )}
              {form && (
                <Page node={form} onAction={(e) => { if (canAnswer) void send(panelAnswer(view, { value: e.extra?.value ?? {} })) }} />
              )}
              {actions.length === 0 && !form && (
                <Textarea autosize minRows={2} value={text} onChange={(e) => setText(e.currentTarget.value)} disabled={!canAnswer} placeholder="Votre réponse…" />
              )}
              {!form && (
                <Group gap="xs">
                  <Button type="submit" size="compact-sm" disabled={!canAnswer} loading={sending} data-guide="panel.answer">Envoyer ma réponse</Button>
                </Group>
              )}
              <Text size="xs" c="dimmed">La tâche attend cette réponse : elle peut attendre des jours.</Text>
            </Stack>
          </form>
        )}
        {view.complete && (
          <Text size="sm" c={view.complete.status === 'ok' ? 'teal' : 'red'} data-guide="panel.complete">
            {view.complete.status === 'ok' ? '✓ Terminé' : view.complete.status === 'cancelled' ? 'Annulé' : 'Échec'}
          </Text>
        )}
        {!followPanel && !tree && <Text size="sm" c="dimmed">Le panneau suit son travail dans son onglet.</Text>}
        {error && <Text size="sm" c="red">{error}</Text>}
        <Group gap="xs">
          {running && sendPanelInput && (
            <Button size="compact-sm" variant="default" disabled={sending} data-guide="panel.cancel"
              onClick={() => void send(panelCancel(node.panelId))}>
              Annuler
            </Button>
          )}
          <Button size="compact-sm" variant="default" disabled={!openPanel} data-guide="panel.open-tab"
            onClick={() => openPanel?.(node.panelId)}>
            Ouvrir dans un onglet
          </Button>
        </Group>
      </Stack>
    </Paper>
  )
}
