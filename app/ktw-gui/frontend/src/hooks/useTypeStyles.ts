// SPDX-License-Identifier: Apache-2.0
// The declared look of each object type: the icon, colour and
// label a type declares, read once per universe from CWM's
// ObjectTypeList - so a project or a module is drawn as what it is, not by a
// hash of its type name. A type that declares none keeps the hashed look.

import { useEffect } from 'react'
import { create } from 'zustand'
import { dispatcherFor } from '../ktsp'
import type { DeclaredTypeStyle } from '../sections/workspace/type-style'

type Styles = Record<string, DeclaredTypeStyle>

const useStyles = create<{ byUniverse: Record<string, Styles>; asked: Record<string, true> }>(() => ({ byUniverse: {}, asked: {} }))

const NONE: Styles = {}

/** typeStylesOf turns ObjectTypeList's answer into the declared looks by type
 *  name (only the types that declare something). */
export function typeStylesOf(out: unknown): Styles {
  type Declared = { name?: string; icon?: string; color?: string; label?: string; description?: string; create_task?: string; label_plural?: string; child_order?: string[]; solution?: string; capabilities?: string[] }
  const list = (out as { object_types?: Declared[] } | undefined)?.object_types ?? []
  const styles: Styles = {}
  for (const t of list) {
    // The level comes from the capabilities its type carries.
    const caps = t?.capabilities ?? []
    const level = caps.includes('project') ? 'project' : caps.includes('module') ? 'module' : undefined
    if (!t?.name || (!t.icon && !t.color && !t.label && !level && !t.solution && !t.label_plural && !t.child_order?.length)) continue
    const style: DeclaredTypeStyle = { icon: t.icon || undefined, color: t.color || undefined, label: t.label || undefined }
    if (level) style.level = level
    if (t.description) style.description = t.description
    if (t.create_task) style.createTask = t.create_task
    if (t.solution) style.solution = t.solution
    if (t.label_plural) style.labelPlural = t.label_plural
    if (t.child_order?.length) style.childOrder = t.child_order
    styles[t.name] = style
  }
  return styles
}

/** The declared look of each type of the universe, read once; empty while it
 *  is read or when the universe serves none. */
export function useTypeStyles(universe: string | null | undefined): Styles {
  const styles = useStyles((s) => (universe ? s.byUniverse[universe] : undefined))
  useEffect(() => {
    if (!universe || useStyles.getState().asked[universe]) return
    useStyles.setState((s) => ({ asked: { ...s.asked, [universe]: true } }))
    void dispatcherFor(universe)('ktw-agents:cwm/object-type-list', {})
      .then((out) => useStyles.setState((s) => ({ byUniverse: { ...s.byUniverse, [universe]: typeStylesOf(out) } })))
      .catch(() => useStyles.setState((s) => ({ byUniverse: { ...s.byUniverse, [universe]: {} } })))
  }, [universe])
  return styles ?? NONE
}
