// SPDX-License-Identifier: Apache-2.0
// Tabler icons resolved from a NAME at runtime — the one resolver.
//
// A name comes from data (a contribution's icon, a task's iconName, a UIAction
// icon), so any of the ≈6000 icons may be asked for. Importing the icon set
// statically put all of them in the startup bundle (≈3.6 MB); here the set is
// a separate chunk, fetched the first time a name is resolved. Until it
// arrives a named icon renders its fallback, then re-renders as itself.
// Icons imported by name in code (`import { IconX }`) are unaffected.

import { forwardRef, useSyncExternalStore } from 'react'
import type React from 'react'
import type { Icon as TablerIcon, IconProps } from '@tabler/icons-react'

type IconSet = Record<string, TablerIcon | undefined>

let iconSet: IconSet | undefined
let loading: Promise<IconSet> | undefined
const listeners = new Set<() => void>()

/** Fetch the icon set once; later calls share the same promise. */
export function loadTablerIcons(): Promise<IconSet> {
  loading ??= import('@tabler/icons-react').then((m) => {
    iconSet = m as unknown as IconSet
    listeners.forEach((l) => l())
    return iconSet
  })
  return loading
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  void loadTablerIcons()
  return () => listeners.delete(listener)
}

/** The icon set, or undefined while it loads (re-renders when it arrives). */
export function useTablerIconSet(): IconSet | undefined {
  return useSyncExternalStore(subscribe, () => iconSet, () => iconSet)
}

/** "layout-grid" and "IconLayoutGrid" both name IconLayoutGrid. */
export function tablerComponentName(name: string): string {
  if (/^Icon[A-Z0-9]/.test(name)) return name
  return 'Icon' + name.split('-').map((p) => p.charAt(0).toUpperCase() + p.slice(1)).join('')
}

const pending = new WeakMap<TablerIcon, Map<string, TablerIcon>>()

/**
 * tablerIcon resolves an icon NAME to its component, or to `fallback` when
 * the name is absent or unknown. While the set is still loading it returns a
 * stand-in that shows the fallback and switches to the named icon once loaded
 * (one stable stand-in per name and fallback, so React keeps its state).
 */
export function tablerIcon(name: string | undefined, fallback: TablerIcon): TablerIcon {
  if (!name) return fallback
  const component = tablerComponentName(name)
  if (iconSet) return iconSet[component] ?? fallback
  let byName = pending.get(fallback)
  if (!byName) pending.set(fallback, (byName = new Map()))
  let standIn = byName.get(component)
  if (!standIn) {
    standIn = forwardRef<SVGSVGElement, IconProps>(function NamedIcon(props, ref) {
      const set = useTablerIconSet()
      const Resolved = (set?.[component] ?? fallback) as React.ElementType
      return <Resolved ref={ref} {...props} />
    }) as TablerIcon
    byName.set(component, standIn)
  }
  return standIn
}
