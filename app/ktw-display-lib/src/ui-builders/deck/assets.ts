// SPDX-License-Identifier: Apache-2.0
// Files beside a deck (modules/slidedeck/): an image or a background a deck
// names by a relative path — `![topology](assets/topology.svg)` — is a file of
// the deck's folder. The host publishes those files to the display server; the
// deck view asks the host where each one is (`localAsset`). The Go reader is
// deckapi.LocalAsset: the same rule.

/** The cleaned relative path when `target` names a file beside the deck, else
 *  null: a URL (https:, data:, icon:…), an absolute path, an anchor, or a
 *  path leaving the deck's folder. */
/** A file of the deck object's module (see localAssetPath). */
export const MODULE_SCHEME = 'module:'
export const MODULE_ASSET_PREFIX = '_module/'

export function localAssetPath(target: string): string | null {
  let t = target.trim()
  // `module:/<path>`: a file of the deck object's module, from its root —
  // listed as `_module/<path>` (the Go rule's ModuleAssetPrefix).
  if (t.startsWith(MODULE_SCHEME)) {
    let rest = t.slice(MODULE_SCHEME.length)
    const at = rest.search(/[?#]/)
    if (at >= 0) rest = rest.slice(0, at)
    const segs = rest.replace(/\\/g, '/').split('/').filter((s) => s !== '' && s !== '.')
    if (!segs.length || segs.includes('..')) return null
    return MODULE_ASSET_PREFIX + segs.join('/')
  }
  if (!t || t.startsWith('/') || t.startsWith('#') || t.startsWith('\\')) return null
  const colon = t.indexOf(':')
  if (colon > 0 && !/[/.]/.test(t.slice(0, colon))) return null
  const cut = t.search(/[?#]/)
  if (cut >= 0) t = t.slice(0, cut)
  const out: string[] = []
  for (const seg of t.replace(/\\/g, '/').split('/')) {
    if (seg === '' || seg === '.') continue
    if (seg === '..') {
      if (!out.length) return null
      out.pop()
    } else out.push(seg)
  }
  return out.length ? out.join('/') : null
}
