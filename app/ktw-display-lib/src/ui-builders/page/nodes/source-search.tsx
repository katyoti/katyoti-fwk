// SPDX-License-Identifier: Apache-2.0
// Renderer of the page DSL's `source-search` node: a
// multi-source search panel - one group per source as it answers, laid over
// by sources.SearchSources' per-source states (`data.sources`). Each group
// says where its source stands - searching, waiting on the source it reads
// from, n found, nothing new, unreachable with « Réessayer », not configured
// with « Configurer » -; a found item says why it was found, with a tick;
// « Tout cocher » per group; « Ajouter depuis <source>… » opens that source's
// own panel - the node's `add` action, or with `pickers` that connector's
// generic picker in a modal, its picks joining the group ticked. What is ticked is handed to the page's selection; a change of
// the values the search ran with says the proposals are out of date, the
// ticks kept. Where it stands is sourceSearchReduce's.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, useReducer, useState, type ReactNode } from 'react'
import { Alert, Badge, Button, Checkbox, Group, Loader, Modal, Stack, Text, Tooltip } from '@mantine/core'

import { enclosingDrafts, usePageContext, useRunAction, useSectionLoad, useSelection } from '../page-context.tsx'
import {
  SOURCE_SEARCH_EMPTY, sourceGroupsShown, sourceSearchReduce, sourceSearchStale, sourceSearchValue, watchedValues,
  type SourceGroup, type UISourceAnswer, type UISourceSearchNode,
  type UIItemKinds, type UIPickerItem,
} from '../ui-page.interactive.ts'
import { SourceItem } from './source-item.tsx'
import { Picker } from './picker.tsx'
import { connectorNode, useConnectors } from '../../fields/source-set/renderer.tsx'
import type { SourceConnector } from '../../fields/source-set/types.ts'

/** The words the panel shows, the node's own when it gives them. */
export const SOURCE_SEARCH_LABELS = {
  title: 'Sources proposées',
  searching: 'recherche…',
  waiting: 'en attente',
  found: 'retenus',
  nothingNew: 'rien de nouveau',
  unreachable: 'injoignable',
  notConfigured: 'non configurée',
  retry: 'Réessayer',
  configure: 'Configurer',
  all: 'Tout cocher',
  none: 'Tout décocher',
  add: 'Ajouter depuis',
  addConfirm: 'Ajouter',
  cancel: 'Annuler',
  kept: 'éléments retenus',
  stale: 'Le périmètre a changé depuis la recherche : « Chercher encore » pour mettre les propositions à jour. Ce que vous avez coché est gardé.',
  empty: 'Aucune source cherchée.',
}

export type SourceSearchLabels = typeof SOURCE_SEARCH_LABELS

/** sourceSearchKey is the page selection's key a panel hands its value to. */
export function sourceSearchKey(node: Pick<UISourceSearchNode, 'key' | 'id'>): string {
  return node.key || node.id || 'source-search'
}

/** sourceSearchFoldKey says when a panel folds its producer's answers in:
 *  when their content changes - never on the node's identity, a page redraws
 *  its nodes on every render - or when the section it is drawn in is read
 *  again (`load`, the section's answers counted), even with the same answer:
 *  that search ran with the values as they are now. */
export function sourceSearchFoldKey(sources: UISourceAnswer[] | undefined, load: number): string {
  return `${load}|${JSON.stringify(sources ?? [])}`
}

/** sourceGroupAddable is the connector a group's « Ajouter depuis
 *  <source>… » opens with no carrier (the node's `pickers`): a group found or
 *  with nothing new - never one searching or waiting (its items hidden), not
 *  configured (« Configurer » is the way) nor unreachable (its items are not
 *  handed on: picks there would be lost) - whose connector the pickers list
 *  gives a search or a browse; undefined otherwise. */
export function sourceGroupAddable(g: SourceGroup, connectors: SourceConnector[] | null | undefined): SourceConnector | undefined {
  if (g.answer.state !== 'found' && g.answer.state !== 'nothing-new') return undefined
  const c = connectors?.find((x) => x.name === g.answer.connector)
  return c && (c.search || c.browse) ? c : undefined
}

/** SourceAddModal is « Ajouter depuis <source>… » with no carrier: the
 *  connector's generic picker (no object, no apply) on a selection of its
 *  own starting empty; « Ajouter » hands the picks on, « Annuler » drops
 *  them. */
