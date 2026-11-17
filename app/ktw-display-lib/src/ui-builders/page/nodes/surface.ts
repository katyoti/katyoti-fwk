// SPDX-License-Identifier: Apache-2.0
// A box's background in both colour schemes. A light
// shade (`<colour>.0`-`.2`, `white`) stays as given in the light scheme and
// becomes the colour's dark tint (Mantine's `--mantine-color-<c>-light`, its
// shade at 15 % over the body) in the dark scheme, so the body text keeps its
// contrast there. Any other value - a mid or dark shade, a CSS variable, a
// `light-dark()` pair - is left as it is.

const LIGHT_SHADE = /^([a-z]+)\.([0-2])$/

export function surfaceBg(bg: string | undefined): string | undefined {
  if (!bg) return bg
  if (bg === 'white') return 'var(--mantine-color-body)'
  const m = LIGHT_SHADE.exec(bg)
  // `dark.0`-`.2` are Mantine's light greys of the dark palette, used as
  // stripes and marks, not as surfaces: left alone.
  if (!m || m[1] === 'dark') return bg
  return `light-dark(var(--mantine-color-${m[1]}-${m[2]}), var(--mantine-color-${m[1]}-light))`
}
