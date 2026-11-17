// SPDX-License-Identifier: Apache-2.0
// Static catalog of debug tests, grouped by category. Drives DebugTreePane
// and the test-id → component dispatch in BaseLayout.

export interface DebugTestEntry {
  id: string
  label: string
  description?: string
}

export interface DebugTestGroup {
  id: string
  label: string
  tests: DebugTestEntry[]
}

export const DEBUG_TESTS: DebugTestGroup[] = [
  {
    id: 'documents',
    label: 'Documents',
    tests: [
      {
        id: 'documents',
        label: 'display engines',
        description: 'Every display engine with its sample documents — markdown, HTML, mermaid, code, every UI component (inline and by schema id), charts, media, draw.io — shown through DocumentView; stream replay, edit, save, events.',
      },
      {
        id: 'native-view',
        label: 'native view',
        description: 'A ui-view rendered NATIVELY in ktw-gui (no iframe) through @ktw/display-lib — the host for live state and custom pages.',
      },
      {
        id: 'confluence-status',
        label: 'confluence status (native)',
        description: 'The Confluence sync-status table-tree rendered natively — the same view the terminal prints.',
      },
      {
        id: 'load-widget',
        label: 'loading screen',
        description: 'The shared loading screen (kind="load") — the animated katyoti mascot.',
      },
    ],
  },
]

export function findDebugTest(id: string): { entry: DebugTestEntry; group: DebugTestGroup } | null {
  for (const group of DEBUG_TESTS) {
    const entry = group.tests.find((t) => t.id === id)
    if (entry) return { entry, group }
  }
  return null
}
