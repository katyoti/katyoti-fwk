// SPDX-License-Identifier: Apache-2.0
/**
 * The object the tree marks as selected: the one open in the Object tab (its
 * payload, which the back history restores), not the last node clicked — a
 * closed tab marks nothing.
 */
export interface TabsLike {
  openTabs: { id: string; payload?: unknown }[]
}

export function openObjectId(tabs: TabsLike | undefined): string | undefined {
  const tab = tabs?.openTabs.find((t) => t.id === 'object')
  const id = (tab?.payload as { objectId?: unknown } | undefined)?.objectId
  return typeof id === 'string' && id ? id : undefined
}
