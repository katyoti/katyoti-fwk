// SPDX-License-Identifier: Apache-2.0
/**
 * The Object tab's label — one rule for every way the tab is opened.
 *
 * A caller that knows the object's name passes it (the tree, a link); one that
 * only holds an id (a page navigating, the parent link) does not, and the tab
 * names itself once the object's detail is read. Kept free of the stores so it
 * is tested on its own.
 */

/** A translation function of the `workspace` namespace (i18n tFor). */
type TabT = (key: string, options?: Record<string, unknown>) => string

/** The tab label for an object title (none yet: the bare "Object"), in the
 *  language of `t` when given (the `workspace` namespace), else English. */
export function objectTabLabel(title?: string, t?: TabT): string {
  if (t) return title ? t('tab.objectTitled', { title }) : t('tab.object')
  return title ? `Object: ${title}` : 'Object'
}

/**
 * The payload the tab takes once the detail of `requestedId` is read with
 * `title` — or null when nothing changes: the tab moved on to another object
 * meanwhile, the read gave no title, or the tab already shows it.
 *
 * `ref` is the id the object is sent by (objectRef): opened bare
 * (a page row, the parent link) and answered with its project, the tab takes
 * the qualified id - one key per object whatever id opened it, so the tree
 * marks it and opening it again from the tree adds no step back.
 */
export function retitledObjectPayload<P extends { objectId: string; objectTitle?: string }>(
  current: P | undefined, requestedId: string, title: string | undefined, ref?: string,
): P | null {
  if (!current || current.objectId !== requestedId) return null
  const requalified = !!ref && ref !== current.objectId
  const retitled = !!title && current.objectTitle !== title
  if (!requalified && !retitled) return null
  return { ...current, ...(requalified ? { objectId: ref } : {}), ...(retitled ? { objectTitle: title } : {}) }
}
