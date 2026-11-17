// SPDX-License-Identifier: Apache-2.0
/**
 * MethodPanel — the GENERIC renderer for a `capability-panel` contribution.
 *
 * A node type declares `capabilities: [<id>]`; the GUI fetches the matching
 * `capability-panel` descriptor from the catalog (WorkspaceCapabilityPanel) and
 * renders it here — WITHOUT importing or naming the publishing sidecar. The
 * descriptor declares an optional `state` method (run on load), `options` (e.g.
 * a dry-run toggle) and `actions` (buttons), each an addressable method URI with
 * `${node.*}` / `${opt.*}` templated args resolved server-side by
 * WorkspaceCallMethod. First consumer: the dossier « Confluence » tab.
 *
 * An action runs through the action surface (WorkspaceRunMethod):
 * a method declared long answers its run's display panel, drawn
 * here in place by DisplayPanel - its page (the views it sends, e.g. a summary
 * and a tree of pages), live as it goes and kept once it ended - whatever
 * method, sidecar or panel type; any other method shows its result as text.
 *
 * Bindings are regenerated on `wails build`; the window.go access pattern lets
 * this compile now.
 */

import { useCallback, useEffect, useState } from 'react'
import { Box, Button, Center, Checkbox, Code, Group, Loader, ScrollArea, Stack, Text, TextInput } from '@mantine/core'
import { IconRefresh, IconX } from '@tabler/icons-react'

import { refreshAfterRun, runOutcome, type ActionSpec, type CallResult, type MethodRef, type RunResult } from './methodAction'
import { DisplayPanel } from '../panels/DisplayPanel'
import { useT } from '../../i18n'

interface OptionSpec { id: string; label: string; type: string; default?: unknown; placeholder?: string }
interface Descriptor {
  id: string
  label?: string
  description?: string
  note?: string
  state?: MethodRef & { label?: string }
  options?: OptionSpec[]
  actions?: ActionSpec[]
  result?: string
}

/** capabilityId fetches the descriptor from the catalog (the node pane);
 *  descriptor passes it directly (the object pane, where it is the
 *  method-panel extension's input). */
interface MethodPanelProps { universeName: string; nodeId: string; capabilityId?: string; descriptor?: unknown }

interface CapResult { uri?: string; descriptor?: string; error?: string }
type Bindings = {
  go?: { main?: { App?: {
    WorkspaceCapabilityPanel?: (universeName: string, capabilityId: string) => Promise<CapResult>
    WorkspaceCallMethod?: (universeName: string, nodeId: string, methodURI: string, argsJSON: string, optionsJSON: string) => Promise<CallResult>
    WorkspaceRunMethod?: (universeName: string, nodeId: string, methodURI: string, argsJSON: string, optionsJSON: string) => Promise<RunResult>
  } } }
}
const bapp = () => (window as unknown as Bindings).go?.main?.App

// Render a method result as a text report. Result bodies commonly carry
// { report, summary } (e.g. the confluence wrappers); fall back to the raw JSON.
function reportText(r: CallResult | null): string {
  if (!r) return ''
  if (r.error) return r.error
  if (!r.result) return ''
  try {
    const o = JSON.parse(r.result) as { report?: string; summary?: string }
    return o.report?.trim() || o.summary || r.result
  } catch {
    return r.result
  }
}

