// SPDX-License-Identifier: Apache-2.0
// <PageHost> — a declared panel's page, rendered IN-PROCESS (no iframe): the
// display library's UIPageView in the GUI's own React tree, with the host's
// action handler. A page's interactive nodes (a button, an item's action, a
// link, a badge, a drop on a lane, a click on a bar or a box) hand it a
// declarative action; the host resolves its templates against the item, the
// panel's scope (`id`, `target`) and the person at the screen (`${person}`),
// asks a confirmation when the action declares one or is dangerous, and does
// the action's ONE verb:
//
//   - uri      → runs the method (WorkspaceRunAction). An async run (a task)
//                opens Global → Runs; a sync answer following the result
//                convention {ok, message, open?} is shown as written and its
//                `open` navigated to; then the panel re-reads;
//   - navigate → opens the object, on the panel named (openObjectTab);
//   - chat     → opens a chat running the task on the target (the chat launcher);
//   - create   → the new-object dialog (ObjectCreateModal), then a re-read;
//   - query    → the panel re-reads its source with the args over its input
//                (onQuery: a filter, a saved view); without a source, nothing;
//   - drawer   → a panel of an object in a side drawer (PanelDrawer); a
//                file of an object read in place in a reader drawer
//                (FileReaderDrawer) - with the source model's switch
//                off, on the object's Files tab; a drawer holding a node is
//                the page's own.
//
// `tab` and `select` are the page's own (UIPageView): never run here. The
// page's selection rides in the event's extras, so `${selection.<key>}`
// fills the templates; a page section loads its content through the host
// services (loadSection).
//
// Chat artifacts keep the sandboxed iframe (UIPage).

import '@mantine/charts/styles.css'

import { useCallback, useEffect, useState } from 'react'
import { Alert, Box, Button, Group, Modal, Text } from '@mantine/core'

import { wireRegistries } from '@ktw/display-lib/registry-wire'
import { initI18n } from '@ktw/display-lib/ui-core/i18n/index.ts'
import {
  DrawioThumbnail, UIPageView, actionVerb, parseActionResult, registerPageNodeRenderer, resolveAction,
  type UINavigateTarget, type UIPageActionEvent, type UIPageNode,
} from '@ktw/display-lib/ui-builders/page/index.ts'
import { GraphFlow } from './GraphFlow'
import { UIDrawio } from './UIDrawio'
import { GuiHostServices } from './hostServices'
import { actionOutcome, outcomeAfterRead, type ActionOutcome } from './actionOutcome'
import { confirmTextsFor, type ConfirmText } from './pageConfirm'
import { useT } from '../../i18n'
import { WorkspaceRunAction } from '../../../wailsjs/go/main/App'
import { usePersonId } from '../../store/profileStore'
import { useAppStore } from '../../store'
import { useTabsStore } from '../../store/tabsSlice'
import { useChatLauncher } from '../../hooks/useChatLauncher'
import { openTaskForm } from '../../hooks/taskForm'
import { openObjectTab } from '../../sections/workspace/openObjectTab'
import { openPanelTab } from '../../sections/panels/openPanelTab'
import { openInboxTab } from '../../hooks/useInboxLauncher'
import { switchServedProject } from '../../sections/univers/projectMount'
import { ObjectCreateModal } from '../../sections/workspace/ObjectCreateModal'
import { objectRef, rowProject } from '../../sections/workspace/objectRef'
import { fileDrawer } from './fileDrawer'

/** The drawer showing an object's declared panel: registered by the workspace
 *  (RenderDefView imports this module, so it cannot be imported here). */
export type PanelDrawerView = (p: { universeName: string; object: string; panel: string; title?: string; onClose: () => void }) => JSX.Element
let panelDrawer: PanelDrawerView | undefined
export function setPanelDrawer(view: PanelDrawerView): void { panelDrawer = view }

/** The drawer reading a file of an object in place:
 *  registered by the workspace, like the panel drawer. */
export type FileReaderDrawerView = (p: { universeName: string; object: string; file: string; title: string; onClose: () => void }) => JSX.Element
let fileReaderDrawer: FileReaderDrawerView | undefined
export function setFileReaderDrawer(view: FileReaderDrawerView): void { fileReaderDrawer = view }

let wired = false
function ensureWired(): void {
  if (wired) return
  void initI18n()
  wireRegistries()
  // The GUI draws a page's graph with @xyflow/react (the Teams tab's library).
  registerPageNodeRenderer('graph', GraphFlow)
  // And a drawio node through the ktw-display viewer every draw.io document
  // uses: the viewer's script is served there (/widget/vendor/drawio/), not
  // by the GUI.
  registerPageNodeRenderer('drawio', PageDrawio)
  wired = true
}

/** PageDrawio draws a page's drawio node: its XML in the draw.io viewer, at
 *  least its height. Its root is the shared iframe widget, which takes no
 *  extra attributes: the node's data-* keys are not placed here. A node drawn
 *  as a thumbnail is display-lib's (a sketch in the page's colours, never the
 *  viewer's white sheet nor its splash), its full view this
 *  viewer. */