export function SourceAddModal({ connector, name, labels, onAdd, onClose }: {
  connector: SourceConnector
  name: string
  labels: SourceSearchLabels
  onAdd: (items: UIPickerItem[]) => void
  onClose: () => void
}): ReactNode {
  const [picked, setPicked] = useState<UIPickerItem[]>([])
  return (
    <Modal opened onClose={onClose} size="lg" centered title={`${labels.add} ${name}`}>
      <Stack gap="sm" data-guide="source-search.add-modal" data-element={`source-search-add:${connector.name}`}>
        <Picker node={connectorNode(connector)} selection={picked} onSelection={setPicked} />
        <Group justify="flex-end" gap="xs">
          <Button variant="default" size="xs" onClick={onClose} data-guide="source-search.add-cancel">{labels.cancel}</Button>
          <Button size="xs" disabled={picked.length === 0} data-guide="source-search.add-confirm"
            onClick={() => { onAdd(picked); onClose() }}>{labels.addConfirm}</Button>
        </Group>
      </Stack>
    </Modal>
  )
}

/** sourceGroupBadge is a group's state in the panel's words. */
export function sourceGroupBadge(g: SourceGroup, labels: SourceSearchLabels): { text: string; color: string } {
  const a = g.answer
  switch (a.state) {
    case 'searching': return { text: labels.searching, color: 'blue' }
    case 'waiting': return { text: labels.waiting, color: 'gray' }
    case 'nothing-new': return { text: labels.nothingNew, color: 'gray' }
    case 'unreachable': return { text: labels.unreachable, color: 'red' }
    case 'not-configured': return { text: labels.notConfigured, color: 'yellow' }
  }
  return { text: `${g.ticked.length}/${(a.items ?? []).length} ${labels.found}`, color: 'green' }
}

/** SourceGroupView draws one source's group: its head (its name, its state,
 *  « Tout cocher », « Ajouter depuis <source>… ») and its body - where its
 *  search is, why it is unreachable with « Réessayer », where it is
 *  configured with « Configurer », or its items each with its why and a tick. */
export function SourceGroupView({ group, labels, kinds, disabled, retrying, onTick, onAll, onRetry, onConfigure, onAdd }: {
  group: SourceGroup
  labels: SourceSearchLabels
  /** The item kinds its items are drawn with; the source's
   *  own laid over them. */
  kinds?: UIItemKinds
  disabled?: boolean
  retrying?: boolean
  onTick: (key: string, on: boolean) => void
  onAll: (on: boolean) => void
  onRetry?: () => void
  onConfigure?: () => void
  onAdd?: () => void
}): ReactNode {
  const a = group.answer
  const name = a.label || a.connector
  const items = a.items ?? []
  const pending = a.state === 'searching' || a.state === 'waiting'
  const tickable = items.filter((i) => !i.unreachable)
  const allOn = tickable.length > 0 && tickable.every((i) => group.ticked.includes(i.key))
  const badge = sourceGroupBadge(group, labels)
  const itemKinds = a.kinds ? { ...(kinds ?? {}), ...a.kinds } : kinds
  return (
    <Stack gap={4} data-guide={`source-search.group.${a.connector}`} data-state={a.state} data-element={`source-search:${a.connector}`}>
      <Group justify="space-between" wrap="nowrap" gap="xs">
        <Group gap={6} wrap="nowrap">
          <Text size="sm" fw={600} c={a.color}>{name}</Text>
          <Badge size="sm" variant="light" color={badge.color} tt="none" data-guide="source-search.state">{badge.text}</Badge>
        </Group>
        <Group gap={6} wrap="nowrap">
          {!pending && tickable.length > 0 && (
            <Button size="compact-xs" variant="subtle" disabled={disabled} data-guide="source-search.all" onClick={() => onAll(!allOn)}>
              {allOn ? labels.none : labels.all}
            </Button>
          )}
          {onAdd && (
            <Button size="compact-xs" variant="light" disabled={disabled} data-guide="source-search.add" onClick={onAdd}>
              {`${labels.add} ${name}…`}
            </Button>
          )}
        </Group>
      </Group>
      {a.state === 'searching' && (
        <Group gap={6}><Loader size="xs" /><Text size="xs" c="dimmed">{`Recherche dans ${name}…`}</Text></Group>
      )}
      {a.state === 'waiting' && (
        <Text size="xs" c="dimmed" data-guide="source-search.waiting">{`attend ${a.waiting_label || a.waiting || ''}`.trim()}</Text>
      )}
      {a.state === 'unreachable' && (
        <Alert color="red" variant="light" p="xs" data-guide="source-search.unreachable">
          <Group gap="xs" justify="space-between" wrap="nowrap">
            <Text size="xs">{a.message || labels.unreachable}</Text>
            {onRetry && (
              <Button size="compact-xs" variant="light" color="red" loading={retrying} disabled={disabled} onClick={onRetry} data-guide="source-search.retry">
                {labels.retry}
              </Button>
            )}
          </Group>
        </Alert>
      )}
      {a.state === 'not-configured' && (
        <Alert color="yellow" variant="light" p="xs" data-guide="source-search.not-configured">
          <Group gap="xs" justify="space-between" wrap="nowrap">
            <Text size="xs">{a.configure?.hint || a.message || labels.notConfigured}</Text>
            {onConfigure ? (
              <Button size="compact-xs" variant="light" color="yellow" disabled={disabled} onClick={onConfigure} data-guide="source-search.configure">
                {a.configure?.label || labels.configure}
              </Button>
            ) : (
              <Text size="xs" fw={600} data-guide="source-search.configure">{a.configure?.label || labels.configure}</Text>
            )}
          </Group>
        </Alert>
      )}
      {!pending && items.map((i) => (
        <Checkbox key={i.key} size="xs" checked={group.ticked.includes(i.key)} disabled={disabled || i.unreachable}
          data-guide="source-search.item" data-guide-name={i.key}
          onChange={(e) => onTick(i.key, e.currentTarget.checked)}
          label={<SourceItem item={i} size="search" kinds={itemKinds} guide="source-search" />} />
      ))}
      {(a.errors ?? []).map((e) => <Text key={e} size="xs" c="orange">{e}</Text>)}
    </Stack>
  )
}