export function MethodPanel({ universeName, nodeId, capabilityId, descriptor: given }: MethodPanelProps) {
  const [descriptor, setDescriptor] = useState<Descriptor | null>(null)
  const [descErr, setDescErr] = useState<string | null>(null)
  const [opts, setOpts] = useState<Record<string, unknown>>({})
  const [stateResult, setStateResult] = useState<CallResult | null>(null)
  const [stateLoading, setStateLoading] = useState(false)
  const [busy, setBusy] = useState<string | null>(null)
  const [actionResult, setActionResult] = useState<CallResult | null>(null)
  // The display panel of the long method run last, drawn in place.
  const [runPanel, setRunPanel] = useState<{ id: string; label?: string; act: ActionSpec } | null>(null)
  const t = useT('workspace')

  // 1 — fetch the descriptor.
  useEffect(() => {
    let cancelled = false
    const adopt = (d: Descriptor) => {
      const seed: Record<string, unknown> = {}
      for (const o of d.options ?? []) seed[o.id] = o.default
      setOpts(seed)
      setDescriptor(d)
    }
    if (given) { adopt(given as Descriptor); return }
    if (!capabilityId) { setDescErr(t('method.noDescriptor')); return }
    const app = bapp()
    if (!app?.WorkspaceCapabilityPanel) { setDescErr(t('method.bindingRebuild')); return }
    app.WorkspaceCapabilityPanel(universeName, capabilityId)
      .then((r) => {
        if (cancelled) return
        if (r.error || !r.descriptor) { setDescErr(r.error || t('method.emptyDescriptor')); return }
        try {
          adopt(JSON.parse(r.descriptor) as Descriptor)
        } catch (e) { setDescErr(t('method.unreadableDescriptor', { error: String(e) })) }
      })
      .catch((e: unknown) => { if (!cancelled) setDescErr(String(e)) })
    return () => { cancelled = true }
  }, [universeName, capabilityId, given, t])

  const call = useCallback(
    async (ref: MethodRef): Promise<CallResult> => {
      const app = bapp()
      if (!app?.WorkspaceCallMethod) return { error: t('method.bindingUnavailable') }
      return app.WorkspaceCallMethod(universeName, nodeId, ref.method, JSON.stringify(ref.args ?? {}), JSON.stringify(opts))
    },
    [universeName, nodeId, opts, t],
  )

  const runMethod = useCallback(
    async (ref: MethodRef): Promise<RunResult> => {
      const app = bapp()
      if (!app?.WorkspaceRunMethod) return { error: t('method.bindingUnavailable') }
      return app.WorkspaceRunMethod(universeName, nodeId, ref.method, JSON.stringify(ref.args ?? {}), JSON.stringify(opts))
    },
    [universeName, nodeId, opts, t],
  )

  const refreshState = useCallback(() => {
    if (!descriptor?.state) return
    setStateLoading(true)
    call(descriptor.state)
      .then((r) => { setStateResult(r); setStateLoading(false) })
      .catch((e: unknown) => { setStateResult({ error: String(e) }); setStateLoading(false) })
  }, [descriptor, call])

  // 2 — run the state method once the descriptor arrives.
  useEffect(() => { if (descriptor?.state) refreshState() }, [descriptor]) // eslint-disable-line react-hooks/exhaustive-deps

  const runAction = (act: ActionSpec) => {
    setBusy(act.method)
    setActionResult(null)
    // Through the action surface: a long method answers at once with its
    // run's panel, drawn below in place; the buttons wait only for that
    // answer, never for the run.
    runMethod(act)
      .then((r) => {
        const o = runOutcome(r)
        setBusy(null)
        if ('panel' in o) { setRunPanel({ id: o.panel, label: act.label, act }); return }
        setRunPanel(null)
        setActionResult(o.result)
        if (!o.result.error && act.refreshStateOnSuccess) refreshState()
      })
      .catch((e: unknown) => { setActionResult({ error: String(e) }); setBusy(null) })
  }

  if (descErr) {
    return (
      <Box p="md">
        <Text size="sm" c="red">{t('method.unavailable')}</Text>
        <Text size="xs" c="dimmed">{descErr}</Text>
      </Box>
    )
  }
  if (!descriptor) return <Center py="xl"><Loader size="sm" /></Center>

  const boolOptions = (descriptor.options ?? []).filter((o) => o.type === 'bool')
  // Text options (a page, a space, a title) an action reads as ${opt.<id>}.
  const textOptions = (descriptor.options ?? []).filter((o) => o.type === 'string')

  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="md">
        <Stack gap="lg">
          {descriptor.state && (
            <div>
              <Group justify="space-between" mb="xs">
                <Text fw={600} size="sm">{descriptor.state.label || t('method.state')}</Text>
                <Button size="xs" variant="light" leftSection={<IconRefresh size={14} />} loading={stateLoading} onClick={refreshState}>
                  {t('method.refresh')}
                </Button>
              </Group>
              {stateLoading ? (
                <Center py="md"><Loader size="sm" /></Center>
              ) : stateResult?.error ? (
                <Text size="sm" c="red">{stateResult.error}</Text>
              ) : (
                <Code block style={{ whiteSpace: 'pre-wrap', maxHeight: 280, overflow: 'auto' }}>
                  {reportText(stateResult) || t('method.noState')}
                </Code>
              )}
            </div>
          )}

          {(descriptor.actions?.length ?? 0) > 0 && (
            <div>
              <Group justify="space-between" mb="xs">
                <Text fw={600} size="sm">{t('method.actions')}</Text>
                <Group gap="sm">
                  {boolOptions.map((o) => (
                    <Checkbox
                      key={o.id}
                      size="xs"
                      label={o.label}
                      checked={Boolean(opts[o.id])}
                      onChange={(e) => setOpts((p) => ({ ...p, [o.id]: e.currentTarget.checked }))}
                    />
                  ))}
                </Group>
              </Group>
              {textOptions.length > 0 && (
                <Group gap="sm" mb="xs" align="flex-end">
                  {textOptions.map((o) => (
                    <TextInput
                      key={o.id}
                      size="xs"
                      label={o.label}
                      placeholder={o.placeholder}
                      value={typeof opts[o.id] === 'string' ? (opts[o.id] as string) : ''}
                      onChange={(e) => { const v = e.currentTarget.value; setOpts((p) => ({ ...p, [o.id]: v })) }}
                      style={{ minWidth: 200 }}
                    />
                  ))}
                </Group>
              )}
              <Group gap="sm">
                {(descriptor.actions ?? []).map((act) => (
                  <Button
                    key={act.method}
                    size="xs"
                    variant={act.variant === 'light' ? 'light' : 'filled'}
                    loading={busy === act.method}
                    disabled={busy !== null}
                    onClick={() => runAction(act)}
                  >
                    {act.label}
                  </Button>
                ))}
              </Group>
              {runPanel && (
                <Box mt="sm" data-element={`method-panel:run:${runPanel.id}`}>
                  <Group justify="flex-end" mb="xs">
                    <Button size="xs" variant="subtle" color="gray" leftSection={<IconX size={14} />} onClick={() => setRunPanel(null)}>
                      {t('method.close')}
                    </Button>
                  </Group>
                  <DisplayPanel universeName={universeName} panelId={runPanel.id} variant="inline" title={runPanel.label}
                    onComplete={(status) => { if (refreshAfterRun(runPanel.act, status)) refreshState() }} />
                </Box>
              )}
              {!runPanel && actionResult && (
                <Box mt="sm">
                  <Text size="xs" c={actionResult.error ? 'red' : 'dimmed'} mb={4}>
                    {actionResult.error ? t('method.failed') : t('method.done')}
                  </Text>
                  {reportText(actionResult) && (
                    <Code block style={{ whiteSpace: 'pre-wrap', maxHeight: 280, overflow: 'auto' }}>
                      {reportText(actionResult)}
                    </Code>
                  )}
                </Box>
              )}
            </div>
          )}

          {descriptor.note && <Text size="xs" c="dimmed">{descriptor.note}</Text>}
        </Stack>
      </Box>
    </ScrollArea>
  )
}
