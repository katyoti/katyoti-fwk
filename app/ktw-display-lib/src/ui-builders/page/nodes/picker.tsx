// SPDX-License-Identifier: Apache-2.0
// Renderer of the page DSL's `picker` node: one generic
// picker - a search run on Enter, its matches, « Sélection » as a list or a
// tree, « Retirer », declared badges, a state per item, an unreachable item
// marked - driven only by the method URIs the node gives. It calls them
// through the host's callMethod: no code per connector or pack,
// and a surface without that service shows the selection only. Tickable, it
// is the picker of proposed items: each item ticked or
// unticked in place with the line saying why it was found, « Tout cocher »
// per group, the items already held checked and disabled, « Réessayer » on
// an unreachable source.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { ActionIcon, Alert, Badge, Box, Button, Checkbox, Group, Paper, Radio, ScrollArea, SimpleGrid, Stack, Text, TextInput, ThemeIcon, Tooltip } from '@mantine/core'
import { IconChevronDown, IconChevronRight, IconFolderOpen, IconLink, IconPlus, IconSearch, IconSparkles, IconX } from '@tabler/icons-react'

import { useHostServices, useSourceModel } from '../../../ui-core/host/index.tsx'
import type { UIPickerBadge, UIPickerItem, UIPickerNode, UIPickerScope, UIPickerStatus } from '../ui-page.interactive.ts'
import { SourceItem } from './source-item.tsx'
import type { RenderNode } from './layout.tsx'
import { tablerIcon } from '../../../ui-core/components/icons/byName.tsx'

// =================================================================================================
// The selection - pure, tested apart from rendering
// =================================================================================================

/** addToSelection adds an item once (by key); an unreachable one is never
 *  added. */
export function addToSelection(selection: UIPickerItem[], item: UIPickerItem): UIPickerItem[] {
  if (item.unreachable || selection.some((s) => s.key === item.key)) return selection
  return [...selection, item]
}

/** pickInto is what picking an item makes of the selection: added once in a
 *  multiple picker; in a single-choice one the item alone -
 *  the one choice held, replacing the one before, taken without what sits
 *  under it. An unreachable item is never picked. */
export function pickInto(selection: UIPickerItem[], item: UIPickerItem, choice?: 'single' | 'multiple'): UIPickerItem[] {
  if (choice !== 'single') return addToSelection(selection, item)
  if (item.unreachable) return selection
  const { with_children: _kids, ...one } = item
  return [one]
}

/** removeFromSelection removes an item and, in a tree, what sits under it. */
export function removeFromSelection(selection: UIPickerItem[], key: string): UIPickerItem[] {
  const gone = new Set([key])
  let grew = true
  while (grew) {
    grew = false
    for (const s of selection) {
      if (s.parent && gone.has(s.parent) && !gone.has(s.key)) {
        gone.add(s.key)
        grew = true
      }
    }
  }
  return selection.filter((s) => !gone.has(s.key))
}

/** A selection row as a tree draws it: the item and its depth. */
export interface PickerRow {
  item: UIPickerItem
  depth: number
}

/** selectionRows orders a selection for display: as given in a list; in a
 *  tree, each item under its parent when the parent is selected too. */
export function selectionRows(selection: UIPickerItem[], layout: 'list' | 'tree' = 'list'): PickerRow[] {
  if (layout === 'list') return selection.map((item) => ({ item, depth: 0 }))
  const keys = new Set(selection.map((s) => s.key))
  const children = new Map<string, UIPickerItem[]>()
  const roots: UIPickerItem[] = []
  for (const s of selection) {
    if (s.parent && keys.has(s.parent)) {
      children.set(s.parent, [...(children.get(s.parent) ?? []), s])
    } else {
      roots.push(s)
    }
  }
  const out: PickerRow[] = []
  const walk = (item: UIPickerItem, depth: number) => {
    out.push({ item, depth })
    for (const c of children.get(item.key) ?? []) walk(c, depth + 1)
  }
  for (const r of roots) walk(r, 0)
  return out
}

/** searchInput is what the search method is called with: the node's args,
 *  its object, the query trimmed - and the keys of the scopes ticked, when
 *  the picker shows its scopes. */
export function searchInput(node: { args?: Record<string, unknown>; object?: string }, query: string, scopes?: string[]): Record<string, unknown> {
  return { ...(node.args ?? {}), ...(node.object ? { object: node.object } : {}), query: query.trim(), ...(scopes ? { scopes } : {}) }
}

/** scopesInput is what the scopes method is called with: the node's args
 *  and its object. */
export function scopesInput(node: { args?: Record<string, unknown>; object?: string }): Record<string, unknown> {
  return { ...(node.args ?? {}), ...(node.object ? { object: node.object } : {}) }
}

/** addScope adds a scope the carrier types, once (by key); a blank one is
 *  never added. */
export function addScope(scopes: UIPickerScope[], key: string): UIPickerScope[] {
  const k = key.trim()
  if (!k || scopes.some((s) => s.key === k)) return scopes
  return [...scopes, { key: k, title: k }]
}

/** toggleScope ticks a scope for the search, or unticks it. */
export function toggleScope(ticked: ReadonlySet<string>, key: string): Set<string> {
  const next = new Set(ticked)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  return next
}

/** tickedScopes is the keys searched in, in the scopes' order. */
export function tickedScopes(scopes: UIPickerScope[], ticked: ReadonlySet<string>): string[] {
  return scopes.filter((s) => ticked.has(s.key)).map((s) => s.key)
}

/** scopeSearch is the scopes a search sends: none where
 *  the picker shows no scope (the source's default ones), the ticked keys
 *  otherwise - and null when scopes show and none is ticked: that search is
 *  refused, never sent with `scopes: []`, which the source reads as its
 *  default ones while the screen shows nothing ticked. */
export function scopeSearch(scopes: UIPickerScope[] | null, ticked: ReadonlySet<string>): string[] | undefined | null {
  if (!scopes || scopes.length === 0) return undefined
  const keys = tickedScopes(scopes, ticked)
  return keys.length ? keys : null
}

/** withoutScope drops a scope and its tick - an added one whose save was
 *  refused. */
export function withoutScope(scopes: UIPickerScope[], ticked: ReadonlySet<string>, key: string): { scopes: UIPickerScope[]; ticked: Set<string> } {
  const next = new Set(ticked)
  next.delete(key)
  return { scopes: scopes.filter((s) => s.key !== key), ticked: next }
}

/** loadScopes asks the scopes method and ticks them all; an empty list when
 *  it fails. */
export async function loadScopes(call: (uri: string, input: Record<string, unknown>) => Promise<unknown>, uri: string,
  node: { args?: Record<string, unknown>; object?: string }): Promise<{ scopes: UIPickerScope[]; ticked: Set<string> }> {
  try {
    const out = await call(uri, scopesInput(node))
    const got = ((out as { scopes?: UIPickerScope[] } | undefined)?.scopes ?? []).filter((x) => x && x.key)
    return { scopes: got, ticked: new Set(got.map((x) => x.key)) }
  } catch {
    return { scopes: [], ticked: new Set() }
  }
}

/** saveScope keeps an added scope for the carrier through the save method:
 *  undefined when kept, why when refused - thrown, or answered `ok: false`.
 */
export async function saveScope(call: (uri: string, input: Record<string, unknown>) => Promise<unknown>, uri: string,
  node: { args?: Record<string, unknown>; object?: string }, key: string): Promise<string | undefined> {
  try {
    const out = (await call(uri, { ...scopesInput(node), scope: key })) as { ok?: boolean; message?: string } | undefined
    return out?.ok === false ? (out.message || 'refused') : undefined
  } catch (e) {
    return String(e)
  }
}

/** browseInput is what the browse method is called with: the node's args,
 *  its object, the parent listed (none: the top level). */
export function browseInput(node: { args?: Record<string, unknown>; object?: string }, parent?: string): Record<string, unknown> {
  return { ...(node.args ?? {}), ...(node.object ? { object: node.object } : {}), ...(parent ? { parent } : {}) }
}

/** badgeOf is a badge as the node declares it, its name when undeclared. */
export function badgeOf(badges: Record<string, UIPickerBadge> | undefined, name: string): UIPickerBadge {
  return badges?.[name] ?? { label: name }
}

