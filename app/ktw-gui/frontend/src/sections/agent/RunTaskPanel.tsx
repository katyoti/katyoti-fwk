// SPDX-License-Identifier: Apache-2.0
/**
 * RunTaskPanel — task launch form. Mounted by MainArea when
 * `pendingTask !== null && runningTaskId === null` (the user clicked a
 * task card in NodeView but hasn't hit Run yet). Provides a single text
 * input for the user's initial message and Run/Cancel buttons.
 *
 * Run → calls App.RunTaskWithInput with the captured runMode/connectURL;
 * startSession() sets runningTaskId, MainArea swaps to ChatPanel.
 * Cancel → closeRunTask() clears pendingTask; MainArea returns to NodeView.
 *
 * Keyboard: Ctrl+Enter submits Run; Escape cancels. Plain Enter inserts a
 * newline in the textarea (multi-line input is fine).
 *
 * A task tagged `no-initial-input` asks its questions itself, in its chat:
 * it opens no box and starts at once (runOpening).
 */
import { useEffect, useRef, useState } from 'react'
import {
  ActionIcon,
  Button,
  Group,
  Paper,
  Stack,
  Text,
  Textarea,
  Title,
} from '@mantine/core'
import { IconPlayerPlay, IconX } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'
import { RunTaskInUniverse, RunTaskInUniverseWithContext } from '../../../wailsjs/go/main/App'
import { FormWidgetPanel } from './FormWidgetPanel'
import { TaskIdentity } from './TaskIdentity'
import { runOpening } from '../../hooks/taskInitialInput'
import { useFormRun } from '../../hooks/useFormRun'

