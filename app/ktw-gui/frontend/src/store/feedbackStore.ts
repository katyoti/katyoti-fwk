// SPDX-License-Identifier: Apache-2.0
/**
 * Feedback store — the Feedback form's open state and the screenshot taken
 * when it was asked for.
 *
 * openFeedback captures the window FIRST (FeedbackCaptureWindow, a native
 * capture of the client area) and only then opens the form, so the form is
 * not in the picture. When the capture fails (another platform, no window),
 * the form opens without one.
 */

import { create } from 'zustand'
import { FeedbackCaptureWindow } from '../../wailsjs/go/main/App'
import { useAppStore } from '../store'
import { useTabsStore } from './tabsSlice'

/** How long the screen gets to settle after the click, before the capture. */
const CAPTURE_SETTLE_MS = 200

interface FeedbackState {
  open: boolean
  /** True while the window is being captured, before the form opens. */
  capturing: boolean
  /** Base64 PNG of the window; empty when none was taken or it was removed. */
  screenshot: string
  /** The universe and the place in the app the form was opened from. */
  universe: string
  route: string

  openFeedback: () => Promise<void>
  close: () => void
  dropScreenshot: () => void
}

export const useFeedbackStore = create<FeedbackState>((set, get) => ({
  open: false,
  capturing: false,
  screenshot: '',
  universe: '',
  route: '',

  openFeedback: async () => {
    if (get().open || get().capturing) return
    const universe = useAppStore.getState().displayedUniverseName ?? ''
    const { openTabs, activeTabId } = useTabsStore.getState().getForUniverse(universe || '__none__')
    const tab = openTabs.find((t) => t.id === activeTabId) ?? openTabs[0]
    const route = tab ? `${tab.kind}: ${tab.label}` : ''

    set({ capturing: true })
    // Let a closing menu or a tooltip leave the screen before the capture.
    await new Promise((r) => setTimeout(r, CAPTURE_SETTLE_MS))
    let screenshot = ''
    try {
      const res = await FeedbackCaptureWindow()
      if (res.error) console.warn('[feedback] no screenshot:', res.error)
      screenshot = res.png ?? ''
    } catch (err) {
      console.warn('[feedback] no screenshot:', err)
    }
    set({ capturing: false, open: true, screenshot, universe, route })
  },

  close: () => set({ open: false, screenshot: '' }),
  dropScreenshot: () => set({ screenshot: '' }),
}))