/** The labels the picker shows, the node's own when it gives them. */
export const PICKER_LABELS = {
  search: 'Rechercher… (Entrée)',
  find: 'Chercher',
  expand: 'Déplier',
  collapse: 'Replier',
  selection: 'Sélection',
  remove: 'Retirer',
  add: 'Ajouter',
  choose: 'Choisir',
  apply: 'Appliquer',
  none: 'Aucun résultat',
  empty: 'Rien de sélectionné',
  unreachable: 'inaccessible',
  noHost: 'La recherche n’est pas disponible ici',
  browse: 'Parcourir',
  top: 'Tout',
  withChildren: 'avec les sous-éléments',
  // A tree proposal row's count of what sits under it ({n}).
  belowOne: 'avec 1 sous-élément',
  belowMany: 'avec {n} sous-éléments',
  // The check-box tree: a host names its unit
  // (« avec les sous-pages », « cette page seule ») through the node's labels.
  self: 'cet élément seul',
  included: 'inclus',
  treeHelp: 'Comment se prennent les sous-éléments',
  treeHelpWith: 'Cocher un élément le prend avec ses sous-éléments, aussi ceux ajoutés plus tard. « cet élément seul » ne prend que lui.',
  treeHelpIncluded: 'Sous-élément inclus par son parent. Le décocher fait passer le parent à « cet élément seul » ; ses autres sous-éléments restent pris.',
  treeHelpMixed: 'L’élément n’est pas pris, mais des éléments en dessous le sont.',
  treeNotice: '« {removed} » retiré : « {parent} » passe à « cet élément seul », ses autres sous-éléments restent pris.',
  notConfigured: 'n’est pas configurée',
  notReachable: 'est injoignable : la sélection reste affichée, la recherche reprendra quand elle répondra',
  searchTitle: 'Rechercher et ajouter',
  removeAll: 'Retirer les',
  of: 'dont',
  read: 'lu',
  waiting: 'en attente',
  removed: 'retiré',
  failed: 'en échec',
  tickAll: 'Tout cocher',
  untickAll: 'Tout décocher',
  kept: 'retenus',
  held: 'déjà retenue',
  retry: 'Réessayer',
  untickBadge: 'Décocher les',
  scopes: 'Chercher dans',
  addScope: 'Ajouter',
  scopePlaceholder: 'Clé à ajouter (Entrée)',
  defaultScope: 'par défaut',
  noScope: 'Cochez au moins un périmètre où chercher',
  // The proposal region and its save bar.
  results: 'Résultats',
  alreadyProposed: 'déjà proposée',
  noneChosen: 'Rien de choisi.',
  cancel: 'Annuler',
  saveRead: 'Enregistrer et lire',
  // « k sur n » on a proposal group's head.
  outOf: 'sur',
}

/** withBadge is the items of a selection carrying a badge. */
export function withBadge(selection: UIPickerItem[], badge: string): UIPickerItem[] {
  return selection.filter((s) => (s.badges ?? []).includes(badge))
}

/** liveQuery says whether a live picker searches what is typed: two
 *  letters at least. */
export function liveQuery(query: string): boolean {
  return query.trim().length >= 2
}

/** searchesWhileTyping says whether the picker searches as it is typed in:
 *  only a live one on a host not showing the source model - with it, a
 *  search runs on Enter or the field's search icon, never while typing.
 */
export function searchesWhileTyping(node: { live?: boolean }, sourceModel: boolean): boolean {
  return !!node.live && !sourceModel
}

/** pickerFinds says how a picker finds items: by a search
 *  where its node names one, by « Parcourir » where it names a browse; a
 *  node with neither only holds its selection - what a proposal kept. */
export function pickerFinds(node: { search?: string; browse?: string }): { search: boolean; browse: boolean } {
  return { search: !!node.search, browse: !!node.browse }
}

/** A row of the search hits nested by their path: a hit, or
 *  a place above hits that is no hit itself - its title only. */
export type MatchRow =
  | { kind: 'item'; item: UIPickerItem; depth: number }
  | { kind: 'place'; id: string; title: string; depth: number }

/** matchRows nests search hits in a tree by their path - the titles above
 *  each: hits sharing a place sit under one row of it, a hit
 *  that is itself a place of another hit's path holds it, in the order the
 *  hits came. Hits with no path stay at the top. */
export function matchRows(matches: UIPickerItem[]): MatchRow[] {
  interface Place { id: string; title: string; hit?: UIPickerItem; children: Place[] }
  const root: Place = { id: '', title: '', children: [] }
  const child = (at: Place, title: string): Place => {
    let c = at.children.find((p) => p.title === title)
    if (!c) {
      c = { id: `${at.id}/${title}`, title, children: [] }
      at.children.push(c)
    }
    return c
  }
  for (const m of matches) {
    let at = root
    for (const title of m.path ?? []) at = child(at, title)
    const own = child(at, m.title)
    if (own.hit && own.hit.key !== m.key) {
      // Two hits of one title at one place: each its own row.
      at.children.push({ id: `${at.id}/${m.title}#${m.key}`, title: m.title, hit: m, children: [] })
    } else {
      own.hit = m
    }
  }
  const out: MatchRow[] = []
  const walk = (p: Place, depth: number) => {
    for (const c of p.children) {
      out.push(c.hit ? { kind: 'item', item: c.hit, depth } : { kind: 'place', id: c.id, title: c.title, depth })
      walk(c, depth + 1)
    }
  }
  walk(root, 0)
  return out
}

/** browseRows draws a browsed source as an expandable tree:
 *  the top level, and under each expanded item the children its browse
 *  answered - each child knowing the parent it was listed under. */
export function browseRows(top: UIPickerItem[], childrenOf: Record<string, UIPickerItem[]>, expanded: ReadonlySet<string>): PickerRow[] {
  const out: PickerRow[] = []
  const seen = new Set<string>()
  const walk = (items: UIPickerItem[], depth: number, parent?: string) => {
    for (const m of items) {
      if (seen.has(m.key)) continue
      seen.add(m.key)
      const item = parent && !m.parent ? { ...m, parent } : m
      out.push({ item, depth })
      if (expanded.has(m.key)) walk(childrenOf[m.key] ?? [], depth + 1, m.key)
    }
  }
  walk(top, 0)
  return out
}

/** browseTree lists the top of « Parcourir » and opens the
 *  top items the connector flagged open - its default scope, Confluence's
 *  default space - their children listed at once. The top
 *  is given to onTop as soon as it is read, before the flagged children (a
 *  large space's pages take a while): the tree draws at once, the default
 *  space unfolding when its pages come. A flagged item whose browse fails
 *  stays closed, its error in failed. */
export async function browseTree(browse: (parent?: string) => Promise<UIPickerItem[]>, onTop?: (top: UIPickerItem[], opening: string[]) => void): Promise<{
  top: UIPickerItem[]; childrenOf: Record<string, UIPickerItem[]>; expanded: Set<string>; failed: Record<string, string>
}> {
  const top = await browse()
  const childrenOf: Record<string, UIPickerItem[]> = {}
  const expanded = new Set<string>()
  const failed: Record<string, string> = {}
  const flagged = top.filter((m) => m.open && m.has_children)
  onTop?.(top, flagged.map((m) => m.key))
  await Promise.all(flagged.map(async (m) => {
    try {
      childrenOf[m.key] = await browse(m.key)
      expanded.add(m.key)
    } catch (e) { failed[m.key] = String(e) /* left closed: a click browses it again */ }
  }))
  return { top, childrenOf, expanded, failed }
}

/** openedAtOnce is the tree's expanded set with the items flagged open
 *  added - drawn opened on the tree's first draw, before their children come
 *  - and those whose browse failed closed again. */
export function openedAtOnce(was: ReadonlySet<string>, open: string[], failed: string[] = []): Set<string> {
  const next = new Set([...was, ...open])
  for (const k of failed) next.delete(k)
  return next
}

/** statusBanner is what a picker says of its source, or nothing when it is
 *  configured and reachable. */
export function statusBanner(status: UIPickerStatus | undefined, labels: { notConfigured: string; notReachable: string }): { tone: 'red' | 'yellow'; text: string } | undefined {
  if (!status) return undefined
  if (status.configured === false) return { tone: 'yellow', text: `${labels.notConfigured}${status.message ? ` - ${status.message}` : ''}` }
  if (status.reachable === false) return { tone: 'red', text: `${labels.notReachable}${status.message ? ` (${status.message})` : ''}` }
  return undefined
}

