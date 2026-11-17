// SPDX-License-Identifier: Apache-2.0
/**
 * MainArea — the right-pane router for ktw-gui.
 *
 * 4 derived modes from store state (no viewMode field):
 *   - runningTaskId !== null  → TaskMode (ChatPanel)
 *   - pendingTask !== null    → run-task form (RunTaskPanel)
 *   - activeNodeId !== null   → NodeView (breadcrumb + scoped gallery)
 *   - else                    → ProjectHome (project landing CTA)
 */
import { useAppStore } from '../../store'
import { ChatPanel }     from '../agent/ChatPanel'
import { RunTaskPanel }  from '../agent/RunTaskPanel'
import { NodeView }      from './NodeView'
import { ProjectHome }   from './ProjectHome'

export function MainArea() {
  const runningTaskId = useAppStore((s) => s.runningTaskId)
  const pendingTask   = useAppStore((s) => s.pendingTask)
  const activeNodeId  = useAppStore((s) => s.activeNodeId)

  if (runningTaskId !== null) return <ChatPanel />
  if (pendingTask !== null)   return <RunTaskPanel />
  if (activeNodeId !== null)  return <NodeView />
  return <ProjectHome />
}
