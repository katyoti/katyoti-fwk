// SPDX-License-Identifier: Apache-2.0
/**
 * « Pour aller plus loin »: a recipe's `next:` - what to read
 * once the guide is done - shown by the player as the guide's last step. One
 * mechanism for every guide: a recipe never writes that step itself.
 *
 * An entry is a guide id (a guide of the same provider), a guide URI
 * (`<provider>:/guide-recipe/<id>`), or an element address of a document
 * (`object/<id>`), opened in the person's universe.
 */
import type { LoadedRecipe } from './recipes'

/** The id of the step the player adds at the end. */
export const FURTHER_STEP = '__further'

/** What an entry of `next` opens. */
export type FurtherTarget = { kind: 'guide'; uri: string } | { kind: 'address'; address: string }

const ADDRESS = /^(object|home|screen|rail)\//

/** The recipe with its last step « Pour aller plus loin » when it names
 *  something to read next; as it is otherwise. */
export function withFurther(recipe: LoadedRecipe): LoadedRecipe {
  const next = (recipe as { next?: string[] }).next ?? []
  if (next.length === 0 || recipe.steps.some((s) => s.id === FURTHER_STEP)) return recipe
  return { ...recipe, steps: [...recipe.steps, { id: FURTHER_STEP, say: '' } as LoadedRecipe['steps'][number]] }
}

/** What an entry opens, the guide playing being `currentUri`. */
export function furtherTarget(entry: string, currentUri: string): FurtherTarget {
  const e = entry.trim()
  if (ADDRESS.test(e)) return { kind: 'address', address: e }
  if (e.includes(':/guide-recipe/')) return { kind: 'guide', uri: e }
  const provider = currentUri.slice(0, currentUri.indexOf(':'))
  return { kind: 'guide', uri: `${provider}:/guide-recipe/${e}` }
}

/** The entries shown: a document's address at once, a guide once it is
 *  loaded in this universe (`named` holds the loaded ones by entry) - a guide
 *  the universe lacks (its sidecar not a member) never shows. */
export function furtherShown(next: string[], named: Readonly<Record<string, string>>, currentUri: string): string[] {
  return next.filter((e) => furtherTarget(e, currentUri).kind === 'address' || e in named)
}