/** withChildrenOf marks an item picked with what sits under it - only an
 *  item that has some. */
export function withChildrenOf(item: UIPickerItem, on: boolean): UIPickerItem {
  return on && item.has_children ? { ...item, with_children: true } : item
}

// =================================================================================================
// The check-box tree (mock-up v6 « Pages prises »): a tree
// picker's browsed items ticked in place - a ticked item taken with what sits
// under it (also what is added later) or « cet élément seul », an item under
// one taken with its children shown included, one with taken items below it
// mixed.
// =================================================================================================

/** What the browsed tree knows: each item by key and the parent each was
 *  listed under. */
export interface BrowsedTree {
  items: Map<string, UIPickerItem>
  parents: Map<string, string>
  childrenOf: Record<string, UIPickerItem[]>
}

/** browsedTree indexes « Parcourir »'s top and the children listed so far,
 *  then the tree's proposals (`proposed`) not browsed yet - each under its
 *  `parent`, so a proposal shows included by a taken ancestor. */
export function browsedTree(top: UIPickerItem[], childrenOf: Record<string, UIPickerItem[]>, proposed: UIPickerItem[] = []): BrowsedTree {
  const items = new Map<string, UIPickerItem>()
  const parents = new Map<string, string>()
  for (const m of top) items.set(m.key, m)
  for (const [parent, kids] of Object.entries(childrenOf)) {
    for (const m of kids) {
      items.set(m.key, m.parent ? m : { ...m, parent })
      parents.set(m.key, parent)
    }
  }
  for (const m of proposed) {
    // A proposal with items under it is taken with them.
    if (!items.has(m.key)) items.set(m.key, (m.below ?? 0) > 0 && m.has_children === undefined ? { ...m, has_children: true } : m)
    if (m.parent && !parents.has(m.key)) parents.set(m.key, m.parent)
  }
  return { items, parents, childrenOf }
}

/** How an item of the tree shows: taken `with` what sits under it, taken
 *  `self` alone, `included` by an ancestor taken with its children, `mixed`
 *  (not taken, items under it are), or `none`. */
export type TreeMark = 'with' | 'self' | 'included' | 'mixed' | 'none'

function parentOf(tree: BrowsedTree, selection: UIPickerItem[], key: string): string | undefined {
  return tree.parents.get(key) ?? tree.items.get(key)?.parent ?? selection.find((s) => s.key === key)?.parent
}

/** ancestorsOf lists an item's ancestors, the nearest first. */
function ancestorsOf(tree: BrowsedTree, selection: UIPickerItem[], key: string): string[] {
  const out: string[] = []
  const seen = new Set([key])
  for (let p = parentOf(tree, selection, key); p && !seen.has(p); p = parentOf(tree, selection, p)) {
    out.push(p)
    seen.add(p)
  }
  return out
}

/** includer is the nearest ancestor taken with its children, if any. */
function includer(tree: BrowsedTree, selection: UIPickerItem[], key: string): string | undefined {
  const taken = new Map(selection.map((s) => [s.key, s]))
  return ancestorsOf(tree, selection, key).find((a) => taken.get(a)?.with_children)
}

/** treeMark is how an item of the tree shows (see TreeMark). */
export function treeMark(selection: UIPickerItem[], key: string, tree: BrowsedTree): TreeMark {
  const s = selection.find((x) => x.key === key)
  if (s) return s.with_children ? 'with' : 'self'
  if (includer(tree, selection, key)) return 'included'
  if (selection.some((x) => ancestorsOf(tree, selection, x.key).includes(key))) return 'mixed'
  return 'none'
}

/** A tick's notice: an included item unticked - its includer now taken
 *  alone, its other children still taken. */
export interface TreeNotice { removed: string; parent: string }

function placed(tree: BrowsedTree, selection: UIPickerItem[], key: string): UIPickerItem {
  const m = tree.items.get(key) ?? selection.find((s) => s.key === key) ?? { key, title: key }
  const parent = parentOf(tree, selection, key)
  return parent && !m.parent ? { ...m, parent } : m
}

function withoutBelow(tree: BrowsedTree, selection: UIPickerItem[], key: string): UIPickerItem[] {
  return selection.filter((s) => !ancestorsOf(tree, selection, s.key).includes(key))
}

/** tickTree is what ticking an item of the tree makes of the selection
 *  (mock-up v6): a taken item is let go; an item included by an ancestor is
 *  let go by taking that ancestor and each item down to it alone, their
 *  other children kept with theirs (a notice says so); any other is taken
 *  with what sits under it - what was taken under it now included. */
export function tickTree(selection: UIPickerItem[], key: string, tree: BrowsedTree): { selection: UIPickerItem[]; notice?: TreeNotice } {
  if (selection.some((s) => s.key === key)) return { selection: selection.filter((s) => s.key !== key) }
  const a = includer(tree, selection, key)
  if (!a) {
    const kept = withoutBelow(tree, selection, key)
    return { selection: [...kept, withChildrenOf(placed(tree, selection, key), true)] }
  }
  // The path from the item up to (not counting) its includer.
  const path: string[] = [key]
  for (const p of ancestorsOf(tree, selection, key)) {
    if (p === a) break
    path.push(p)
  }
  let next = selection.map((s) => (s.key === a ? alone(s) : s))
  for (const p of path.slice(1)) next = [...next.filter((s) => s.key !== p), alone(placed(tree, selection, p))]
  for (const p of path) {
    const parent = parentOf(tree, selection, p)
    for (const sib of (parent ? tree.childrenOf[parent] : undefined) ?? []) {
      if (sib.key === p || path.includes(sib.key) || next.some((s) => s.key === sib.key)) continue
      next.push(withChildrenOf(placed(tree, selection, sib.key), true))
    }
  }
  const title = (k: string) => tree.items.get(k)?.title ?? selection.find((s) => s.key === k)?.title ?? k
  return { selection: next, notice: { removed: title(key), parent: title(a) } }
}

/** tickAllTree is a tree's « Tout cocher »: each proposal neither taken nor
 *  included is ticked as the tree ticks it, in order - one taken with what
 *  sits under it includes the proposals below it, which are then skipped. */
export function tickAllTree(selection: UIPickerItem[], items: UIPickerItem[], tree: BrowsedTree): UIPickerItem[] {
  let next = selection
  for (const m of items) {
    const mark = treeMark(next, m.key, tree)
    if (mark === 'none' || mark === 'mixed') next = tickTree(next, m.key, tree).selection
  }
  return next
}

/** treeRowNote is the words after a tree proposal's title: the `self` label
 *  when it is taken alone, else « avec n sous-éléments » when items sit
 *  under it, else none. */
export function treeRowNote(mark: TreeMark, below: number | undefined, labels: { self: string; belowOne: string; belowMany: string }): string {
  if (mark === 'self') return labels.self
  if (!below || below < 1) return ''
  return below === 1 ? labels.belowOne.replace('{n}', '1') : labels.belowMany.replace('{n}', String(below))
}

function alone(item: UIPickerItem): UIPickerItem {
  const { with_children: _w, ...rest } = item
  return rest
}

/** setTreeMode takes a taken item `with` what sits under it - what was taken
 *  under it now included - or `self`, alone. */
export function setTreeMode(selection: UIPickerItem[], key: string, mode: 'with' | 'self', tree: BrowsedTree): UIPickerItem[] {
  if (!selection.some((s) => s.key === key)) return selection
  if (mode === 'self') return selection.map((s) => (s.key === key ? alone(s) : s))
  return withoutBelow(tree, selection, key).map((s) => (s.key === key ? withChildrenOf({ ...s, has_children: s.has_children ?? true }, true) : s))
}

/** toggleItem ticks or unticks one item of a tickable selection: an
 *  unticked one stays, `off`. A tree is ticked as a whole:
 *  unticking an item unticks what sits under it and
 *  takes `with_children` off what it sits under (its items are no longer
 *  all wanted); ticking one ticks what it sits under - so what is handed
 *  back never holds an item without its parent. `to` sets the tick instead
 *  of flipping it (a row shows unticked what an unticked ancestor leaves
 *  out,: ticking it ticks that ancestor back). */
