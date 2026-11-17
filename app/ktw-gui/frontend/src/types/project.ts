// SPDX-License-Identifier: Apache-2.0
// Project model — represents a workspace/project in the sidebar.
//
// `contextTree` and `sessions` are dummy in-memory data for v1; the eventual
// CWM (ms10) integration will replace these with backend queries — same
// shape, swap-in is mechanical. See modules/cwm/concepts.md.

export type ContextNodeType =
  | 'workspace'
  | 'area'
  | 'project'
  | 'milestone'
  | 'task'
  | 'subtask'
  | 'note'

export interface ContextNode {
  id:           string
  type:         ContextNodeType
  name:         string
  description?: string
  /** Children sort key (ascending). Ties broken by `name`. */
  order:        number
  children?:    ContextNode[]
  /**
   * Tasks runnable from this node. Names are looked up in
   * `App.ListTasksDetail()` to fetch full TaskInfo for the gallery cards.
   */
  taskNames?:   string[]
}

export type SessionEntryStatus = 'completed' | 'failed' | 'running'

export interface SessionEntry {
  id:          string
  taskName:    string
  startedAt:   number          // unix ms
  status:      SessionEntryStatus
  durationMs?: number
}

export interface Project {
  id:          string
  name:        string
  color:       string          // Mantine color name
  icon:        string          // short label shown in the avatar (e.g. "KTS", "APP")
  path:        string          // workspace root path
  chats:       number          // number of active chats
  contextTree: ContextNode     // root node of the project's hierarchy
  sessions:    SessionEntry[]  // recent sessions for this project
}

/** Sidebar tabs in ProjectPanel.
 *
 * Canonical values (post-2026-05-15): 'workspace' | 'sessions' | 'terminals'.
 *
 * Legacy values ('work', 'chats', 'context') are kept here for store-
 * hydration compatibility; the migration `useEffect` in ProjectPanel maps
 * them to 'workspace' on first render. */
export type SidebarTab =
  | 'objects'
  | 'sessions'
  | 'terminals'
  // Legacy values — kept resolvable so persisted store values don't crash;
  // migrated to 'objects' on first render.
  | 'workspace'
  | 'work'
  | 'chats'
  | 'context'

// ── Tree helpers ──────────────────────────────────────────────────────────

/**
 * Walk a subtree and collect all `taskNames` from this node and ALL its
 * descendants (deduplicated, preserving first-seen order). Lets parent
 * nodes show the union of their children's tasks — clicking the workspace
 * root surfaces everything below it.
 */
export function collectAllTaskNames(node: ContextNode): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  function walk(n: ContextNode) {
    for (const name of n.taskNames ?? []) {
      if (!seen.has(name)) {
        seen.add(name)
        out.push(name)
      }
    }
    for (const child of n.children ?? []) walk(child)
  }
  walk(node)
  return out
}

// ── Dummy session helpers ────────────────────────────────────────────────

const HOUR = 3_600_000
const DAY = 24 * HOUR
const NOW = Date.now()

function s(taskName: string, ageMs: number, status: SessionEntryStatus, durationMs = 30_000): SessionEntry {
  return {
    id:        `sess-${taskName}-${ageMs}`,
    taskName,
    startedAt: NOW - ageMs,
    status,
    durationMs,
  }
}

// ── Dummy MOCK_PROJECTS ──────────────────────────────────────────────────

