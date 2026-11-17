// SPDX-License-Identifier: Apache-2.0
// The action a task reference names (taskForm's lookup), kept apart so it is
// tested without the Wails bindings.

/** taskActionURI is the `<provider>:tasks/<name>` action a task reference
 *  names, or '' when it names none (a bare name). */
export function taskActionURI(ref: string): string {
  const r = ref.trim()
  const i = r.indexOf(':tasks/')
  if (i > 0) return r
  const j = r.indexOf(':/task/')
  if (j > 0) return `${r.slice(0, j)}:tasks/${r.slice(j + ':/task/'.length)}`
  return ''
}
