// SPDX-License-Identifier: Apache-2.0
/**
 * The object a tool result names to open (the action result convention:
 * `{ok, message, open}` - a create tool's): its
 * id, or `<id>#<panel>`; null when the result does not name one or says it
 * failed.
 */
export function openOf(output: string | undefined): { id: string; panel?: string } | null {
  if (!output) return null
  let out: unknown
  try { out = JSON.parse(output) } catch { return null }
  if (!out || typeof out !== 'object') return null
  const r = out as { ok?: unknown; open?: unknown }
  if (r.ok === false || typeof r.open !== 'string' || !r.open.trim()) return null
  const [id, panel] = r.open.trim().split('#')
  return panel ? { id, panel } : { id }
}