function PageDrawio({ node }: { node: UIPageNode }) {
  if (node.kind !== 'drawio') return null
  if (node.thumbnail) return <DrawioThumbnail node={node} full={drawioFull} />
  const value = { xml: node.xml }
  return <UIDrawio value={value} pushValue={value} initialHeight={node.height ?? 720} />
}

function drawioFull(node: Extract<UIPageNode, { kind: 'drawio' }>) {
  const value = { xml: node.xml }
  return <UIDrawio value={value} pushValue={value} initialHeight={node.height ?? 600} />
}

/** The task a chat verb names: a bare name, or the last segment of a
 *  `<provider>:tasks/<name>` URI. */
function chatTaskName(task: string): string {
  const i = task.indexOf(':tasks/')
  return i >= 0 ? task.slice(i + ':tasks/'.length) : task
}

/** openRuns shows Global → Runs, where a started run is followed. */
function openRuns(universeName: string): void {
  const tabs = useTabsStore.getState()
  const home = tabs.getForUniverse(universeName).openTabs.find((t) => t.id === 'home')
  tabs.updateTabPayload(universeName, 'home', { ...((home?.payload ?? {}) as object), activeKind: 'runs' })
  tabs.setActive(universeName, 'home')
  useAppStore.getState().setMainPaneRoute({ kind: 'universe' })
}

export interface PageHostProps {
  node: UIPageNode
  data?: unknown
  universeName: string
  /** The panel's scope, read by `${name}` templates: `id` (the object) or
   *  `target` (a global panel's), and anything else the panel knows. */
  scope?: Record<string, unknown>
  /** The panel re-reads its data (after an action ran). */
  onRefresh?: () => void
  /** The panel re-reads its source with these args over its input (a query
   *  action); absent where there is no source to re-read. */
  onQuery?: (args: Record<string, unknown>) => void
  /** The page is drawn in a dialog of the host (the create dialog): told
   *  when an action's answer opened an object (the dialog closes on it, the
   *  object opened) and when a `close` action asks it to close (« Annuler »). */
  onDone?: (opened: boolean) => void
}