export function toggleItem(selection: UIPickerItem[], key: string, to?: boolean): UIPickerItem[] {
  const item = selection.find((s) => s.key === key)
  if (!item) return selection
  const on = to ?? !!item.off
  const byKey = new Map(selection.map((s) => [s.key, s]))
  const above = new Set<string>()
  for (let p = item.parent; p && byKey.has(p) && !above.has(p); p = byKey.get(p)!.parent) above.add(p)
  const below = new Set<string>()
  let grew = true
  while (grew) {
    grew = false
    for (const s of selection) {
      if (s.parent && (s.parent === key || below.has(s.parent)) && !below.has(s.key) && s.key !== key) {
        below.add(s.key)
        grew = true
      }
    }
  }
  return selection.map((s) => {
    if (s.key === key) return tick(s, on)
    if (on) return above.has(s.key) ? tick(s, true) : s
    if (below.has(s.key)) return tick(s, false)
    if (above.has(s.key) && s.with_children) {
      const { with_children: _w, ...rest } = s
      return rest
    }
    return s
  })
}

/** untickBadge unticks the items of a tickable selection carrying a badge
 *  - kept, struck through, never removed. */
export function untickBadge(selection: UIPickerItem[], badge: string): UIPickerItem[] {
  return selection.map((s) => ((s.badges ?? []).includes(badge) ? tick(s, false) : s))
}

/** changeApplier hands a picker's selection to its apply method as it
 *  changes: after `delay` ms of calm, one call at a time - a
 *  change made during a call is sent once it answers, so the last state is
 *  always the one held. */
export function changeApplier(send: (input: Record<string, unknown>) => Promise<unknown>, delay = 400): {
  change: (input: Record<string, unknown>) => void
  cancel: () => void
} {
  let timer: ReturnType<typeof setTimeout> | undefined
  let running = false
  let pending: Record<string, unknown> | undefined
  const flush = async (): Promise<void> => {
    if (running || pending === undefined) return
    const input = pending
    pending = undefined
    running = true
    try {
      await send(input)
    } catch {
      // the answer's message says it; the next change tries again
    } finally {
      running = false
      if (pending !== undefined) void flush()
    }
  }
  return {
    change: (input) => {
      pending = input
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => { void flush() }, delay)
    },
    cancel: () => { if (timer) clearTimeout(timer) },
  }
}

/** applyInput is what the apply method is called with: the node's args, its
 *  object and the items handed back - in a tickable picker the ticked ones
 *  only. */
export function applyInput(node: { args?: Record<string, unknown>; object?: string; tickable?: boolean }, selection: UIPickerItem[], by?: { id: string; kind: string }): Record<string, unknown> {
  const handed = node.tickable ? tickedItems(selection) : selection
  return {
    ...(node.args ?? {}),
    ...(node.object ? { object: node.object } : {}),
    // Who applies: the host's person, when it has one.
    ...(by && by.id ? { by } : {}),
    items: handed.map((s) => ({ key: s.key, title: s.title, ...(s.parent ? { parent: s.parent } : {}), ...(s.with_children ? { with_children: true } : {}) })),
  }
}

/** tickGroup ticks (or unticks) every item of one group ("" the items in
 *  none). */
export function tickGroup(selection: UIPickerItem[], group: string, on: boolean): UIPickerItem[] {
  if (!on) return selection.map((s) => ((s.group ?? '') === group ? tick(s, false) : s))
  // Ticked as one by one, so what they sit under is ticked back - even in
  // another group - and the group is handed back whole.
  return selection.filter((s) => (s.group ?? '') === group).reduce((acc, s) => toggleItem(acc, s.key, true), selection)
}

function tick(item: UIPickerItem, on: boolean): UIPickerItem {
  if (!on) return { ...item, off: true }
  const { off: _off, ...rest } = item
  return rest
}

/** tickedItems is what a tickable selection hands back: the items not
 *  unticked. A tree is enforced here, whatever moved the ticks (an item, a
 *  group's « Tout cocher », a badge): an item goes back
 *  only when every ancestor present in the selection is ticked, and keeps
 *  `with_children` only when every descendant present is ticked. A parent
 *  absent from the selection is not the picker's to judge. */
export function tickedItems(selection: UIPickerItem[]): UIPickerItem[] {
  const byKey = new Map(selection.map((s) => [s.key, s]))
  const ancestorsOn = (s: UIPickerItem): boolean => {
    const seen = new Set<string>([s.key])
    for (let p = s.parent; p && byKey.has(p) && !seen.has(p); p = byKey.get(p)!.parent) {
      if (byKey.get(p)!.off) return false
      seen.add(p)
    }
    return true
  }
  const handed = selection.filter((s) => !s.off && ancestorsOn(s))
  const offBelow = new Set<string>()
  for (const s of selection) {
    if (!s.off) continue
    const seen = new Set<string>([s.key])
    for (let p = s.parent; p && byKey.has(p) && !seen.has(p); p = byKey.get(p)!.parent) {
      offBelow.add(p)
      seen.add(p)
    }
  }
  return handed.map((s) => {
    if (!s.with_children || !offBelow.has(s.key)) return s
    const { with_children: _w, ...rest } = s
    return rest
  })
}

/** A group of a tickable selection: its name, its items, how many are
 *  ticked. */
export interface PickerGroup {
  group: string
  items: UIPickerItem[]
  ticked: number
}

/** pickerGroups splits a selection by group, in the order the groups first
 *  appear; the items in no group are one group "". Its counts are what is
 *  handed back (tickedItems): an item an unticked ancestor leaves out is
 *  not counted. */
export function pickerGroups(selection: UIPickerItem[]): PickerGroup[] {
  const sent = new Set(tickedItems(selection).map((s) => s.key))
  const out: PickerGroup[] = []
  for (const s of selection) {
    const name = s.group ?? ''
    let g = out.find((o) => o.group === name)
    if (!g) {
      g = { group: name, items: [], ticked: 0 }
      out.push(g)
    }
    g.items.push(s)
    if (sent.has(s.key)) g.ticked++
  }
  return out
}

/** The tone of an item's state. */
const STATE_COLOR: Record<string, string> = { read: 'green', waiting: 'yellow', removed: 'gray', failed: 'red' }

// =================================================================================================
// The renderer
// =================================================================================================

/** PickerBanner says where the picker's source stands; an
 *  unreachable source offers « Réessayer » when the status can be asked
 *  again. */
export function PickerBanner({ banner, status, labels, retrying, onRetry }: {
  banner: { tone: 'red' | 'yellow'; text: string }
  status: UIPickerStatus
  labels: typeof PICKER_LABELS
  retrying?: boolean
  onRetry?: () => void
}) {
  return (
    <Alert color={banner.tone} variant="light" p="xs" data-guide="picker.banner">
      <Group gap="xs" justify="space-between" wrap="nowrap">
        <Text size="xs">{banner.text}</Text>
        {status.configure && (
          <Tooltip label={status.configure.hint ?? ''} disabled={!status.configure.hint} multiline w={280}>
            <Text size="xs" fw={600} data-guide="picker.configure">{status.configure.label}</Text>
          </Tooltip>
        )}
        {status.reachable === false && onRetry && (
          <Button size="compact-xs" variant="light" color="red" loading={retrying} onClick={onRetry} data-guide="picker.retry">
            {labels.retry}
          </Button>
        )}
      </Group>
    </Alert>
  )
}

/** PickerAddControl starts a listed item's row, beside its name so a long
 *  or wide list never leaves it apart: an item already held is a
 *  checked, disabled box; any other, « Ajouter » - disabled
 *  when it is chosen, unreachable or the picker is. */
export function PickerAddControl({ held, off, labels, onAdd, single, chosen }: {
  held: boolean
  off: boolean
  labels: typeof PICKER_LABELS
  onAdd: () => void
  single?: boolean
  chosen?: boolean
}) {
  if (held) return <Checkbox size="xs" checked disabled readOnly aria-label={labels.held} data-guide="picker.held-box" />
  // A single-choice picker: « Choisir », the chosen one checked.
  if (single) return <Radio size="xs" checked={!!chosen} disabled={off && !chosen} aria-label={labels.choose} onChange={onAdd} data-guide="picker.choose" />
  return (
    <ActionIcon size="sm" variant="subtle" disabled={off} aria-label={labels.add} onClick={onAdd}>
      <IconPlus size={14} />
    </ActionIcon>
  )
}

