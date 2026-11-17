// SPDX-License-Identifier: Apache-2.0
// Renderer overrides: a HOST may draw a node kind with its own component —
// the GUI draws `graph` with @xyflow/react (pan, zoom, minimap), while the
// iframe runtime keeps the built-in SVG renderer. The node's contract does not
// change; an override reads the same node and acts through the same page
// context (usePageContext().onAction). The node's own data-* keys are the
// override's to place: `dataAttrs(node)` spread first on its root element,
// as every built-in renderer does.

import type { ComponentType } from 'react'

import type { UIPageNode } from './ui-page.types.ts'

type Override = ComponentType<{ node: UIPageNode }>

const overrides = new Map<string, Override>()

/** registerPageNodeRenderer makes `kind` render with `component` in this
 *  process (idempotent; the last registration wins). */
export function registerPageNodeRenderer(kind: UIPageNode['kind'], component: Override | undefined): void {
  if (component) overrides.set(kind, component)
  else overrides.delete(kind)
}

/** pageNodeOverride is the host's renderer for a kind, if it registered one. */
export function pageNodeOverride(kind: string): Override | undefined {
  return overrides.get(kind)
}
