// SPDX-License-Identifier: Apache-2.0
// Apply outer-data binding to a single node before it's passed to its
// per-kind renderer. When the node has a `dataField: "$.path"` reference,
// the resolver fills the node's "main" field (per kind) from the bound
// value, replacing any inline value.
//
// Wired by display-package-restructure Phase 3.4 (2026-05-06). Per the
// host-driven data flow decision: this runs at render time inside the
// iframe, pure (no side effects), no network access.

import { resolveJSONPath } from '../../../utils/json-path.ts'
import type { UIPageNode } from '../ui-page.types.ts'
import { objectCardRow } from '../ui-page.interactive.ts'

/** Apply data-binding to a node. Returns a new node with the bound
 *  field filled from `data` via JSONPath, or the original node when
 *  `dataField` is absent or `data` is undefined.
 *
 *  The "main" field varies per kind (heading→text, kpi→value,
 *  table→data, etc.) — see the switch below. */
export function applyDataBinding(node: UIPageNode, data: unknown): UIPageNode {
  return bindMain(bindId(node, data), data)
}

/** bindId gives a node its id from the data when it declares `idField`: the
 *  bound value is appended to its id (`<id>:<value>`), or is its id when it
 *  has none - so a node in a row template draws its own ids in each row
 *  (`$.item.id`), its text toggle included. An unresolved or
 *  empty value leaves the node as it is. */
function bindId(node: UIPageNode, data: unknown): UIPageNode {
  if (!node.idField || data === undefined) return node
  const value = stringify(resolveJSONPath(data, node.idField))
  if (value === '') return node
  return { ...node, id: node.id ? `${node.id}:${value}` : value } as UIPageNode
}

