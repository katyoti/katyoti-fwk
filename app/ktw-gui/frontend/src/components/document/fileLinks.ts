// SPDX-License-Identifier: Apache-2.0
// A document read from a file (an object's attached file, a module file) may
// point at its neighbours: `![diagram](media/flow.drawio)`, `[next](01-a.md)`.
// These helpers resolve such a relative reference against the file's folder,
// so the markdown engine draws the image or diagram in place and a link to a
// sibling opens it (modules/object-pane — the Files tab).

/** True for a reference inside the same tree: no scheme, no leading `/`, not
 *  an anchor, not a data URI. */
export function isRelativeRef(ref: string | undefined | null): boolean {
  if (!ref) return false
  const r = ref.trim()
  if (r === '' || r.startsWith('#') || r.startsWith('/') || r.startsWith('\\')) return false
  return !/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(r)
}

/** The folder of a slash path ('' at the root). */
export function folderOf(path: string): string {
  const p = path.replace(/\\/g, '/')
  const i = p.lastIndexOf('/')
  return i < 0 ? '' : p.slice(0, i)
}

/** Joins a relative reference onto a folder, resolving `.` and `..`; the
 *  query and the anchor are dropped and percent-escapes decoded. Undefined
 *  when it climbs above the root (the module file API refuses it anyway). */
export function joinRef(folder: string, ref: string): string | undefined {
  let r = ref.trim().replace(/\\/g, '/')
  r = r.replace(/[?#].*$/, '')
  try { r = decodeURIComponent(r) } catch { /* kept as written */ }
  const parts = folder ? folder.split('/').filter(Boolean) : []
  for (const seg of r.split('/')) {
    if (seg === '' || seg === '.') continue
    if (seg === '..') {
      if (parts.length === 0) return undefined
      parts.pop()
      continue
    }
    parts.push(seg)
  }
  return parts.length ? parts.join('/') : undefined
}

/** An image the browser draws by itself, as opposed to a document (a
 *  diagram, a PDF) an engine draws in a frame of its own height. */
export function isPlainImage(path: string): boolean {
  return /\.(png|jpe?g|gif|webp|bmp|svg)$/i.test(path)
}

/** CommonMark refuses a link destination holding a space unless it is
 *  written `<…>`; files exported from Confluence write
 *  `![x](media/My diagram.drawio)`. This wraps such destinations so the
 *  reference parses — outside fenced code, and only when the destination has
 *  no title (a quote) and is not already wrapped. */
export function wrapSpacedDestinations(md: string): string {
  let fenced = false
  return md.split('\n').map((line) => {
    if (/^\s*(```|~~~)/.test(line)) {
      fenced = !fenced
      return line
    }
    if (fenced) return line
    return line.replace(/(\]\()([^()<>\n"']*\s[^()<>\n"']*)(\))/g, (_m, open: string, dest: string, close: string) => `${open}<${dest.trim()}>${close}`)
  }).join('\n')
}

/** A file exported from Confluence opens with a YAML frontmatter (`page_id`,
 *  `title`): metadata, not prose — dropped before the text is drawn. */
export function stripFrontmatter(md: string): string {
  const m = /^\uFEFF?---\r?\n[\s\S]*?\r?\n---[ \t]*(\r?\n|$)/.exec(md)
  return m ? md.slice(m[0].length) : md
}
