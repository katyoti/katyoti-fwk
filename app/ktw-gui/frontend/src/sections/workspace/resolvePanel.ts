// SPDX-License-Identifier: Apache-2.0
// resolvePanelTree — a page tree made self-contained the way the object pane
// resolves a declared panel (object-pane/ResolvePanel): schemas named by id
// or by an action's input, ui blocks and options inlined. A tree holding no
// schema-bound node is answered as it is, without a call; a data-type named
// by URI is read in the person's language. Used by a panel's
// page (RenderDefView) and by a page section (the host's loadSection).

import { hasSchemaBound } from '@ktw/display-lib/ui-builders/page/index.ts'
import { WorkspaceRunAction } from '../../../wailsjs/go/main/App'
import { ct, ctSchema } from '../../i18n/contrib'
import { localizeResolved, type Localize } from '../../lib/specCache'

export const RESOLVE_PANEL = 'ktw-agents:object-pane/resolve-panel'

/** The person's language: a data-type's texts from the contribution
 *  translations in the GUI language, each falling back to its own (ctSchema). */
const guiLocalize: Localize = (uri, part) => ctSchema(ct, uri, part)

/** resolvePanelTree answers the tree resolved, its data-types read in the
 *  person's language (`localize`, default the GUI's: localizeResolved); it
 *  rejects with the pane's error. */
export async function resolvePanelTree(universeName: string, tree: unknown, localize: Localize = guiLocalize): Promise<unknown> {
  if (!hasSchemaBound(tree)) return tree
  const r = await WorkspaceRunAction(universeName, RESOLVE_PANEL, JSON.stringify({ def: { kind: 'page', dslTree: tree } }))
  if (r.error) throw new Error(r.error)
  const out = JSON.parse(r.value || '{}') as { def?: { dslTree?: unknown } }
  return localizeResolved(tree, out.def?.dslTree, localize)
}

/** A page tree's last resolution: whose page it was (its identity: the
 *  universe, the object or scope, the panel or source), the tree (by its key)
 *  it was asked for, and what came back. */
export interface Resolution { identity?: string; key: string; tree?: unknown; error?: string }

/** shownResolution answers what a page shows for the tree `key` of the page
 *  `identity` given the last resolution `got`: that resolution once it is the
 *  tree's; while the next tree of the SAME page resolves, the last resolved
 *  tree stays on screen, so a refresh re-renders the page in place (its
 *  filter, its selection, a half-typed answer) instead of unmounting it behind
 *  a loader. Another page's tree (another universe, object, scope or panel) is
 *  never shown: its actions would run against the new scope — nothing (the
 *  loader) until this page's own tree resolved. */
export function shownResolution(key: string, got: Resolution, identity = ''): { tree?: unknown; error?: string } {
  if ((got.identity ?? '') !== identity) return {}
  if (got.key === key) return { tree: got.tree, error: got.error }
  if (got.tree !== undefined) return { tree: got.tree }
  return {}
}

/** pageIdentity — whose page a tree is: the universe, the object, the scope
 *  its actions read, the panel. Also the key its direct mounts carry, so
 *  React remounts the page (its filter, its selection) on another one. */
export function pageIdentity(universeName: string, objectId: string, scope: Record<string, unknown> | undefined, panel?: string): string {
  return JSON.stringify([universeName, objectId, scope ?? { id: objectId }, panel ?? ''])
}