/** PickerLine is one found item's line - a search hit, a browsed level's or
 *  tree's item: its « + » (or held box, or choice) first, then its name and
 *  badges, so the « + » sits beside the name it adds in a long or wide list.
 */
export function PickerLine({ add, depth = 0, guide, name, expanded, children }: {
  add: ReactNode
  depth?: number
  guide: string
  name: string
  expanded?: boolean
  children: ReactNode
}) {
  return (
    <Group gap="xs" wrap="nowrap" pl={depth * 16} data-guide={guide} data-guide-name={name} data-expanded={expanded ? 'true' : undefined}>
      {add}
      {children}
    </Group>
  )
}

/** TreeHelp is the check-box tree's « Comment se prennent les sous-éléments »
 *  (mock-up v6's treeHelp): the three marks, folded. */
export function TreeHelp({ labels }: { labels: typeof PICKER_LABELS }) {
  return (
    <details data-guide="picker.tree-help">
      <summary style={{ cursor: 'pointer' }}><Text span size="xs" c="dimmed">{labels.treeHelp}</Text></summary>
      <Stack gap={4} mt={4}>
        <Group gap="xs" wrap="nowrap"><Checkbox size="xs" checked readOnly aria-hidden /><Text size="xs">{labels.treeHelpWith}</Text></Group>
        <Group gap="xs" wrap="nowrap"><Checkbox size="xs" checked readOnly color="gray" aria-hidden /><Text size="xs">{labels.treeHelpIncluded}</Text></Group>
        <Group gap="xs" wrap="nowrap"><Checkbox size="xs" indeterminate readOnly aria-hidden /><Text size="xs">{labels.treeHelpMixed}</Text></Group>
      </Stack>
    </details>
  )
}

/** TreeModeButtons switches a taken item of the check-box tree between
 *  « avec les sous-éléments » and « cet élément seul ». */
export function TreeModeButtons({ mode, labels, disabled, onMode }: {
  mode: 'with' | 'self'
  labels: typeof PICKER_LABELS
  disabled?: boolean
  onMode: (to: 'with' | 'self') => void
}) {
  return (
    <Group gap={2} wrap="nowrap" data-guide="picker.tree-mode">
      {(['with', 'self'] as const).map((m) => (
        <Button key={m} size="compact-xs" variant={mode === m ? 'light' : 'subtle'} color={mode === m ? 'blue' : 'gray'}
          aria-pressed={mode === m} disabled={disabled} onClick={() => onMode(m)} data-mode={m}>
          {m === 'with' ? labels.withChildren : labels.self}
        </Button>
      ))}
    </Group>
  )
}

/** ScopeChips draws a picker's scopes: one chip per scope,
 *  filled while ticked for the search, the default ones saying so, and «
 *  + Ajouter » opening a field where Enter adds one for the carrier. */
