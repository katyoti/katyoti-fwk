// SPDX-License-Identifier: Apache-2.0
// The window's height below an element: what a `fill` node takes (a board of
// lanes with laneHeight: fill, a card with height: fill).

import { useEffect, useRef, useState, type RefObject } from 'react'

/** The height a `fill` node gets before it is measured (a static render). */
export const FILL_HEIGHT_DEFAULT = 480

/** useFillHeight measures the window's height below an element - what a
 *  `fill` node takes - again on a resize and when the element shows (a tab
 *  panel made visible). */
export function useFillHeight(on: boolean): [RefObject<HTMLDivElement>, number] {
  const box = useRef<HTMLDivElement>(null)
  const [height, setHeight] = useState(FILL_HEIGHT_DEFAULT)
  useEffect(() => {
    const el = box.current
    if (!on || !el) return
    const measure = () => {
      if (el.offsetParent === null) return // hidden (another tab): measured when it shows
      const top = el.getBoundingClientRect().top
      setHeight(Math.max(240, Math.floor(window.innerHeight - top - 40)))
    }
    measure()
    const seen = new ResizeObserver(measure)
    seen.observe(el)
    window.addEventListener('resize', measure)
    return () => { seen.disconnect(); window.removeEventListener('resize', measure) }
  }, [on])
  return [box, height]
}
