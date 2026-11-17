// SPDX-License-Identifier: Apache-2.0
// autoCloseXml — balance open tags in a partial mxGraph buffer so the
// document is parseable by DOMParser even mid-stream.
//
// Design rationale: modules/drawio/docs/viewer-and-streaming.md §3 —
// token-level streaming sends invalid XML
// fragments (`<mxfile>...<mxCell id="2" val`) that the strict parser
// rejects, leaving the canvas blank. The shim walks the buffer, tracks
// open tags on a stack, and appends balancing closes so each debounce
// tick produces a renderable document — possibly with fewer cells than
// the final state, growing visibly as the stream arrives.
//
// **Grammar assumptions** (mxGraph producers):
//   - Tag set is small + well-formed: mxfile, diagram, mxGraphModel,
//     root, mxCell, mxGeometry, UserObject, Array, mxPoint.
//   - No DOCTYPE, no CDATA, no processing instructions.
//   - Attributes use double-quoted values.
//   - Self-closing tags use `/>` (e.g. `<mxGeometry .../>`).
//
// **Edge cases** handled by truncation at last `>`:
//   - Partial tag (`<mxCell id="2" val`) → truncated; no broken half-
//     element in output. The cell appears on the next chunk when its
//     `>` arrives.
//   - Partial attribute value (`<mxCell value="Box`) → truncated.
//   - Self-closing tags → recognised via the `(/?)` capture, not pushed.

const TAG_RE = /<\/?([A-Za-z][\w-]*)[^>]*?(\/?)>/g

/**
 * Balance open tags in a partial mxGraph buffer. Returns a string that
 * is always parse-clean (or empty when no complete tag has arrived).
 *
 * Behaviour:
 *   - Empty / whitespace-only input → returns '' (caller renders empty).
 *   - Already-valid input (closing tags balance opens) → returns input.
 *     `mount(complete)` ↔ `mount(autoCloseXml(complete))` produce the
 *     same parse tree.
 *   - Partial input → truncates at the last complete tag boundary
 *     (`>`), then appends `</tag>` for each unclosed tag, deepest first.
 *
 * @param partial mxGraph XML, possibly truncated mid-stream.
 * @returns balanced XML, or '' if no complete tag has arrived yet.
 */
export function autoCloseXml(partial: string): string {
  if (!partial) return ''
  const lastClose = partial.lastIndexOf('>')
  if (lastClose < 0) return ''
  // Truncate any in-progress tag fragment after the last complete `>`.
  const buf = partial.substring(0, lastClose + 1)

  const stack: string[] = []
  // Reset regex state — TAG_RE is module-level and stateful (g flag).
  TAG_RE.lastIndex = 0
  let m: RegExpExecArray | null
  while ((m = TAG_RE.exec(buf)) !== null) {
    const [full, name, selfClose] = m
    if (full.startsWith('</')) {
      // Closing tag — pop matching open. Use lastIndexOf to handle
      // misnested-but-recoverable cases (LLM omitted a close earlier).
      const i = stack.lastIndexOf(name)
      if (i >= 0) stack.length = i
    } else if (selfClose !== '/') {
      // Opening tag (not self-closing) — push.
      stack.push(name)
    }
    // Self-closing tags don't affect the stack.
  }

  if (stack.length === 0) return buf

  // Build close tags in reverse stack order — innermost first.
  let closes = ''
  for (let i = stack.length - 1; i >= 0; i--) {
    closes += `</${stack[i]}>`
  }
  return buf + closes
}
