// SPDX-License-Identifier: Apache-2.0
/**
 * ActionsPanel — an object's actions as a searchable card grid: click runs
 * the action through ActionService (WorkspaceRunAction), an agent task opens
 * as a chat, an action whose input needs values opens its launch dialog.
 */
import { useEffect, useState } from 'react'
import {
  ActionIcon, Alert, Anchor, Badge, Box, Button, Card, Center, Divider, Group, Loader, Modal,
  ScrollArea, SimpleGrid, Stack, Text, Title, Tooltip,
} from '@mantine/core'
import {
  IconAlertCircle, IconBolt, IconEye, IconInfoCircle, IconLayoutGrid, IconList, IconMessage, IconPencil,
  IconRefresh, IconTable, IconTrash,
} from '@tabler/icons-react'
import { tablerIcon } from '@ktw/display-lib/ui-core/components/icons/index.ts'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'
import { useChatLauncher } from '../../hooks/useChatLauncher'
import { formInitial } from '../../hooks/formInitial'
import { WorkspaceActionTask, WorkspaceDescribeAction, WorkspaceRunAction } from '../../../wailsjs/go/main/App'
import { ActionLaunchModal, needsLaunchDialog } from './ActionLaunchModal'
import type { main } from '../../../wailsjs/go/models'
import { SearchableCardGrid, type CardListItem } from './SearchableCardGrid'

interface ActionsPanelProps {
  actions: main.WorkspaceQuickAction[]   // WorkspaceAction === WorkspaceQuickAction (type alias)
  universeName: string
  nodeId: string
  /** `buttons`: a row of small buttons (an object's quick actions on its first
   *  page) - the same dispatch as the grid. */
  layout?: 'grid' | 'buttons'
}

/**
 * ActionsPanel — thin adapter mapping the Node tab's substrate
 * actions (WorkspaceQuickAction[] from cwm.node.Detail) onto the
 * generic SearchableCardGrid items grid. cwm-v2-providers US-15
 * (2026-06-08): click → ActionService.Run via WorkspaceRunAction
 * Wails binding. The dispatcher (`internal/extensions/actions/`)
 * routes by URI slot prefix (tasks/ → TaskService; scripts/ →
 * ScriptService). Async tasks return a task ID for follow-up; sync
 * actions return their value inline.
 *
 * v1 sends `{node_id}` as the action input — enough for headless
 * tasks like `seed-default-children`. Schema-driven launch forms
 * (drive input from the action's input_schema) are a US-S4 / D1
 * follow-up.
 */
// taskNameFromToolURI extracts the task name from a tool/action URI of
// the form `<provider>:tasks/<name>` (e.g. ktw-compose:tasks/compose-artifact
// → "compose-artifact"). Returns null for non-tasks URIs (scripts/,
// mutations/, KTSP methods, or the resource form system:/tasks/chat which
// the quick-action path already handles). Used to route agent-task actions
// to an interactive chat tab instead of fire-and-forget ActionService.Run.
function taskNameFromToolURI(uri: string): string | null {
  const colon = uri.indexOf(':')
  if (colon < 0) return null
  const rest = uri.slice(colon + 1)
  // tool/action form is `tasks/<name>`; the resource form `/tasks/...`
  // (leading slash) is NOT a launchable task action here.
  if (!rest.startsWith('tasks/')) return null
  const name = rest.slice('tasks/'.length).trim()
  return name.length > 0 ? name : null
}

