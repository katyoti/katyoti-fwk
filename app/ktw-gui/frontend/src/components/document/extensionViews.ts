// SPDX-License-Identifier: Apache-2.0
// Named UI extension views — the `extension` component type of a
// ui-component document (What's Next, the module dashboard, …). The views are
// the GUI's own; the section that owns them registers them here, so the
// ui-components engine mounts them without importing any section.
import type { ReactNode } from 'react'
import type { EngineObjectContext } from './engines'

/** What an extension view is mounted with: the object it is shown on and the
 *  component's own input (its `data`). */
export interface ExtensionViewContext extends EngineObjectContext {
  input: unknown
}

export type ExtensionView = (ctx: ExtensionViewContext) => ReactNode

const views = new Map<string, ExtensionView>()

export function registerExtensionViews(entries: Record<string, ExtensionView>): void {
  for (const [name, view] of Object.entries(entries)) views.set(name, view)
}

export function extensionView(name: string | undefined): ExtensionView | undefined {
  return name ? views.get(name) : undefined
}