export function SourceSearchRenderer({ node }: { node: UISourceSearchNode }): ReactNode {
  const page = usePageContext()
  const { selection, select } = useSelection()
  const runAction = useRunAction()
  const retry = runAction(node.retry)
  const configure = runAction(node.configure)
  const add = runAction(node.add)
  // With no `add`, `pickers` lists the connectors for the generic picker.
  const { connectors } = useConnectors(!add && node.pickers ? { connectors: node.pickers } : undefined)
  const [adding, setAdding] = useState<{ connector: SourceConnector; name: string } | null>(null)
  const labels = { ...SOURCE_SEARCH_LABELS, ...(node.labels ?? {}) }
  // Watched in its page's data, else in the pages around it: drawn in a
  // loaded section, it watches the form of the page holding the section.
  const watched = watchedValues(node.watch, page.draft, enclosingDrafts(page))
  // The first answers are drawn at once (a section's answer has every source
  // answered); a section read again folds its answer in, the ticks kept.
  const [state, move] = useReducer(sourceSearchReduce, undefined,
    () => sourceSearchReduce(SOURCE_SEARCH_EMPTY, { type: 'answers', sources: node.data?.sources, watched }))

  // The producer's answers fold in when they change, or when the section the
  // panel is drawn in is read again (sourceSearchFoldKey).
  const fold = sourceSearchFoldKey(node.data?.sources, useSectionLoad())
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { move({ type: 'answers', sources: node.data?.sources, watched }) }, [fold])

  // What is ticked goes to the page's selection, a form's submit reading it.
  const key = sourceSearchKey(node)
  const value = sourceSearchValue(state)
  const valueKey = JSON.stringify(value)
  useEffect(() => {
    if (state.groups.length === 0) return
    if (JSON.stringify(selection[key]) !== valueKey) select(key, value)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, valueKey])

  const groups = sourceGroupsShown(state)
  // « Ajouter depuis <source>… »: the node's action wins; else the
  // connector's picker in a modal where the group can take picks.
  const addOf = (g: SourceGroup): (() => void) | undefined => {
    if (add) return () => add(g.answer)
    const c = sourceGroupAddable(g, connectors)
    return c ? () => setAdding({ connector: c, name: g.answer.label || c.label || c.name }) : undefined
  }
  const kept = value.selected.reduce((n, s) => n + s.items.length, 0)
  return (
    <Stack {...dataAttrs(node)} gap="sm" data-element={`source-search:${node.id ?? 'source-search'}`} data-guide="source-search">
      <Group justify="space-between">
        <Text size="sm" fw={600}>{node.title ?? labels.title}</Text>
        {groups.length > 0 && <Badge size="sm" variant="light" color="gray" tt="none" data-guide="source-search.kept">{`${kept} ${labels.kept}`}</Badge>}
      </Group>
      {node.note && <Text size="xs" c="dimmed">{node.note}</Text>}
      {sourceSearchStale(state, watched) && <Text size="xs" c="orange" data-guide="source-search.stale">{labels.stale}</Text>}
      {groups.length === 0 && <Text size="xs" c="dimmed">{labels.empty}</Text>}
      {groups.map((g) => (
        <SourceGroupView key={g.answer.connector} group={g} labels={labels} kinds={node.kinds} disabled={node.disabled}
          retrying={state.retrying.includes(g.answer.connector)}
          onTick={(k, on) => move({ type: 'tick', connector: g.answer.connector, key: k, on })}
          onAll={(on) => move({ type: 'all', connector: g.answer.connector, on })}
          onRetry={retry ? () => {
            move({ type: 'retry', connector: g.answer.connector })
            retry(g.answer, { only: g.answer.connector })
          } : undefined}
          onConfigure={configure ? () => configure(g.answer) : undefined}
          onAdd={addOf(g)} />
      ))}
      {adding && (
        <SourceAddModal connector={adding.connector} name={adding.name} labels={labels}
          onAdd={(items) => move({ type: 'add', connector: adding.connector.name, items })}
          onClose={() => setAdding(null)} />
      )}
    </Stack>
  )
}
