// SPDX-License-Identifier: Apache-2.0
// « Retirer » and the hard delete: « Retirer » archives the
// object - its file kept in its module's history, its links read as ended -;
// the hard delete sits in the pane's « ⋯ » menu, offered only when nothing points to the object.

/** Why the hard delete is not offered, or undefined when it is. */
export type HardDeleteBlock = 'checking' | 'linked' | 'children' | 'unknown'

/** hardDeleteBlock says whether the hard delete is offered: `inbound` the
 *  ids of the objects linking to it (undefined while asked), `inboundError`
 *  the question's failure, `children` how many children it holds. */
export function hardDeleteBlock(inbound: string[] | undefined, inboundError: string, children: number): HardDeleteBlock | undefined {
  if (inboundError) return 'unknown'
  if (inbound === undefined) return 'checking'
  if (inbound.length > 0) return 'linked'
  if (children > 0) return 'children'
  return undefined
}
