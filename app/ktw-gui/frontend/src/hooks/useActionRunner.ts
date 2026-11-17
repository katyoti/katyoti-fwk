// SPDX-License-Identifier: Apache-2.0
/**
 * useActionRunner — THE one way the GUI runs an action (ms-action-system:
 * gui-run-action-unified). Every surface (node bar, item strip, item-collection
 * header, What's-Next) routes here so there is a single, consistent run path
 * instead of the five divergent ones (WorkspaceRunAction · launchTaskChat ·
 * RunTaskInUniverse×2 · WorkspaceMethodAct).
 *
 * Routing is by the ACTION's own kind / session mode, NOT by which caller it is:
 *  - kind `ai` (session mode `chat`) → the chat launcher (interactive tab).
 *  - else → headless `WorkspaceRunAction`, with the target context
 *    (`node_id` / `item_id`) folded into the payload; returns the value/taskId.
 *
 * The target carries the scope ids. As the unified backend RunAction lands
 * (authoritative session handle), only this hook changes — every surface
 * inherits the fix.
 */

import { useCallback } from 'react'
import { WorkspaceRunAction } from '../../wailsjs/go/main/App'
import type { main } from '../../wailsjs/go/models'
import { useChatLauncher } from './useChatLauncher'

/** The target context an action runs against. */
export interface ActionTarget {
  universeName: string
  nodeId?:      string
  itemId?:      string
  itemType?:    string
}

export interface ActionRunResult {
  ok:      boolean
  message: string
}

/** Extract the task name from a `<provider>:tasks/<name>` action URI. */
function taskNameFromToolURI(uri: string): string | null {
  const colon = uri.indexOf(':')
  if (colon < 0) return null
  const rest = uri.slice(colon + 1)
  if (!rest.startsWith('tasks/')) return null
  const name = rest.slice('tasks/'.length).trim()
  return name.length > 0 ? name : null
}

export function useActionRunner() {
  const { launchTaskChat } = useChatLauncher()

  const runAction = useCallback(
    async (action: main.WorkspaceQuickAction, target: ActionTarget): Promise<ActionRunResult> => {
      if (action.disabled) {
        return { ok: false, message: action.disabledReason || 'Action unavailable' }
      }
      // Interactive (chat) actions → open a chat scoped to the target node.
      if (action.kind === 'ai') {
        const taskName = taskNameFromToolURI(action.uri)
        if (taskName) {
          void launchTaskChat(taskName)
          return { ok: true, message: 'Opened chat' }
        }
      }
      // Headless — one payload shape carrying the target scope ids.
      try {
        const payload = JSON.stringify({
          ...(target.nodeId ? { node_id: target.nodeId } : {}),
          ...(target.itemId ? { item_id: target.itemId } : {}),
          ...(target.itemType ? { item_type: target.itemType } : {}),
          ...(action.boundInput ?? {}),
        })
        const r = await WorkspaceRunAction(target.universeName, action.uri, payload)
        if (r.error) return { ok: false, message: r.error }
        return { ok: true, message: r.taskId ? `Started (${r.taskId})` : (r.value || 'Done') }
      } catch (e) {
        return { ok: false, message: String(e) }
      }
    },
    [launchTaskChat],
  )

  return { runAction }
}
