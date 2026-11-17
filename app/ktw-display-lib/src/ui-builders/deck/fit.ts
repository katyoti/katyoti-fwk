// SPDX-License-Identifier: Apache-2.0
// Fitting a slide to its page (modules/slidedeck/): a slide whose content is
// taller (or wider) than the slide — seven long agenda items, a dense list in
// a theme with large type — is shrunk as a whole, its text sizes being
// relative to the slide's, down to FIT_MIN. A slide that still does not fit is
// flagged (`data-overflow`): the editing view says to split it. What is shown
// is what the exports measure, so they get the fitted size too.

/** The smallest a slide's text is shrunk to. */
export const FIT_MIN = 0.55

/** A slide overflows when its content is taller or wider than its box, when
 *  its box itself grew taller than the page (frame) — reveal.js then centres
 *  it and it spills over the top and the bottom — or when its text runs into
 *  the slide's margins (its padding: where the theme puts its rules, logo and
 *  footer, and the view its buttons). */
function overflows(el: HTMLElement, frame: number): boolean {
  if (el.scrollHeight > el.clientHeight + 1 || el.scrollWidth > el.clientWidth + 1) return true
  if (frame > 0 && el.offsetHeight > frame + 1) return true
  const box = el.getBoundingClientRect()
  if (!box.height || !el.offsetHeight) return false
  const scale = box.height / el.offsetHeight // reveal.js scales the page
  const cs = getComputedStyle(el)
  const top = box.top + parseFloat(cs.paddingTop) * scale - 1
  const bottom = box.bottom - parseFloat(cs.paddingBottom) * scale + 1
  for (const c of el.querySelectorAll<HTMLElement>('.deck-title *, .deck-slot *')) {
    const r = c.getBoundingClientRect()
    if (r.height && (r.top < top || r.bottom > bottom)) return true
  }
  return false
}

/** Fits one slide (its `.deck-slide` element) to a page `frame` px tall and
 *  returns the scale kept: 1 when it fits as written. A slide not laid out
 *  (hidden) is left alone. */
export function fitSlide(el: HTMLElement, frame = 0): number {
  el.style.removeProperty('font-size')
  delete el.dataset.fit
  delete el.dataset.overflow
  if (!el.clientHeight || !overflows(el, frame)) return 1
  const base = parseFloat(getComputedStyle(el).fontSize)
  if (!base) return 1
  let lo = FIT_MIN
  let hi = 1
  for (let i = 0; i < 7; i++) {
    const mid = (lo + hi) / 2
    el.style.fontSize = `${base * mid}px`
    if (overflows(el, frame)) hi = mid
    else lo = mid
  }
  el.style.fontSize = `${base * lo}px`
  el.dataset.fit = lo.toFixed(2)
  if (overflows(el, frame)) el.dataset.overflow = 'true'
  return lo
}

/** Fits every slide of root that is laid out now (reveal.js lays out the
 *  slides near the current one; print lays out all). */
export function fitSlides(root: ParentNode): void {
  // The page: reveal.js lays the slides out at the deck's size (1280×720).
  const slides = root.querySelector<HTMLElement>('.reveal .slides')
  const frame = slides ? slides.offsetHeight : 0
  root.querySelectorAll<HTMLElement>('.reveal .slides section .deck-slide').forEach((el) => {
    if (el.offsetParent !== null || el.getClientRects().length > 0) fitSlide(el, frame)
  })
}
