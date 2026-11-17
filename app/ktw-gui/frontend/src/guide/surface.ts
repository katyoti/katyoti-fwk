// SPDX-License-Identifier: Apache-2.0
/**
 * Where the guide opens: the `guide` tab inside a universe, or a drawer where
 * there are no tabs (the universe picker, the edit/delete panes, a terminal
 * or session view). `topics` narrows the list to the recipes about what is on
 * screen (the `?` buttons).
 */

import { create } from 'zustand'

export const GUIDE_TAB_ID = 'guide'

/** Payload of the `guide` tab. */
export interface GuideTabPayload {
  topics?: string[]
}

interface GuideDrawerState {
  open: boolean
  topics: string[]
  openDrawer: (topics?: string[]) => void
  setTopics: (topics: string[]) => void
  close: () => void
}

export const useGuideDrawer = create<GuideDrawerState>((set) => ({
  open: false,
  topics: [],
  openDrawer: (topics) => set({ open: true, topics: topics ?? [] }),
  setTopics: (topics) => set({ topics }),
  close: () => set({ open: false }),
}))

/** Topics of the recipes about a main-pane tab kind (the tab strip's `?`). */
export function topicsForTabKind(kind: string | undefined): string[] {
  switch (kind) {
    case 'inbox':       return ['inbox']
    case 'marketplace': return ['marketplace', 'config-set']
    case 'chat':        return ['chat']
    case 'object':      return ['tree', 'actions']
    default:            return []
  }
}

/** Topics of the project panel's `?`. */
export const PROJECT_PANEL_TOPICS = ['tree', 'project']
