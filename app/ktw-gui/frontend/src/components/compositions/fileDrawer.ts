// SPDX-License-Identifier: Apache-2.0
// What a page's `drawer: {file, object?, title?}` action opens:
// a file of an object - a markdown section, a draw.io diagram, a document -
// read in place in a reader drawer, never by leaving for the Files tab.
// The GUI always opens the reader (its switch retired); `sourceModel`
// false - a host without the source model - opens the Files tab.
//
// React-free: tested with `bun test` (frontend/tests/file-reader.test.ts).

/** Where a file drawer opens: the reader drawer, or the Files tab. */
export type FileDrawerOpen =
  | { kind: 'reader'; object: string; file: string; title: string }
  | { kind: 'files'; object: string; file: string }

/** readerTitle is the reader's title: the action's, else the file's name. */
export function readerTitle(file: string, title?: string): string {
  if (title && title.trim() !== '') return title
  const parts = file.split(/[\/]/).filter((p) => p !== '')
  return parts[parts.length - 1] ?? file
}

/** fileDrawer says where a drawer naming a file opens: the object is the
 *  drawer's, else the page's (`here`); nothing when there is no file or no
 *  object. */
export function fileDrawer(d: { file?: string; object?: string; title?: string }, here: string, sourceModel: boolean): FileDrawerOpen | undefined {
  const file = (d.file ?? '').trim()
  const object = d.object || here
  if (!file || !object) return undefined
  return sourceModel
    ? { kind: 'reader', object, file, title: readerTitle(file, d.title) }
    : { kind: 'files', object, file }
}

/** readerDocument is the document the reader shows: the file by its
 *  module-relative path, read and typed in Go (DocumentOpen) with the object
 *  as its node - its media type picks the engine (markdown, draw.io, …). */
export function readerDocument(file: string, title: string): { source: { path: string }; title: string } {
  return { source: { path: file }, title }
}
