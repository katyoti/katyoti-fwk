// SPDX-License-Identifier: Apache-2.0
// Reading a document's inline content, the one way every engine and the host
// do it.
import type { Document } from '../../generated/document'

/** The content as text: the string itself, or indented JSON for a structured
 *  value. Empty when the document has no inline content. */
export function documentText(doc: Document): string {
  const c = doc.content
  if (c === undefined || c === null) return ''
  if (typeof c === 'string') {
    if (doc.mediaType === 'application/json') {
      try { return JSON.stringify(JSON.parse(c), null, 2) } catch { return c }
    }
    return c
  }
  try { return JSON.stringify(c, null, 2) } catch { return String(c) }
}

/** Size of the content in bytes (base64 content counted decoded). */
export function documentSize(doc: Document): number {
  const text = typeof doc.content === 'string' ? doc.content : documentText(doc)
  return doc.encoding === 'base64' ? Math.floor((text.length * 3) / 4) : text.length
}

/** A data: URL for binary (base64) or textual content, for engines that hand
 *  the bytes to the browser (image, pdf). */
export function documentDataUrl(doc: Document): string {
  const text = typeof doc.content === 'string' ? doc.content : documentText(doc)
  const payload = doc.encoding === 'base64' ? text : btoa(unescape(encodeURIComponent(text)))
  return `data:${doc.mediaType};base64,${payload}`
}

/** highlight.js names that differ from the media type's `text/x-<name>`. */
const HLJS_ALIASES: Record<string, string> = { sh: 'bash', tsx: 'typescript', jsx: 'javascript', toml: 'ini' }

/** The highlighting language of a text / code media type, or '' for plain
 *  text: `text/x-<language>`, and the named data and web types. */
export function codeLanguage(mediaType: string): string {
  const base = mediaType.split(';')[0].trim().toLowerCase()
  if (base.startsWith('text/x-')) {
    const lang = base.slice('text/x-'.length)
    return HLJS_ALIASES[lang] ?? lang
  }
  switch (base) {
    case 'application/json': return 'json'
    case 'application/yaml': case 'application/x-yaml': return 'yaml'
    case 'application/xml': case 'image/svg+xml': return 'xml'
    case 'text/css': return 'css'
    case 'text/javascript': return 'javascript'
    case 'text/csv': case 'text/plain': return ''
  }
  if (base.endsWith('+json')) return 'json'
  if (base.endsWith('+xml')) return 'xml'
  return ''
}

const SCHEMA_BOUND = new Set(['form', 'view', 'table', 'tree-table', 'wizard'])

/** Whether a ui-component content still names a schema by id or carries a
 *  ui block anywhere (a page's nested nodes included) — Go resolves those
 *  (DocumentOpen) before it is shown. */
export function needsResolution(content: unknown): boolean {
  if (Array.isArray(content)) return content.some(needsResolution)
  if (!content || typeof content !== 'object') return false
  const c = content as Record<string, unknown>
  if (typeof c.kind === 'string' && SCHEMA_BOUND.has(c.kind) && (typeof c.schema === 'string' || c.ui !== undefined)) {
    return true
  }
  return Object.entries(c).some(([k, v]) => k !== 'schema' && k !== 'data' && k !== 'ui' && needsResolution(v))
}