export const MOCK_PROJECTS: Project[] = [
  // ── p1 ktw-agents ──────────────────────────────────────────────────────
  {
    id:    'p1',
    name:  'ktw-agents',
    color: 'green',
    icon:  'KTS',
    path:  '/projects/ktw-agents',
    chats: 1,
    sessions: [
      s('task-builder-master',     30 * 60_000,  'completed', 145_000),
      s('demo-magentic-pattern',   2 * HOUR,    'completed', 92_000),
      s('demo-team-coordination',  4 * HOUR,    'failed',    18_000),
      s('hello-world',             1 * DAY,     'completed', 4_000),
      s('codebase-audit',          2 * DAY,     'completed', 380_000),
      s('document-demo',           3 * DAY,     'completed', 22_000),
    ],
    contextTree: {
      id: 'p1-root', type: 'workspace', name: 'ktw-agents',
      order: 0,
      description: 'Main agent orchestration platform',
      children: [
        {
          id: 'p1-engine', type: 'area', name: 'Engine',
          order: 0,
          description: 'Task engine, workflow primitives, agent loop',
          children: [
            {
              id: 'p1-engine-builder', type: 'project', name: 'Task Builder',
              order: 0,
              description: 'Guided creation of new ktw-agents items',
              taskNames: [
                'task-builder-master',
                'task-builder-interview',
                'task-builder-extract',
                'task-builder-planner',
                'task-builder-executor',
                'task-builder-evaluator',
                'task-builder-reviewer',
              ],
            },
            {
              id: 'p1-engine-demos', type: 'project', name: 'Demos',
              order: 1,
              description: 'Workflow demos exercising orchestration patterns',
              taskNames: [
                'demo-magentic-pattern',
                'demo-parallel-fallback',
                'demo-pipeline-agents',
                'demo-team-coordination',
                'demo-team-with-messaging',
              ],
            },
          ],
        },
        {
          id: 'p1-research', type: 'area', name: 'Research',
          order: 1,
          description: 'Research workflows and analysis tasks',
          children: [
            {
              id: 'p1-research-co', type: 'project', name: 'Company Research',
              order: 0,
              taskNames: ['company-research', 'company-info'],
            },
            {
              id: 'p1-research-audit', type: 'project', name: 'Code Audits',
              order: 1,
              taskNames: ['codebase-audit'],
            },
          ],
        },
        {
          id: 'p1-misc', type: 'area', name: 'Misc',
          order: 2,
          taskNames: ['hello-world', 'document-demo', 'display-demo', 'display-data-binding-demo', 'display-drawio-demo', 'goal-planner'],
        },
      ],
    },
  },

  // ── p2 my-app ──────────────────────────────────────────────────────────
  {
    id:    'p2',
    name:  'my-app',
    color: 'blue',
    icon:  'APP',
    path:  '/projects/my-app',
    chats: 0,
    sessions: [
      s('hello-world',      6 * HOUR,  'completed', 3_500),
      s('codebase-audit', 2 * DAY,  'completed', 410_000),
      s('document-demo',  5 * DAY,  'completed', 18_000),
    ],
    contextTree: {
      id: 'p2-root', type: 'workspace', name: 'my-app',
      order: 0,
      description: 'Demo application — frontend + backend',
      children: [
        {
          id: 'p2-frontend', type: 'area', name: 'Frontend',
          order: 0,
          description: 'React + TypeScript + Mantine',
          taskNames: ['hello-world', 'document-demo', 'display-demo', 'display-data-binding-demo', 'display-drawio-demo', 'song-writer', 'goal-planner'],
        },
        {
          id: 'p2-backend', type: 'area', name: 'Backend',
          order: 1,
          description: 'Go + Gin + PostgreSQL',
          taskNames: ['codebase-audit', 'spec-extract', 'sidecar-tester', 'plan-builder'],
        },
        {
          id: 'p2-planning', type: 'area', name: 'Planning',
          order: 2,
          description: 'Requirements + design + interviews',
          taskNames: ['requirements-analysis', 'planning-methodology', 'interview-technique', 'plan-writer'],
        },
      ],
    },
  },

  // ── p3 data-pipeline ────────────────────────────────────────────────────
  {
    id:    'p3',
    name:  'data-pipeline',
    color: 'orange',
    icon:  'DPL',
    path:  '/projects/data-pipeline',
    chats: 3,
    sessions: [
      s('demo-pipeline-agents',     45 * 60_000, 'running',   undefined),
      s('demo-parallel-fallback',  3 * HOUR,    'completed', 67_000),
      s('codebase-audit',          1 * DAY,     'completed', 220_000),
      s('demo-pipeline-agents',    2 * DAY,     'failed',    12_000),
      s('hello-world',              4 * DAY,     'completed', 4_200),
    ],
    contextTree: {
      id: 'p3-root', type: 'workspace', name: 'data-pipeline',
      order: 0,
      description: 'ETL pipeline demo project',
      children: [
        {
          id: 'p3-ingest', type: 'area', name: 'Ingest',
          order: 0,
          description: 'Data ingestion from sources',
          children: [
            {
              id: 'p3-ingest-api', type: 'project', name: 'API Sources',
              order: 0,
              taskNames: ['demo-pipeline-agents', 'codebase-audit', 'company-research'],
            },
            {
              id: 'p3-ingest-files', type: 'project', name: 'File Sources',
              order: 1,
              taskNames: ['hello-world', 'document-demo', 'display-demo', 'spec-extract'],
            },
          ],
        },
        {
          id: 'p3-transform', type: 'area', name: 'Transform',
          order: 1,
          description: 'Data transformation + enrichment',
          taskNames: ['demo-parallel-fallback', 'demo-pipeline-agents', 'demo-magentic-pattern', 'demo-team-coordination'],
        },
        {
          id: 'p3-load', type: 'area', name: 'Load',
          order: 2,
          description: 'Loading into target stores',
          taskNames: ['hello-world', 'document-demo', 'song-writer'],
        },
        {
          id: 'p3-ops', type: 'area', name: 'Ops',
          order: 3,
          description: 'Operations + audit + tooling',
          taskNames: ['codebase-audit', 'sidecar-tester', 'goal-planner', 'plan-builder', 'plan-writer'],
        },
      ],
    },
  },
]
