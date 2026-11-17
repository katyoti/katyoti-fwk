// SPDX-License-Identifier: Apache-2.0
/**
 * Shared node types for the Team task-tree (React Flow custom node). These are
 * the only structural types the live source + AgentNode renderer depend on.
 */

export type AgentStatus = 'idle' | 'running' | 'done' | 'error' | 'waiting'

export interface AgentNodeData {
  label: string
  role: string            // 'coordinator' | 'team-lead' | 'worker'
  kind: string            // 'agent' | 'task' | 'workflow' | 'simulator' | 'ledger'
  mode: string            // resolved SPAWN mode (worker / background / fork / coordinator / …)
  taskName: string
  status: AgentStatus
  capability: string      // 'smart-best', 'fast-best', etc.
  turns: number
  summary: string
  toolCount: number
  promptCount: number
  sessionId: string
}