function bindMain(node: UIPageNode, data: unknown): UIPageNode {
  if (!node.dataField || data === undefined) return node

  const value = resolveJSONPath(data, node.dataField)
  if (value === undefined) return node

  switch (node.kind) {
    // Atomic: bind into the "main" content field
    case 'heading':
      return { ...node, text: stringify(value) }
    case 'text':
      return { ...node, content: stringify(value) }
    case 'divider':
      return { ...node, label: stringify(value) }
    case 'badge':
      return { ...node, text: stringify(value) }
    case 'monogram':
      return { ...node, text: stringify(value) }
    case 'link':
      return { ...node, text: stringify(value) }
    case 'image':
    case 'video':
    case 'audio':
    case 'file':
      // Media nodes: the bound field is the URL. For 'file' the caller
      // should set a separate name via the static node payload; this
      // path only updates `src`.
      return { ...node, src: stringify(value) }
    case 'timeline':
      // Timeline binds an Array of items. Wildcards (`$.events[*]`)
      // return the array naturally; non-array values yield an empty
      // timeline (renderer handles `items: []` cleanly).
      return {
        ...node,
        items: Array.isArray(value) ? (value as typeof node.items) : [],
      }
    case 'kpi':
      // KPI value is more permissive — preserve numbers; stringify the rest.
      return { ...node, value: typeof value === 'number' ? value : stringify(value) }
    case 'code-block':
      return { ...node, content: stringify(value) }
    case 'markdown':
      return { ...node, content: stringify(value) }
    case 'drawio':
      return { ...node, xml: stringify(value) }
    case 'legend':
      return { ...node, items: Array.isArray(value) ? (value as typeof node.items) : [] }
    case 'chart':
      // Chart's main field is the `data` array. Wildcards (`$.series[*]`)
      // return arrays naturally; non-array values get wrapped in [] so
      // the renderer's empty-data placeholder doesn't crash on bad input.
      return { ...node, data: Array.isArray(value) ? value as Array<Record<string, unknown>> : [] }

    // Schema-bound: bind into data (the schema's record value
    // or array). Tables typically use this with `$.path[*]` for rows.
    case 'form':
    case 'view':
    case 'table':
    case 'wizard':
      return { ...node, data: value }
    case 'timeline-lanes':
      return { ...node, data: Array.isArray(value) ? value : [] }

    // Interactive: the collections bind their elements; the gauge its value;
    // the graph its {nodes, edges}; a button its label; a select its value; a
    // menu its entries.
    case 'list':
    case 'tree':
    case 'lanes':
    case 'gantt':
      return { ...node, data: Array.isArray(value) ? value as never : [] }
    case 'graph':
      return { ...node, data: value && typeof value === 'object' && !Array.isArray(value) ? value as never : { nodes: [], edges: [] } }
    case 'picker':
      return { ...node, selected: Array.isArray(value) ? value as never : [] }
    case 'stepper':
      // A list gives the steps (a request's own, each with its state); a
      // value, the current step.
      return Array.isArray(value) ? { ...node, steps: value as never } : { ...node, current: stringify(value) }
    case 'board':
    case 'grid-board':
      return { ...node, data: Array.isArray(value) ? value as never : [] }
    case 'progress':
      // A number is how far the work is; an object its bar.
      return typeof value === 'number' ? { ...node, current: value } : value && typeof value === 'object' ? { ...node, ...(value as object) } as never : node
    case 'log':
      // The lines written so far.
      return { ...node, data: Array.isArray(value) ? value as never : [] }
    case 'item-detail':
      // An opened item: what its source's fetch answers.
      return { ...node, data: value && typeof value === 'object' && !Array.isArray(value) ? value as never : node.data }
    case 'source-search':
      // A multi-source search's answers: its sources, or {sources}.
      return { ...node, data: Array.isArray(value) ? { sources: value as never } : value && typeof value === 'object' ? value as never : node.data }
    case 'progress-list':
      // The stages of a run.
      return { ...node, stages: Array.isArray(value) ? value as never : [] }
    case 'terms':
      // The search terms bind as strings.
      return { ...node, terms: Array.isArray(value) ? value.map((v) => stringify(v)) : [] }  // de-duplicated where drawn
    case 'chips':
      // A chip group binds the rows it counts.
      return { ...node, countRows: Array.isArray(value) ? value as never : [] }
    case 'gauge':
      // A list gives the stacked segments (a goal's requests by state).
      return Array.isArray(value) ? { ...node, segments: value as never } : { ...node, value: Number(value) || 0 }
    case 'button':
      return { ...node, label: stringify(value) }
    case 'select':
      return { ...node, value: stringify(value) }
    // Choice cards bind their options; a text field its starting value.
    case 'choice-cards':
      return { ...node, options: Array.isArray(value) ? value as never : [] }
    case 'text-field':
      return { ...node, value: stringify(value) }
    case 'menu':
      return { ...node, items: Array.isArray(value) ? value as never : [] }
    case 'object-card':
      // The object's CWM row.
      return { ...node, row: objectCardRow(value) }

    // A section bound to a collection presents it: its count is the badge
    // (unless one is declared), and with no element it shows its empty state
    // instead of its children.
    case 'section':
      if (!Array.isArray(value)) return node
      return { ...node, badge: node.badge ?? value.length, children: value.length === 0 ? [] : node.children }

    // The other layout nodes: dataField has no meaning. Silently ignore.
    case 'vbox':
    case 'hbox':
    case 'grid':
    case 'box':
    case 'card':
    case 'tabs':
    case 'accordion':
    case 'split':
    // A search box and a switch read the selection, not the data.
    case 'search':
    case 'switch':
    // An action bar reads its parts' paths in the data itself.
    case 'action-bar':
      return node

    // A panel's data are its events': it binds nothing.
    case 'panel':
      return node

    default: {
      // Exhaustiveness — TS will complain if a new kind is added without
      // a binding rule.
      const _exhaustive: never = node
      void _exhaustive
      return node
    }
  }
}

/** Coerce a resolved value to string for display fields. Arrays/objects
 *  are JSON-stringified compactly; null/undefined become empty string. */
function stringify(value: unknown): string {
  if (value === null || value === undefined) return ''
  if (typeof value === 'string') return value
  if (typeof value === 'number' || typeof value === 'boolean') return String(value)
  try {
    return JSON.stringify(value)
  } catch {
    return String(value)
  }
}