export function ScopeChips({ scopes, ticked, unit, add, adding, labels, disabled, onToggle, onAdding, onAdd }: {
  scopes: UIPickerScope[]
  ticked: ReadonlySet<string>
  unit?: string
  add: boolean
  adding: string | null
  labels: typeof PICKER_LABELS
  disabled?: boolean
  onToggle: (key: string) => void
  onAdding: (text: string | null) => void
  onAdd: (key: string) => void
}) {
  return (
    <Group gap={6} data-guide="picker.scopes">
      <Text size="xs" c="dimmed">{labels.scopes}{unit ? ` (${unit})` : ''}</Text>
      {scopes.map((sc) => {
        const on = ticked.has(sc.key)
        return (
          <Tooltip key={sc.key} label={sc.default ? labels.defaultScope : sc.key}>
            <Badge size="sm" radius="sm" variant={on ? 'filled' : 'outline'} color={on ? 'blue' : 'gray'}
              style={{ cursor: disabled ? undefined : 'pointer' }} data-guide={`picker.scope.${sc.key}`}
              data-default={sc.default ? 'true' : undefined} role="button" tabIndex={disabled ? -1 : 0}
              aria-pressed={on} aria-disabled={disabled || undefined}
              onClick={disabled ? undefined : () => onToggle(sc.key)}
              onKeyDown={disabled ? undefined : (e) => {
                if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onToggle(sc.key) }
              }}>
              {sc.title || sc.key}
            </Badge>
          </Tooltip>
        )
      })}
      {add && (adding === null ? (
        <Button size="compact-xs" variant="subtle" leftSection={<IconPlus size={12} />} disabled={disabled}
          onClick={() => onAdding('')} data-guide="picker.scope-add">
          {labels.addScope}
        </Button>
      ) : (
        <TextInput size="xs" w={180} autoFocus placeholder={labels.scopePlaceholder} value={adding}
          onChange={(e) => onAdding(e.currentTarget.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') onAdd(adding); if (e.key === 'Escape') onAdding(null) }}
          data-guide="picker.scope-input" />
      ))}
      {scopes.length > 0 && !scopes.some((sc) => ticked.has(sc.key)) && (
        <Text size="xs" c="red" data-guide="picker.scope-none">{labels.noScope}</Text>
      )}
    </Group>
  )
}

/** TreeProposals is a check-box tree's proposal region (mock-up v6 « Pages
 *  prises »): a card titled `proposals.title` with the producer's `sub` node
 *  and « Tout cocher » while one is neither taken nor included, then a row
 *  per proposal - the tree's check box (on when taken, grey when included),
 *  the title and « cet élément seul » or « avec n sous-éléments », the path
 *  « / » joined « · » why, the pill at the right -, or `none`. */
export function TreeProposals({ node, selection, tree, labels, disabled, render, onTick, onTickAll }: {
  node: Omit<UIPickerNode, 'kind'>
  selection: UIPickerItem[]
  tree: BrowsedTree
  labels: typeof PICKER_LABELS
  disabled?: boolean
  render?: RenderNode
  onTick: (key: string) => void
  onTickAll: () => void
}): ReactNode {
  const decl = node.proposals!
  const items = decl.items ?? []
  const marks = items.map((m) => treeMark(selection, m.key, tree))
  const open = marks.some((k) => k === 'none' || k === 'mixed')
  return (
    <Paper withBorder data-guide="picker.tree-proposals">
      <Group gap="sm" wrap="nowrap" align="flex-start" p="sm" style={{ borderBottom: '1px solid var(--mantine-color-default-border)' }}>
        <ThemeIcon size={30} variant="light" radius="md"><IconSparkles size={16} /></ThemeIcon>
        <Box style={{ flex: 1, minWidth: 0 }}>
          <Text size="sm" fw={600}>{decl.title}</Text>
          {decl.sub && render && <Group gap={4} mt={4}>{render(decl.sub, 'sub')}</Group>}
        </Box>
        {open && (
          <Button size="compact-xs" variant="subtle" disabled={disabled} data-guide="picker.tick-all" onClick={onTickAll}>{labels.tickAll}</Button>
        )}
      </Group>
      {items.length === 0
        ? <Text size="sm" c="dimmed" p="xs" data-guide="picker.proposals-none">{decl.none ?? labels.none}</Text>
        : items.map((m, i) => {
          const mark = marks[i]
          const note = treeRowNote(mark, m.below, labels)
          const where = (m.path ?? []).join(' / ')
          const Pill = m.pill?.icon ? tablerIcon(m.pill.icon, IconLink) : undefined
          const off = disabled || !!m.unreachable
          return (
            <Group key={m.key} gap="xs" wrap="nowrap" align="flex-start" px={8} py={6} data-guide="picker.tree-proposal" data-guide-name={m.key}
              data-mark={mark} style={{ cursor: off ? undefined : 'pointer' }} onClick={() => { if (!off) onTick(m.key) }}>
              <Checkbox size="xs" mt={3} checked={mark !== 'none' && mark !== 'mixed'} indeterminate={mark === 'mixed'}
                color={mark === 'included' ? 'gray' : undefined} disabled={off} aria-label={m.title} data-guide="picker.tree-tick" data-mark={mark}
                onClick={(e) => e.stopPropagation()} onChange={() => onTick(m.key)} />
              <Box style={{ flex: 1, minWidth: 0 }}>
                <Text size="sm">
                  <Text span size="sm" fw={500}>{m.title}</Text>
                  {note && <Text span size="xs" c="dimmed" data-guide="picker.tree-proposal-note">{` ${note}`}</Text>}
                </Text>
                {(where || m.why) && (
                  <Text size="xs" c="dimmed">
                    {where && <span data-guide="picker.tree-proposal-path">{where}</span>}{where && m.why ? ' · ' : ''}
                    {m.why && <Text span size="xs" c="blue" data-guide="picker.why">{m.why}</Text>}
                  </Text>
                )}
              </Box>
              {m.pill && (
                <Badge size="sm" variant="light" color={m.pill.color ?? 'gray'} tt="none" data-guide="picker.pill"
                  leftSection={Pill ? <Pill size={11} /> : undefined}>{m.pill.label}</Badge>
              )}
            </Group>
          )
        })}
    </Paper>
  )
}

/** The picker node renders a Picker holding its own selection. */
export function PickerRenderer({ node, render }: { node: UIPickerNode; render?: RenderNode }) {
  const [selection, setSelection] = useState<UIPickerItem[]>(node.selected ?? [])
  return <Picker node={node} selection={selection} onSelection={setSelection} render={render} />
}

/** Picker is the picker, its selection held by its caller - the page node,
 *  or a form field (the source-set field). */
export function Picker({ node, selection, onSelection, disabled, render }: {
  node: Omit<UIPickerNode, 'kind'> & { kind?: string }
  selection: UIPickerItem[]
  onSelection: (next: UIPickerItem[]) => void
  disabled?: boolean
  /** Draws the proposals' `sub` node. */
  render?: RenderNode
}) {
  const host = useHostServices()
  const labels = { ...PICKER_LABELS, ...(node.labels ?? {}) }
  const [query, setQuery] = useState('')
  const [matches, setMatches] = useState<UIPickerItem[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('')
  // « Parcourir »: the path walked down and its level's items.
  const [path, setPath] = useState<UIPickerItem[] | null>(null)
  const [level, setLevel] = useState<UIPickerItem[]>([])
  // Where the source stands.
  const [status, setStatus] = useState<UIPickerStatus | undefined>()
  // The check-box tree's notice.
  const [notice, setNotice] = useState('')
  // The source model's picker (behind IN-045's switch): a
  // search on Enter or the icon only, the field editable while it runs,
  // hits nested by path, « Parcourir » an expandable tree.
  const sourceModel = useSourceModel()
  const [tree, setTree] = useState<UIPickerItem[] | null>(null)
  const [childrenOf, setChildrenOf] = useState<Record<string, UIPickerItem[]>>({})
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set())
  const [opening, setOpening] = useState('')
  const call = host.callMethod
  // applyOn change: each change after the first draw is
  // handed to apply, with no « Appliquer ».
  const onChange = node.applyOn === 'change' && !!node.apply && !!call
  const applier = useMemo(() => changeApplier(async (input) => {
    const out = (await call!(node.apply!, input)) as { ok?: boolean; message?: string } | undefined
    setMessage(out?.message ?? (out?.ok === false ? 'refused' : ''))
  }), [call, node.apply])
  const drawn = useRef(false)
  const selectionKey = JSON.stringify(selection)
  useEffect(() => {
    if (!onChange) return
    if (!drawn.current) { drawn.current = true; return }
    applier.change(applyInput(node, selection, host.person))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectionKey, onChange])
  useEffect(() => () => applier.cancel(), [applier])
  const statusURI = node.status
  const argsKey = JSON.stringify(node.args ?? {})
  // The source's scopes as chips (behind IN-045's switch):
  // listed once, all ticked; a search sends the ticked keys.
  const scopesDecl = sourceModel ? node.scopes : undefined
  const [scopes, setScopes] = useState<UIPickerScope[] | null>(null)
  const [ticked, setTicked] = useState<ReadonlySet<string>>(new Set())
  const [adding, setAdding] = useState<string | null>(null)
  useEffect(() => {
    if (!call || !scopesDecl?.list || !node.search) return
    let live = true
    void loadScopes(call, scopesDecl.list, node).then((got) => {
      if (!live) return
      setScopes(got.scopes)
      setTicked(got.ticked)
    })
    return () => { live = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [call, scopesDecl?.list, argsKey, node.object])
  const addOne = async (key: string) => {
    const k = key.trim()
    if (!k || !scopes) return
    setScopes(addScope(scopes, k))
    setTicked((was) => new Set(was).add(k))
    setAdding(null)
    if (call && scopesDecl?.save && !scopes.some((s) => s.key === k)) {
      // A refused save drops the chip it added.
      const why = await saveScope(call, scopesDecl.save, node, k)
      if (why !== undefined) {
        setScopes((was) => (was ? withoutScope(was, new Set(), k).scopes : was))
        setTicked((was) => withoutScope([], was, k).ticked)
        setMessage(why)
      }
    }
  }
  // « Réessayer » asks the status again.
  const [asked, setAsked] = useState(0)
  const [retrying, setRetrying] = useState(false)
  useEffect(() => {
    if (!call || !statusURI) return
    let live = true
    call(statusURI, { ...(node.args ?? {}) })
      .then((out) => { if (live) setStatus(out as UIPickerStatus) })
      .catch((e) => { if (live) setStatus({ reachable: false, message: String(e) }) })
      .finally(() => { if (live) setRetrying(false) })
    return () => { live = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [call, statusURI, argsKey, asked])
  const banner = statusBanner(status, labels)
  // A single-choice picker takes one item, never with what
  // sits under it.
  const single = node.choice === 'single'
  // A tree picker of several items is a check-box tree (
  // mock-up v6): an item is taken with what sits under it, « cet élément
  // seul » chosen on its line.
  const checkTree = node.layout === 'tree' && !single
  const take = (m: UIPickerItem) => withChildrenOf(m, checkTree)
  // An item already held: checked and disabled where it is listed, its
  // badge « déjà retenue ».
  const held = new Set(node.held ?? [])
  const heldMark = (m: UIPickerItem): ReactNode => held.has(m.key)
    ? <Badge size="xs" variant="light" color="gray" data-guide="picker.held">{labels.held}</Badge>
    : null
  const addControl = (m: UIPickerItem, chosen: boolean, onAdd: () => void): ReactNode => (
    <PickerAddControl held={held.has(m.key)} off={chosen || !!m.unreachable || !!disabled} labels={labels} onAdd={onAdd} single={single} chosen={chosen} />
  )
  // What the picker hands back: in a tickable one, the ticked items only.
  const handed = node.tickable ? tickedItems(selection) : selection
  // A tickable row shows ticked exactly what is handed back.
  const sentKeys = new Set(handed.map((s) => s.key))
  // A check-box tree's proposals, drawn above its search.
  const treeProps = checkTree && node.proposals ? node.proposals : undefined
  const browsed = useMemo(() => browsedTree(tree ?? [], childrenOf, treeProps?.items), [tree, childrenOf, treeProps?.items])
  const tickTreeKey = (key: string) => {
    const got = tickTree(selection, key, browsed)
    setNotice(got.notice ? labels.treeNotice.replace('{removed}', got.notice.removed).replace('{parent}', got.notice.parent) : '')
    onSelection(got.selection)
  }

  const open = async (to: UIPickerItem[]) => {
    if (!call || !node.browse) return
    setBusy(true)
    setMessage('')
    try {
      const parent = to.length ? to[to.length - 1].key : undefined
      const out = (await call(node.browse, browseInput(node, parent))) as { items?: UIPickerItem[] } | undefined
      setPath(to)
      setLevel(out?.items ?? [])
    } catch (e) {
      setMessage(String(e))
    } finally {
      setBusy(false)
    }
  }

  // The tree of « Parcourir »: its top level, then each
  // item's children listed once, when it is first expanded.
  const browseTop = async () => {
    if (!call || !node.browse) return
    setOpening('/')
    setMessage('')
    try {
      const uri = node.browse, browseCall = call
      const got = await browseTree(async (parent) =>
        ((await browseCall(uri, browseInput(node, parent))) as { items?: UIPickerItem[] } | undefined)?.items ?? [],
      (top, opening) => {
        // The top drawn at once, the default scope drawn opened, loading its pages.
        setTree(top)
        setExpanded((was) => openedAtOnce(was, opening))
        if (opening.length) setOpening(opening[0])
      })
      setChildrenOf((was) => ({ ...was, ...got.childrenOf }))
      setExpanded((was) => openedAtOnce(was, [...got.expanded], Object.keys(got.failed)))
      setTree(got.top)
      const why = Object.values(got.failed)
      if (why.length) setMessage(why[0])
    } catch (e) {
      setMessage(String(e))
    } finally {
      setOpening('')
    }
  }
  const toggle = async (m: UIPickerItem) => {
    if (expanded.has(m.key)) {
      setExpanded((was) => { const next = new Set(was); next.delete(m.key); return next })
      return
    }
    if (!childrenOf[m.key] && call && node.browse) {
      setOpening(m.key)
      setMessage('')
      try {
        const out = (await call(node.browse, browseInput(node, m.key))) as { items?: UIPickerItem[] } | undefined
        setChildrenOf((was) => ({ ...was, [m.key]: out?.items ?? [] }))
      } catch (e) {
        setMessage(String(e))
        return
      } finally {
        setOpening('')
      }
    }
    setExpanded((was) => new Set(was).add(m.key))
  }

  // A live picker searches while typing, from two letters -
  // never where the host shows the source model.
  // No search named: no search field, no scope chips, no
  // hint - the picker browses, or holds what was proposed.
  const finds = pickerFinds(node)
  const typing = finds.search && searchesWhileTyping(node, sourceModel)
  useEffect(() => {
    if (!typing || !call || !liveQuery(query)) return
    const timer = setTimeout(() => { void search() }, 300)
    return () => clearTimeout(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, typing, call])

  const search = async () => {
    if (!call || !node.search || query.trim() === '') return
    // Scopes shown and none ticked: refused, said on screen.
    const searched = scopeSearch(scopes, ticked)
    if (searched === null) {
      setMessage(labels.noScope)
      return
    }
    setBusy(true)
    setMessage('')
    try {
      const out = (await call(node.search, searchInput(node, query, searched))) as { items?: UIPickerItem[] } | undefined
      setMatches(out?.items ?? [])
    } catch (e) {
      setMessage(String(e))
    } finally {
      setBusy(false)
    }
  }

  const apply = async () => {
    if (!call || !node.apply) return
    setBusy(true)
    try {
      const out = (await call(node.apply, applyInput(node, selection, host.person))) as { ok?: boolean; message?: string } | undefined
      setMessage(out?.message ?? (out?.ok === false ? 'refused' : ''))
    } catch (e) {
      setMessage(String(e))
    } finally {
      setBusy(false)
    }
  }

  // A held item's state (its why its tooltip) and its source unreachable,
  // drawn on its card's line.
  const marks = (item: UIPickerItem): ReactNode => (
    <>
      {item.state && (
        <Tooltip label={item.why ?? item.state} disabled={!item.why}>
          <Badge size="xs" variant="dot" color={STATE_COLOR[item.state] ?? 'gray'}>{labels[item.state as 'read'] ?? item.state}</Badge>
        </Tooltip>
      )}
      {item.unreachable && <Badge size="xs" color="red" variant="light">{labels.unreachable}</Badge>}
    </>
  )

  return (
    <Stack {...dataAttrs(node)} gap="xs" data-element={node.id ? `picker:${node.id}` : undefined}>
      {banner && status && (
        <PickerBanner banner={banner} status={status} labels={labels} retrying={retrying}
          onRetry={call && statusURI ? () => { setRetrying(true); setAsked((n) => n + 1) } : undefined} />
      )}
      {node.derived && <Text size="xs" c="dimmed" data-guide="picker.derived">{node.derived}</Text>}
      <SimpleGrid cols={node.columns ? { base: 1, sm: 2 } : 1} spacing="md">
      <Stack gap="xs" data-guide="picker.find">
      {node.columns && (finds.search || finds.browse) && <Text size="sm" fw={600}>{finds.search ? labels.searchTitle : labels.browse}</Text>}
      {treeProps && (
        <TreeProposals node={node} selection={selection} tree={browsed} labels={labels} disabled={disabled} render={render}
          onTick={tickTreeKey} onTickAll={() => { setNotice(''); onSelection(tickAllTree(selection, treeProps.items ?? [], browsed)) }} />
      )}
      {treeProps?.choose && <Text size="sm" fw={600} mt={4} data-guide="picker.choose">{treeProps.choose}</Text>}
      {!call ? (
        <Text size="sm" c="dimmed">{labels.noHost}</Text>
      ) : finds.search && (
        <TextInput placeholder={labels.search} value={query} disabled={sourceModel ? disabled : busy || disabled}
          onChange={(e) => setQuery(e.currentTarget.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') void search() }}
          rightSection={sourceModel ? (
            <ActionIcon size="sm" variant="subtle" loading={busy} disabled={disabled} aria-label={labels.find}
              onClick={() => void search()} data-guide="picker.search-icon">
              <IconSearch size={14} />
            </ActionIcon>
          ) : undefined}
          data-guide="picker.search" />
      )}
      {finds.search && scopesDecl && scopes && (
        <ScopeChips scopes={scopes} ticked={ticked} unit={scopesDecl.unit} add={!!scopesDecl.add} adding={adding}
          labels={labels} disabled={disabled} onToggle={(key) => setTicked((was) => toggleScope(was, key))}
          onAdding={setAdding} onAdd={(key) => void addOne(key)} />
      )}
      {node.browse && call && (
        <Group gap="xs">
          {sourceModel ? (
            <Button size="compact-xs" variant="subtle" leftSection={<IconFolderOpen size={12} />} disabled={disabled}
              loading={opening === '/'} onClick={() => (tree ? setTree(null) : void browseTop())} data-guide="picker.browse">
              {labels.browse}
            </Button>
          ) : (
            <Button size="compact-xs" variant="subtle" leftSection={<IconFolderOpen size={12} />} disabled={busy || disabled}
              onClick={() => (path ? setPath(null) : void open([]))} data-guide="picker.browse">
              {labels.browse}
            </Button>
          )}
        </Group>
      )}
      {sourceModel && tree && checkTree && <TreeHelp labels={labels} />}
      {((sourceModel && tree) || treeProps) && notice && <Alert color="blue" variant="light" p="xs" data-guide="picker.tree-notice"><Text size="xs">{notice}</Text></Alert>}
      {sourceModel && tree && (
        <Paper withBorder p="xs" data-guide="picker.tree">
          {tree.length === 0 ? <Text size="sm" c="dimmed">{labels.none}</Text> : (
            <ScrollArea.Autosize mah={node.height ?? 260}>
              <Stack gap={4}>
                {browseRows(tree, childrenOf, expanded).map(({ item: m, depth }) => {
                  const chosen = selection.some((s) => s.key === m.key)
                  const isOpen = expanded.has(m.key)
                  const mark = checkTree ? treeMark(selection, m.key, browsed) : 'none'
                  // The check box first (mock-up v6): ticked, included or mixed.
                  const add = checkTree && !held.has(m.key) ? (
                    <Checkbox size="xs" checked={mark !== 'none' && mark !== 'mixed'} indeterminate={mark === 'mixed'}
                      color={mark === 'included' ? 'gray' : undefined}
                      disabled={!!m.unreachable || !!disabled} aria-label={m.title} data-guide="picker.tree-tick" data-mark={mark}
                      onChange={() => tickTreeKey(m.key)} />
                  ) : addControl(m, chosen, () => onSelection(pickInto(selection, take(m), node.choice)))
                  return (
                    <PickerLine key={m.key} depth={depth} guide="picker.tree-item" name={m.key} expanded={isOpen}
                      add={add}>
                      <Group gap={6} wrap="nowrap">
                        {m.has_children ? (
                          <ActionIcon size="xs" variant="subtle" loading={opening === m.key} aria-label={isOpen ? labels.collapse : labels.expand}
                            onClick={() => void toggle(m)} data-guide="picker.tree-toggle">
                            {isOpen ? <IconChevronDown size={12} /> : <IconChevronRight size={12} />}
                          </ActionIcon>
                        ) : <Box w={18} />}
                        <SourceItem item={m} size="browse" kinds={node.kinds} badges={node.badges} guide="picker" extra={heldMark(m)} />
                        {(mark === 'with' || mark === 'self') && m.has_children && (
                          <TreeModeButtons mode={mark} labels={labels} disabled={disabled}
                            onMode={(to) => { setNotice(''); onSelection(setTreeMode(selection, m.key, to, browsed)) }} />
                        )}
                        {mark === 'included' && <Text size="xs" c="dimmed" data-guide="picker.included">{labels.included}</Text>}
                      </Group>
                    </PickerLine>
                  )
                })}
              </Stack>
            </ScrollArea.Autosize>
          )}
        </Paper>
      )}
      {!sourceModel && path && (
        <Paper withBorder p="xs" data-guide="picker.level">
          <Group gap={4} mb={4}>
            <Text size="xs" c="dimmed" style={{ cursor: 'pointer' }} onClick={() => void open([])}>{labels.top}</Text>
            {path.map((p, i) => (
              <Group key={p.key} gap={4}>
                <IconChevronRight size={10} />
                <Text size="xs" c="dimmed" style={{ cursor: 'pointer' }} onClick={() => void open(path.slice(0, i + 1))}>{p.title}</Text>
              </Group>
            ))}
          </Group>
          {level.length === 0 ? <Text size="sm" c="dimmed">{labels.none}</Text> : (
            <ScrollArea.Autosize mah={node.height ?? 260}>
              <Stack gap={4}>
                {level.map((m) => {
                  const chosen = selection.some((s) => s.key === m.key)
                  return (
                    <PickerLine key={m.key} guide="picker.level-item" name={m.key}
                      add={addControl(m, chosen, () => onSelection(pickInto(selection, take(path.length ? { ...m, parent: m.parent ?? path[path.length - 1].key } : m), node.choice)))}>
                      <Group gap={6} wrap="nowrap" style={m.has_children ? { cursor: 'pointer' } : undefined}
                        onClick={m.has_children ? () => void open([...path, m]) : undefined}>
                        {m.has_children && <IconChevronRight size={12} />}
                        <SourceItem item={m} size="browse" kinds={node.kinds} badges={node.badges} guide="picker" extra={heldMark(m)} />
                      </Group>
                    </PickerLine>
                  )
                })}
              </Stack>
            </ScrollArea.Autosize>
          )}
        </Paper>
      )}
      {!matches && node.hint && call && finds.search && <Text size="xs" c="dimmed" data-guide="picker.hint">{node.hint}</Text>}
      {matches && (
        <Paper withBorder p="xs" data-guide="picker.matches">
          {matches.length === 0 ? <Text size="sm" c="dimmed">{labels.none}</Text> : (
            <ScrollArea.Autosize mah={node.height ?? 260}>
              <Stack gap={4}>
                {(sourceModel ? matchRows(matches) : matches.map((m): MatchRow => ({ kind: 'item', item: m, depth: 0 }))).map((row) => {
                  if (row.kind === 'place') {
                    return (
                      <Text key={row.id} size="xs" c="dimmed" pl={row.depth * 16} data-guide="picker.match-place" data-guide-name={row.title}>{row.title}</Text>
                    )
                  }
                  const m = row.item
                  const chosen = selection.some((s) => s.key === m.key)
                  return (
                    <PickerLine key={m.key} depth={row.depth} guide="picker.match" name={m.key}
                      add={addControl(m, chosen, () => onSelection(pickInto(selection, take(m), node.choice)))}>
                      <SourceItem item={m} size="search" kinds={node.kinds} badges={node.badges} guide="picker" nested={sourceModel}
                        extra={<>{heldMark(m)}{m.unreachable && <Badge size="xs" color="red" variant="light">{labels.unreachable}</Badge>}</>} />
                    </PickerLine>
                  )
                })}
              </Stack>
            </ScrollArea.Autosize>
          )}
        </Paper>
      )}
      {/* columns: the selection beside the search. */}
      </Stack>
      <Box data-guide="picker.selection">
        <Group gap="xs" mb={4} justify="space-between">
          <Text size="sm" fw={600}>{labels.selection} ({handed.length})</Text>
          {node.removeBadge && withBadge(handed, node.removeBadge).length > 0 && (() => {
            const d = badgeOf(node.badges, node.removeBadge)
            // In a tickable picker the marked items are unticked, never
            // removed, and only the ticked ones count.
            const marked = withBadge(handed, node.removeBadge)
            return (
              <Group gap={4} data-guide="picker.remove-badge">
                <Badge size="xs" variant="light" color={d.color ?? 'gray'}>{labels.of} {marked.length} {d.label}</Badge>
                <Button size="compact-xs" variant="subtle" color="red" disabled={disabled}
                  onClick={() => onSelection(node.tickable ? untickBadge(selection, node.removeBadge!) : marked.reduce((acc, m) => removeFromSelection(acc, m.key), selection))}>
                  {node.tickable ? labels.untickBadge : labels.removeAll} {d.label}
                </Button>
              </Group>
            )
          })()}
        </Group>
        {selection.length === 0 ? <Text size="sm" c="dimmed">{labels.empty}</Text> : node.tickable ? (
          <Stack gap="xs">
            {pickerGroups(selection).map((g) => {
              const all = g.ticked === g.items.length
              return (
                <Stack key={g.group} gap={4} data-guide="picker.group" data-guide-name={g.group || undefined}>
                  <Group gap="xs" justify="space-between" wrap="nowrap">
                    <Group gap={6} wrap="nowrap">
                      {g.group && <Text size="sm" fw={600}>{g.group}</Text>}
                      <Badge size="xs" variant="light" color={g.ticked ? 'blue' : 'gray'} data-guide="picker.kept">{g.ticked}/{g.items.length} {labels.kept}</Badge>
                    </Group>
                    <Button size="compact-xs" variant="subtle" disabled={disabled} data-guide="picker.tick-all"
                      onClick={() => onSelection(tickGroup(selection, g.group, !all))}>
                      {all ? labels.untickAll : labels.tickAll}
                    </Button>
                  </Group>
                  {selectionRows(g.items, node.layout ?? 'list').map(({ item, depth }) => (
                    <Group key={item.key} gap="xs" wrap="nowrap" align="flex-start" pl={depth * 16}
                      data-guide="picker.selected" data-guide-name={item.key} data-off={sentKeys.has(item.key) ? undefined : 'true'}>
                      <Checkbox size="xs" mt={3} checked={sentKeys.has(item.key)} disabled={disabled} aria-label={item.title}
                        onChange={() => onSelection(toggleItem(selection, item.key, !sentKeys.has(item.key)))} />
                      <SourceItem item={item} size="selection" kinds={node.kinds} badges={node.badges} guide="picker" off={!sentKeys.has(item.key)}
                        extra={marks(item)} />
                    </Group>
                  ))}
                </Stack>
              )
            })}
          </Stack>
        ) : (
          <Stack gap={4}>
            {selectionRows(selection, node.layout ?? 'list').map(({ item, depth }) => (
              <Group key={item.key} gap="xs" wrap="nowrap" pl={depth * 16}
                data-guide="picker.selected" data-guide-name={item.key}>
                {/* « Retirer » first, beside the name it removes. */}
                <Button size="compact-xs" variant="subtle" color="red" leftSection={<IconX size={12} />}
                  disabled={disabled} onClick={() => onSelection(removeFromSelection(selection, item.key))}>
                  {labels.remove}
                </Button>
                {/* Why it was found, a visible line (a state's why stays its tooltip). */}
                <SourceItem item={item} size="selection" kinds={node.kinds} badges={node.badges} guide="picker" extra={marks(item)} />
                {checkTree && item.with_children && <Text size="xs" c="dimmed" data-guide="picker.selected-with">{labels.withChildren}</Text>}
              </Group>
            ))}
          </Stack>
        )}
      </Box>
      </SimpleGrid>
      {node.apply && call && !onChange && (
        <Group gap="xs">
          <Button size="xs" loading={busy} onClick={() => void apply()} data-guide="picker.apply">{labels.apply}</Button>
          {message && <Text size="xs" c="dimmed">{message}</Text>}
        </Group>
      )}
      {onChange && message && <Text size="xs" c="dimmed" data-guide="picker.message">{message}</Text>}
      {!node.apply && message && <Text size="xs" c="red">{message}</Text>}
    </Stack>
  )
}
