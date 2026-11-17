// SPDX-License-Identifier: Apache-2.0
import { ActionIcon, Group, Textarea } from '@mantine/core'
import {
  IconPaperclip,
  IconPlayerStop,
  IconSend,
} from '@tabler/icons-react'
import { useAppStore, useSessionStatus } from '../../store'
import { RunTaskInUniverse, StopTask } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'

interface Props {
  text: string
  onChange: (v: string) => void
  /** The session this input belongs to (per-run display). Undefined for the
   *  "start a new task" entry (no session yet → idle). */
  sessionId?: string
}

/** Bare textarea + send/stop + attachment/mic icons */
export function ChatInput({ text, onChange, sessionId }: Props) {
  const sessionStatus = useSessionStatus(sessionId)
  const startSession  = useAppStore((s) => s.startSession)
  const t = useT('chat')
  const isRunning = sessionStatus === 'running'

  // Universe-aware routing — see RunTaskPanel.tsx for the full rationale.
  const displayedName = useAppStore((s) => s.displayedUniverseName)
  const universes     = useAppStore((s) => s.universes)
  const displayedUniverse = universes.find((u) => u.name === displayedName)

  async function handleRun() {
    const task = text.trim()
    if (!task) return
    if (!displayedUniverse) {
      console.error('[ChatInput] no displayed universe — activate one in the IconRibbon first')
      useAppStore.getState().setSessionStatus('error')
      return
    }
    onChange('')
    startSession(task)
    try {
      // Always route via the displayed universe's core. If busRunning is
      // false (transient state mismatch), the backend will Activate
      // idempotently or surface a clear error — better than the legacy
      // RunTask fallback which spawns a fresh subprocess with the wrong
      // KTW_HOME and silently produces no events.
      await RunTaskInUniverse(displayedUniverse.name, task, '')
    } catch (err) {
      console.error('[ChatInput] RunTaskInUniverse failed:', err)
      useAppStore.getState().setSessionStatus('error')
    }
  }

  async function handleStop() {
    await StopTask(sessionId ?? useAppStore.getState().runningSessionId ?? '')
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Enter' && !e.shiftKey && !isRunning) {
      e.preventDefault()
      handleRun()
    }
  }

  return (
    <Group gap="xs" align="flex-end">
      <ActionIcon variant="subtle" color="gray" size="md" title={t('input.attachFile')}>
        <IconPaperclip size={16} />
      </ActionIcon>

      <Textarea
        style={{ flex: 1 }}
        placeholder={isRunning ? t('input.runningPlaceholder') : t('input.idlePlaceholder')}
        value={text}
        onChange={(e) => onChange(e.currentTarget.value)}
        onKeyDown={handleKeyDown}
        minRows={1}
        maxRows={6}
        autosize
        disabled={isRunning}
        styles={{
          input: {
            background: 'var(--mantine-color-default)',
            border: '1px solid var(--mantine-color-default-border)',
            resize: 'none',
            fontSize: 13,
          },
        }}
      />

      {/* Voice input is not built: no microphone button until it is. */}

      {isRunning ? (
        <ActionIcon variant="light" color="red" size="md" onClick={handleStop} title={t('input.stop')}>
          <IconPlayerStop size={16} />
        </ActionIcon>
      ) : (
        <ActionIcon
          variant="filled"
          color="blue"
          size="md"
          onClick={handleRun}
          disabled={!text.trim()}
          title={t('input.send')}
        >
          <IconSend size={16} />
        </ActionIcon>
      )}
    </Group>
  )
}