export function PageHost({ node, data, universeName, scope, onRefresh, onQuery, onDone }: PageHostProps) {
  ensureWired()
  // `${person}`: who acts from the GUI — the person at the screen.
  const personId = usePersonId()
  const { launchTaskChat } = useChatLauncher()
  const [asking, setAsking] = useState<UIPageActionEvent | null>(null)
  // The confirmation dialog speaks the page's language on every button.
  const t = useT('workspace')
  const confirm = confirmTextsFor(asking?.action)
  const say = (c: ConfirmText) => ('key' in c ? t(c.key) : c.text)
  const [running, setRunning] = useState(false)
  const [outcome, setOutcome] = useState<ActionOutcome | null>(null)
  // The alert outlives the action's own re-read and goes at the next one.
  useEffect(() => { setOutcome(outcomeAfterRead) }, [data])
  // What the action owes the panel: one re-read, when the panel can re-read.
  const ownReads = onRefresh ? 1 : 0
  const [creating, setCreating] = useState<{ type?: string; parent: string } | null>(null)
  const [drawer, setDrawer] = useState<{ object: string; panel: string; title?: string } | null>(null)
  const DrawerView = panelDrawer
  const [reader, setReader] = useState<{ object: string; file: string; title: string } | null>(null)
  const ReaderView = fileReaderDrawer

  const navigate = useCallback((t: UINavigateTarget) => {
    openObjectTab(universeName, { id: t.object, ...(t.panel ? { panel: t.panel } : {}), ...(t.query ? { query: t.query } : {}), ...(t.element ? { element: t.element } : {}) })
  }, [universeName])

  const run = useCallback(async (event: UIPageActionEvent) => {
    const person = { id: personId, kind: 'person' }
    const action = resolveAction(event.action, event.item, { person, ...(scope ?? {}), ...(event.extra ?? {}) })
    const here = typeof scope?.id === 'string' ? scope.id : ''
    // An object a row names, on the row's project when it names one (a
    // universe serving several): sent by its qualified id.
    const inRow = (id: string) => objectRef(id, rowProject(event.item))
    setOutcome(null)
    switch (actionVerb(action)) {
      case 'navigate':
        if (action.navigate?.object) navigate({ ...action.navigate, object: inRow(action.navigate.object) })
        return
      case 'chat':
        if (action.chat?.task) {
          const task = action.chat.task
          const target = (action.chat.target ? inRow(action.chat.target) : here) || undefined
          // A task with a form asks for it first (taskForm).
          void openTaskForm(universeName, task, target).then((opened) => {
            if (!opened) void launchTaskChat(chatTaskName(task), { targetId: target })
          })
        }
        return
      case 'create':
        setCreating({ type: action.create?.type || undefined, parent: action.create?.parent ? inRow(action.create.parent) : here })
        return
      case 'query':
        onQuery?.(action.query?.args ?? {})
        return
      case 'drawer':
        if (action.drawer?.file) {
          const open = fileDrawer({ ...action.drawer, object: action.drawer.object ? inRow(action.drawer.object) : undefined }, here, true)
          if (open?.kind === 'reader') setReader({ object: open.object, file: open.file, title: open.title })
          else if (open) navigate({ object: open.object, panel: 'files', query: { file: open.file } })
          return
        }
        if (action.drawer?.panel && (action.drawer.object || here)) {
          setDrawer({ object: action.drawer.object ? inRow(action.drawer.object) : here, panel: action.drawer.panel, title: action.drawer.title })
        }
        return
      case 'panel':
        // A display panel in a tab of its own (a conversation
        // of « À faire » opens its chat).
        if (action.panel) openPanelTab(universeName, action.panel)
        return
      case 'close':
        // What the page is drawn in closes; a page filling a pane ignores it.
        onDone?.(false)
        return
      case 'screen':
        // One of the host's own screens: « À faire », what the rail's To do
        // opens (the deck's « Ouvrir À faire »).
        if (action.screen === 'todo') openInboxTab(universeName)
        return
      case 'uri':
        break
      // A tab or a select action is the page's own (UIPageView runs it): the
      // host never receives one, and ignores one if it did.
      case 'tab':
      case 'select':
      default:
        return
    }
    setRunning(true)
    try {
      const r = await WorkspaceRunAction(universeName, action.uri!, JSON.stringify(action.args ?? {}))
      const label = action.label ?? action.id ?? 'Done'
      if (r.error) {
        setOutcome(actionOutcome(false, r.error, ownReads))
      } else if (r.taskId) {
        // An async action is a run: it is followed where the runs are.
        setOutcome(actionOutcome(true, `${label}: started (${r.taskId})`, ownReads))
        openRuns(universeName)
      } else {
        const res = parseActionResult(r.value)
        if (res) {
          setOutcome(actionOutcome(res.ok, res.message ?? (res.ok ? `${label}: done` : `${label}: refused`), ownReads, res.undo))
          if (res.ok && res.open) { navigate(res.open); onDone?.(true) }
          // « Ouvrir un dossier »: the universe serves the
          // project the method names - the hub switches it, the universe
          // restarts on it.
          if (res.ok && res.project) await switchServedProject(universeName, res.project)
        } else {
          setOutcome(actionOutcome(true, `${label}: done`, ownReads))
        }
      }
    } catch (e) {
      setOutcome(actionOutcome(false, String(e), ownReads))
    } finally {
      setRunning(false)
      onRefresh?.()
    }
  }, [universeName, scope, onRefresh, onQuery, onDone, ownReads, personId, navigate, launchTaskChat])

  const onAction = useCallback((event: UIPageActionEvent) => {
    if (running) return
    if (event.action.confirm || event.action.dangerous) setAsking(event)
    else void run(event)
  }, [run, running])

  return (
    // The page scrolls in its own box: a declared panel fills its pane.
    <Box p="md" style={{ height: '100%', overflowY: 'auto' }}>
      {outcome && (
        <Alert mb="sm" color={outcome.ok ? 'teal' : 'red'} variant="light" withCloseButton onClose={() => setOutcome(null)}>
          <Group gap="sm" wrap="nowrap" justify="space-between">
            <Text size="xs">{outcome.text}</Text>
            {/* A result carrying its inverse offers it: run
                at once, no confirmation - it only takes the change back. */}
            {outcome.undo && (
              <Button size="compact-xs" variant="default" disabled={running} data-guide="page.undo"
                onClick={() => { const undo = outcome.undo!; setOutcome(null); void run({ action: undo }) }}>
                {t('pageUndo')}
              </Button>
            )}
          </Group>
        </Alert>
      )}
      {/* A reference field searches the objects and a reference opens one
          through the GUI's host services. */}
      <GuiHostServices universeName={universeName}>
        <UIPageView node={node} data={data} onAction={onAction} />
      </GuiHostServices>
      <Modal opened={!!asking} onClose={() => setAsking(null)} title={say(confirm.title)} centered>
        <Text size="sm">{say(confirm.body)}</Text>
        <Group justify="flex-end" mt="md">
          <Button variant="default" onClick={() => setAsking(null)}>{say(confirm.cancel)}</Button>
          <Button color={asking?.action.dangerous ? 'red' : undefined} loading={running}
            onClick={() => { const e = asking; setAsking(null); if (e) void run(e) }}>
            {say(confirm.run)}
          </Button>
        </Group>
      </Modal>
      {creating && (
        <ObjectCreateModal opened universeName={universeName} parentId={creating.parent} presetType={creating.type}
          onClose={() => { setCreating(null); onRefresh?.() }} />
      )}
      {/* The drawer view is a component of its own: rendered as an element,
          never called - its hooks are not PageHost's. */}
      {drawer && DrawerView && (
        <DrawerView universeName={universeName} object={drawer.object} panel={drawer.panel} title={drawer.title}
          onClose={() => { setDrawer(null); onRefresh?.() }} />
      )}
      {reader && ReaderView && (
        <ReaderView universeName={universeName} object={reader.object} file={reader.file} title={reader.title}
          onClose={() => setReader(null)} />
      )}
    </Box>
  )
}