// EXPORTED so the OBJECT pane renders the SAME action grid (2026-09-25). One
// grid, two models: a second copy would drift on the confirm gate, the preview
// modal, the chat routing and the describe-then-dispatch rule — all of which
// live here and none of which are about nodes.
export function ActionsPanel({ actions, universeName, nodeId, layout = 'grid' }: ActionsPanelProps) {
  const t = useT('workspace')
  const { launchTaskChat } = useChatLauncher()
  const openRunTask = useAppStore((s) => s.openRunTask)
  // The create-task display: a create task's form opens with its parent - the object,
  // or the project's root for a global action - filled.
  // Last-action result banner state. cwm-v2-providers US-15
  // (2026-06-08): no Mantine notifications package installed in
  // ktw-gui, so we use an inline Alert above the cards. Auto-clears
  // after 6s on success; sticks until next click on failure.
  const [lastResult, setLastResult] = useState<
    | { kind: 'ok-async'; uri: string; taskId: string }
    | { kind: 'ok-sync'; uri: string; value: string }
    | { kind: 'error'; uri: string; message: string }
    | null
  >(null)
  useEffect(() => {
    if (lastResult && lastResult.kind !== 'error') {
      const t = window.setTimeout(() => setLastResult(null), 6000)
      return () => window.clearTimeout(t)
    }
    return undefined
  }, [lastResult])

  // Launch-modal target (confirm gate and/or parameter form). Null =
  // no modal open.
  const [launchTarget, setLaunchTarget] = useState<main.WorkspaceQuickAction | null>(null)
  // Coming-soon modal target for a preview (future-surface) action. Null =
  // no modal open. Preview actions never dispatch (sdd10).
  const [previewTarget, setPreviewTarget] = useState<main.WorkspaceQuickAction | null>(null)

  // Project substrate actions onto CardListItem; `source` becomes a
  // tag so it's filterable from the same MultiSelect that handles
  // real hashtags once tasks ship them. Disabled bindings (their
  // when-predicate is false for this node) render muted with the
  // reason in place of the URI line. Card id = binding identity
  // (uri + label) — one URI can be bound several times with
  // different labels + pre-bound inputs.
  const bindingId = (a: main.WorkspaceQuickAction) => `${a.uri}::${a.label}`
  const itemList: CardListItem[] = actions.map((a) => ({
    id:          bindingId(a),
    title:       a.label || a.uri,
    description: a.disabled
      ? (a.disabledReason || t('actionsPanel.unavailable'))
      : (a.description || a.uri),
    hashtags:    a.source ? [a.source] : [],
    iconName:    a.icon || 'bolt',
    // Preview (future-surface) actions wear a grape accent + a "coming soon"
    // badge so they read as honestly distinct from the real ones (sdd10).
    accent:      a.preview ? 'grape' : a.disabled ? 'gray' : 'blue',
    // Badge priority: a preview marker wins; otherwise a `kind: ai` action
    // gets a "Chat" badge so a conversational action reads differently from
    // a silent sync/async run (P1 task-vs-chat signal).
    badge:       a.preview
      ? { label: t('actionsPanel.preview'), color: 'grape' }
      : a.kind === 'ai'
        ? { label: 'Chat', color: 'violet' }
        : undefined,
    disabled:    a.disabled,
  }))

  const applyResult = (uri: string, result: main.WorkspaceRunActionResult) => {
    if (result.error) {
      setLastResult({ kind: 'error', uri, message: result.error })
      return
    }
    if (result.taskId) {
      setLastResult({ kind: 'ok-async', uri, taskId: result.taskId })
    } else {
      setLastResult({ kind: 'ok-sync', uri, value: result.value ?? '' })
      if (result.value) {
        // eslint-disable-next-line no-console
        console.info('[action]', uri, 'returned:', result.value)
      }
    }
  }

  const handleClick = async (item: CardListItem) => {
    const action = actions.find((a) => bindingId(a) === item.id)
    if (!action || action.disabled) return
    setLastResult(null)

    // Preview (future-surface) actions NEVER dispatch — open a coming-soon
    // panel instead. Guarantees the sdd10 anti-probe: no error, no mutation.
    if (action.preview) {
      setPreviewTarget(action)
      return
    }

    // Interactive agent actions (kind: ai) — e.g. the compose-artifact /
    // asset-intake wizards — open a CHAT TAB running the task, exactly like
    // the "Start chat" quick action. Only kind:ai routes here; sync/async
    // actions stay on the fire-and-forget ActionService.Run path below.
    // The author declares kind:ai on the node-type ActionRef.
    if (action.kind === 'ai') {
      const taskName = taskNameFromToolURI(action.uri)
      if (taskName) {
        // A task with a form asks for it first (the language of a
        // translation, a speaking time…), then opens its chat on the object.
        const task = await WorkspaceActionTask(universeName, action.uri).catch(() => null)
        if (task?.task && task.inputSchemaURI) {
          openRunTask({
            name: task.task,
            taskName,
            uri: task.task,
            title: action.label || task.title || taskName,
            description: task.description ?? '',
            inputSchemaURI: task.inputSchemaURI,
            targetId: nodeId,
            initial: formInitial(nodeId, true),
          })
          return
        }
        void launchTaskChat(taskName, {
          tabId: `chat:${taskName}:${nodeId}`,
          label: action.label || taskName,
        })
        return
      }
    }

    // Confirm-gated actions always go through the launch modal. For
    // the rest, a quick Describe decides: free schema fields → modal
    // with the auto-generated form; none → direct dispatch with the
    // node binding + pre-bound input.
    try {
      if (action.confirm) {
        setLaunchTarget(action)
        return
      }
      const desc = await WorkspaceDescribeAction(universeName, action.uri)
      if (!desc.error && needsLaunchDialog(action, desc.inputSchema ?? '')) {
        setLaunchTarget(action)
        return
      }
      const payload = JSON.stringify({ node_id: nodeId, ...(action.boundInput ?? {}) })
      const result = await WorkspaceRunAction(universeName, action.uri, payload)
      applyResult(action.uri, result)
    } catch (err) {
      setLastResult({ kind: 'error', uri: item.id, message: String(err) })
    }
  }

  return (
    <Stack gap="xs" style={{ height: '100%' }}>
      {lastResult?.kind === 'error' && (
        <Alert
          color="red"
          variant="light"
          title={t('actionsPanel.failed', { uri: lastResult.uri })}
          withCloseButton
          onClose={() => setLastResult(null)}
        >
          {lastResult.message}
        </Alert>
      )}
      {lastResult?.kind === 'ok-async' && (
        <Alert color="blue" variant="light" title={t('actionsPanel.launched', { uri: lastResult.uri })}>
          {t('actionsPanel.taskStarted', { id: lastResult.taskId })}
        </Alert>
      )}
      {lastResult?.kind === 'ok-sync' && (
        <Alert color="green" variant="light" title={t('actionsPanel.completed', { uri: lastResult.uri })}>
          {lastResult.value ? t('actionsPanel.seeConsole') : t('actionsPanel.done')}
        </Alert>
      )}
      {layout === 'buttons' ? (
        <Group gap="xs">
          {actions.map((a) => {
            const Icon = tablerIcon(a.icon, IconBolt)
            return (
              <Tooltip key={bindingId(a)} label={a.disabledReason || a.description || a.label} withArrow disabled={!a.disabledReason && !a.description}>
                <Button size="xs" variant="light" leftSection={<Icon size={14} />} disabled={a.disabled}
                  onClick={() => void handleClick({ id: bindingId(a), title: a.label })}
                  data-guide="quick-action" data-guide-name={a.uri}>
                  {a.label}
                </Button>
              </Tooltip>
            )
          })}
        </Group>
      ) : (
      <SearchableCardGrid
        items={itemList}
        onItemClick={handleClick}
        emptyDataMessage={t('actionsPanel.emptyData')}
        emptyFilteredMessage={t('actionsPanel.emptyFiltered')}
        // THE SAME POPOVER the task gallery has, paid for one card at a time.
        // The gallery already holds every body because it lists tasks; an
        // action list holds none, and reading forty to draw one panel is the
        // cost that made the gallery take four seconds.
        infoFetch={async (item) => {
          const uri = String(item.id).split('::')[0]
          const d = await WorkspaceDescribeAction(universeName, uri)
          return d.details || d.description || ''
        }}
      />
      )}
      {launchTarget && (
        <ActionLaunchModal
          action={launchTarget}
          universeName={universeName}
          nodeId={nodeId}
          onClose={() => setLaunchTarget(null)}
          onResult={(r) => applyResult(launchTarget.uri, r)}
        />
      )}
      {previewTarget && (
        <Modal
          opened
          onClose={() => setPreviewTarget(null)}
          title={previewTarget.label || previewTarget.uri}
          centered
        >
          <Stack gap="sm">
            <Badge color="grape" variant="light" style={{ textTransform: 'none', alignSelf: 'flex-start' }}>
              {t('actionsPanel.preview')}
            </Badge>
            {previewTarget.description && (
              <Text size="sm">{previewTarget.description}</Text>
            )}
            <Text size="sm" c="dimmed">
              {t('actionsPanel.previewBody')}
            </Text>
          </Stack>
        </Modal>
      )}
    </Stack>
  )
}

function formatDate(iso: string): string {
  if (!iso) return ''
  try {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return iso
    return d.toLocaleString(undefined, {
      year: 'numeric', month: 'short', day: '2-digit',
      hour: '2-digit', minute: '2-digit',
    })
  } catch {
    return iso
  }
}