export function RunTaskPanel() {
  const t = useT('chat')
  const pendingTask = useAppStore((s) => s.pendingTask)
  const closeRunTask = useAppStore((s) => s.closeRunTask)
  const startSession = useAppStore((s) => s.startSession)
  // Universe-aware routing: when a universe is displayed and active,
  // run via its long-lived core (KTSP Terminal service). Otherwise fall
  // back to the legacy per-task subprocess path.
  const displayedName = useAppStore((s) => s.displayedUniverseName)
  const universes     = useAppStore((s) => s.universes)
  const displayedUniverse = universes.find((u) => u.name === displayedName)

  const [input, setInput] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const formRun = useFormRun(pendingTask, displayedUniverse?.name)

  // Auto-focus the textarea when the form opens.
  useEffect(() => {
    textareaRef.current?.focus()
  }, [])

  // A task that takes no initial message starts at once: its questions are
  // asked in its chat. Once per pending task.
  const opening = runOpening(pendingTask)
  const startedFor = useRef<typeof pendingTask>(null)
  useEffect(() => {
    if (opening !== 'start' || !pendingTask || !displayedUniverse) return
    if (startedFor.current === pendingTask) return
    startedFor.current = pendingTask
    setSubmitting(true)
    const run = pendingTask.targetId
      ? RunTaskInUniverseWithContext(displayedUniverse.name, pendingTask.name, '', { _target: pendingTask.targetId })
      : RunTaskInUniverse(displayedUniverse.name, pendingTask.name, '')
    run.then(() => startSession(pendingTask.name)).catch((err) => {
      console.error('[RunTaskPanel/start] RunTaskInUniverse failed:', err)
      setSubmitting(false)
    })
  }, [opening, pendingTask, displayedUniverse, startSession])

  // ── Schema-driven branch (ms-schema-driven-ui US-S1 Phase 1) ────────
  // When the pending task ships a canonical input-schema, render the
  // FormWidgetPanel — the universal task-run surface that auto-generates
  // the form from the schema's ui-form extension. The legacy free-form
  // textarea below stays as the fallback for tasks without a schema
  // (every existing demo task except hello-world today).
  //
  // Submit converts the form's structured value to a JSON string and
  // hands it to the existing RunTaskInUniverse pipeline. The receiving
  // task's body documents that `input` arrives as JSON so it can parse
  // structured fields. A future iteration adds a structured-input task
  // run path (TaskRunRequest.Input map) so the JSON-string detour
  // disappears.
  if (pendingTask?.inputSchemaURI && displayedUniverse) {
    return (
      <FormWidgetPanel
        universeName={displayedUniverse.name}
        schemaURI={pendingTask.inputSchemaURI}
        header={{
          title:       pendingTask.title || pendingTask.taskName || pendingTask.name,
          description: pendingTask.description,
          iconName:    pendingTask.iconName,
          provider:    pendingTask.provider,
          name:        pendingTask.taskName,
          uri:         pendingTask.uri,
        }}
        // Task launcher policy: replace the lib's default
        // (Save/Cancel/Reset/Delete) with just [Cancel, Run]. The
        // policy lives HERE (the caller) — FormWidgetPanel itself is
        // policy-free now and any other caller (e.g. the Type Browser
        // preview) gets a different action set.
        actionMode="replace"
        actionRefs={[
          { type: 'ui-action', id: 'cancel', scope: 'item', label: t('form.cancel'), icon: 'IconX' },
          { type: 'ui-action', id: 'submit', scope: 'item', label: t('form.run'),    icon: 'IconPlayerPlay' },
        ]}
        submitting={formRun.submitting}
        serverViolations={formRun.violations}
        initial={pendingTask.initial}
        onCancel={closeRunTask}
        onSubmit={formRun.submit}
      />
    )
  }

  // Defensive: nothing to render without a pending task. Shouldn't happen
  // because MainArea only mounts this when pendingTask !== null, but we
  // render an empty shell rather than throw.
  if (!pendingTask) {
    return (
      <Stack p="md">
        <Text size="sm" c="dimmed">{t('runTask.noTask')}</Text>
      </Stack>
    )
  }

  // Starting at once: the task's identity while its chat opens - no box.
  if (opening === 'start') {
    return (
      <Stack p="md" gap="md" data-guide="task.starting">
        <Title order={3}>{pendingTask.title || pendingTask.taskName || pendingTask.name}</Title>
        <TaskIdentity provider={pendingTask.provider} name={pendingTask.taskName} uri={pendingTask.uri} />
      </Stack>
    )
  }

  // A new task always opens a FRESH session — never gate it on another
  // (focused/global) session's status. Parallel chats must be launchable
  // while other chats are still running.
  const canSubmit = !submitting

  const handleRun = async () => {
    if (!canSubmit) return
    if (!displayedUniverse) {
      console.error('[RunTaskPanel] no displayed universe — activate one first')
      setSubmitting(false)
      return
    }
    setSubmitting(true)
    try {
      // Always route via the displayed universe's core. The backend's
      // runViaUniverse calls Master.Activate idempotently — so a transient
      // busRunning=false in the frontend doesn't matter as long as the
      // universe exists. The previous fallback to RunTaskWithInput
      // (legacy stdio subprocess) silently produced no events whenever
      // the universe state had drifted.
      await RunTaskInUniverse(displayedUniverse.name, pendingTask.name, input)
      startSession(pendingTask.name)
    } catch (err) {
      console.error('[RunTaskPanel] RunTaskInUniverse failed:', err)
      // Leave the form open so the user can see they're still in the launcher.
      setSubmitting(false)
    }
  }

  const handleCancel = () => {
    closeRunTask()
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault()
      handleRun()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      handleCancel()
    }
  }

  return (
    <Stack p="md" gap="md" style={{ flex: 1, height: '100%' }}>
      <Group justify="space-between" align="flex-start">
        <Stack gap={4}>
          <Title order={3}>{pendingTask.title || pendingTask.taskName || pendingTask.name}</Title>
          {pendingTask.description && (
            <Text size="sm" c="dimmed">{pendingTask.description}</Text>
          )}
          <TaskIdentity
            provider={pendingTask.provider}
            name={pendingTask.taskName}
            uri={pendingTask.uri}
          />
        </Stack>
        <ActionIcon variant="subtle" color="gray" onClick={handleCancel} title={t('runTask.cancelEsc')}>
          <IconX size={16} />
        </ActionIcon>
      </Group>

      <Paper p="md" radius="sm" withBorder bg="dark.7" style={{ flex: 1 }}>
        <Stack gap="sm" style={{ height: '100%' }}>
          <Text size="sm" fw={500}>{t('runTask.inputLabel')}</Text>
          <Textarea
            ref={textareaRef}
            value={input}
            onChange={(e) => setInput(e.currentTarget.value)}
            onKeyDown={handleKeyDown}
            placeholder={t('runTask.inputPlaceholder')}
            autosize
            minRows={6}
            maxRows={20}
            disabled={submitting}
            styles={{ input: { fontFamily: 'monospace', fontSize: 13 } }}
          />
          <Text size="xs" c="dimmed">
            {t('runTask.tip')}
          </Text>
        </Stack>
      </Paper>

      <Group justify="flex-end" gap="sm">
        <Button variant="subtle" color="gray" onClick={handleCancel} disabled={submitting}>
          {t('form.cancel')}
        </Button>
        <Button
          leftSection={<IconPlayerPlay size={14} />}
          onClick={handleRun}
          disabled={!canSubmit}
          loading={submitting}
          data-guide="task.run"
        >
          {t('form.run')}
        </Button>
      </Group>
    </Stack>
  )
}

