// SPDX-License-Identifier: Apache-2.0
/**
 * useChatLauncher — single source of truth for "open the chat".
 *
 * The chat task (`system:/tasks/chat`) is the workspace's default
 * agent conversation. It has the `skill-enabled` plugin group + the
 * `cwm` plugin already bound (see TASK.md frontmatter), so the agent
 * starts with file/bash/skill tools, the 26 cwm-* tools, and — when
 * `_target` is set in the plugin context — the
 * `<cwm_object_context>` system-prompt block.
 *
 * Two callers today:
 *   1. The chat icon in [IconRibbon.tsx] (between Debug and Camera)
 *   2. The Ctrl+I global hotkey registered in App.tsx
 *
 * Both surfaces use the same `launchChat` callback so the disabled
 * gating (no active universe = no-op) lives in one place.
 *
 * Phase 3 of US-3 will extend the helper to thread `activeNodeId`
 * down to the backend so the cwm plugin's currentNodePlugin wrapper
 * picks up `_target` automatically.
 */

import { useCallback } from 'react'
import { useAppStore } from '../store'
import { useTabsStore, type ChatTabPayload } from '../store/tabsSlice'
import { RunTaskInUniverse, RunTaskInUniverseWithContext } from '../../wailsjs/go/main/App'

const CHAT_TASK_NAME = 'chat'
const CHAT_TAB_ID    = 'chat'

/** Options for launching a non-default task as an interactive chat tab. */
export interface LaunchTaskChatOptions {
  /** Tab id — defaults to `chat:<taskName>`. Re-launch focuses it. */
  tabId?: string
  /** Tab label — defaults to a Title-Cased task name. */
  label?: string
  /**
   * Explicit `_target` for this launch — overrides the store's
   * activeNodeId. Use when the action targets a SPECIFIC node that isn't the
   * one whose tab is open (e.g. a per-feature "Break down" button binds the
   * clicked feature node, not its parent module). Falls back to activeNodeId.
   */
  targetId?: string
}

function titleCase(name: string): string {
  return name
    .split(/[-_]/)
    .map((w) => (w ? w[0].toUpperCase() + w.slice(1) : w))
    .join(' ')
}

export function useChatLauncher() {
  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)
  const activeNodeId          = useAppStore((s) => s.activeNodeId)
  const startSession          = useAppStore((s) => s.startSession)
  const openTab               = useTabsStore((s) => s.openTab)

  // launchTaskChat — open (or focus) an interactive chat tab that runs
  // the named task with the active node as context. The default chat is
  // just `launchTaskChat('chat', {tabId:'chat', label:'Chat'})`; node
  // actions that target a mode:chat task (e.g. compose-artifact,
  // asset-intake) route here so they open a live chat tab like the chat
  // icon does — instead of fire-and-forget ActionService.Run.
  const launchTaskChat = useCallback(
    async (taskName: string, opts: LaunchTaskChatOptions = {}) => {
      if (!displayedUniverseName) return
      // Explicit target wins over the store's active node (per-item actions
      // target a specific node, not the panel's host).
      const target = opts.targetId || activeNodeId
      // NOTE: the chat tab is NO LONGER opened eagerly here. The
      // `session.created` event (useAgentEvents) opens + focuses this run's
      // OWN tab `chat:<sessionId>` once the server issues the journal
      // session id — that is what lets parallel chats each get a distinct
      // tab instead of colliding on a fixed `chat` id. We still call
      // startSession so the global task-state lights up immediately.
      startSession(taskName)
      try {
        if (target) {
          // Inject `_target` so the cwm plugin prepends the
          // `<cwm_object_context>` block — the wizard lands on THIS node.
          await RunTaskInUniverseWithContext(
            displayedUniverseName,
            taskName,
            '',
            { _target: target },
          )
        } else {
          await RunTaskInUniverse(displayedUniverseName, taskName, '')
        }
      } catch (err) {
        console.error('[useChatLauncher] start failed:', err)
      }
    },
    [displayedUniverseName, activeNodeId, startSession, openTab],
  )

  const launchChat = useCallback(
    () => launchTaskChat(CHAT_TASK_NAME, { tabId: CHAT_TAB_ID, label: 'Chat' }),
    [launchTaskChat],
  )

  return {
    launchChat,
    launchTaskChat,
    canLaunch: !!displayedUniverseName,
  }
}
