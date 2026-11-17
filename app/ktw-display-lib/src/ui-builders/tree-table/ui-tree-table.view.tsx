// SPDX-License-Identifier: Apache-2.0
// UITreeTableView — renders a built UITreeTable as nested, indented rows.
//
// The data is a FLAT node list linked by `parentField`; this view derives the
// hierarchy, renders each row with a depth indent + expand/collapse chevron on
// the first column, and renders the `stateField` column as a colored status
// badge. Reuses the ui-table column renderers for every other cell.

import { ActionIcon, Badge, Box, Group, Paper, ScrollArea, Table, Text, ThemeIcon } from '@mantine/core'
import {
  IconCheck,
  IconChevronDown,
  IconChevronRight,
  IconChevronsLeft,
  IconChevronsRight,
  IconEqual,
  IconPencil,
} from '@tabler/icons-react'
import { Fragment, useMemo, useState, type ReactNode } from 'react'

import type { UIActionProcessor } from '../../ui-core/action/index.ts'
import type { UITreeTable } from './ui-tree-table.types.ts'

/** state value → Mantine badge color. Unknown states fall back to gray. */
const STATE_COLORS: Record<string, string> = {
  done: 'green',
  failed: 'red',
  processing: 'blue',
  pending: 'gray',
  discovered: 'gray',
  clean: 'gray',
  skip: 'gray',
  download: 'cyan',
  upload: 'orange',
  create: 'teal',
  'delete-local': 'red',
  'delete-remote': 'red',
  'move-local': 'grape',
  'move-remote': 'grape',
  'rename-local': 'violet',
  conflict: 'yellow',
}

const ROW_CLICK_ACTION = { id: 'click', scope: 'item' as const }

export type UITreeTableViewProps<T = any> = {
  tree: UITreeTable<T>
  data: T[]
  actionProcessor?: UIActionProcessor
}

export type FlatRow<T> = { row: T; id: string; depth: number; hasChildren: boolean }

/** Read a row's tree key. Rows may legitimately carry no id (e.g. a local page
 *  not yet created remotely); those are leaves — never a parent key, since the
 *  empty key addresses the ROOT bucket. */
function nodeKey(row: Record<string, unknown>, idField: string): string {
  const v = row[idField]
  return v === undefined || v === null ? '' : String(v)
}

/** Derive the display order (depth-first) from the flat node list, honoring the
 *  collapsed set. Roots = nodes whose parent is empty or not present.
 *
 *  Hostile data is expected: key-less rows and parent cycles both used to make
 *  `walk` recurse forever ("Maximum call stack size exceeded"). Key-less rows
 *  are leaves, a node is descended into once per path, and anything left
 *  unreachable (a pure cycle) is appended flat rather than dropped. */
export function flatten<T>(
  data: T[],
  idField: string,
  parentField: string,
  collapsed: Set<string>,
): FlatRow<T>[] {
  const rec = data as unknown as Record<string, unknown>[]
  const byId = new Map<string, Record<string, unknown>>()
  const childrenOf = new Map<string, Record<string, unknown>[]>()
  for (const r of rec) {
    const id = nodeKey(r, idField)
    if (id) byId.set(id, r)
  }
  for (const r of rec) {
    const parent = String(r[parentField] ?? '')
    const key = parent && byId.has(parent) ? parent : ''
    if (!childrenOf.has(key)) childrenOf.set(key, [])
    childrenOf.get(key)!.push(r)
  }

  // Which rows hang off a root at all — computed ignoring `collapsed`, so a
  // collapsed subtree counts as reachable (hidden, not orphaned).
  const reachable = new Set<Record<string, unknown>>()
  const mark = (parentKey: string, onPath: Set<string>) => {
    for (const r of childrenOf.get(parentKey) ?? []) {
      if (reachable.has(r)) continue
      reachable.add(r)
      const id = nodeKey(r, idField)
      if (!id || onPath.has(id)) continue
      onPath.add(id)
      mark(id, onPath)
      onPath.delete(id)
    }
  }
  mark('', new Set())

  const out: FlatRow<T>[] = []
  const onPath = new Set<string>()
  const walk = (parentKey: string, depth: number) => {
    for (const r of childrenOf.get(parentKey) ?? []) {
      const id = nodeKey(r, idField)
      const kids = id && !onPath.has(id) ? (childrenOf.get(id) ?? []) : []
      out.push({ row: r as T, id: id || `row-${out.length}`, depth, hasChildren: kids.length > 0 })
      if (kids.length > 0 && !collapsed.has(id)) {
        onPath.add(id)
        walk(id, depth + 1)
        onPath.delete(id)
      }
    }
  }
  walk('', 0)
  // Rows no root reaches (a parent cycle) — show them rather than silently
  // swallow them.
  for (const r of rec) {
    if (reachable.has(r)) continue
    out.push({ row: r as T, id: nodeKey(r, idField) || `row-${out.length}`, depth: 0, hasChildren: false })
  }
  return out
}

export function UITreeTableView<T = any>({ tree, data, actionProcessor }: UITreeTableViewProps<T>) {
  const {
    columns,
    idField,
    parentField,
    stateField,
    defaultExpanded,
    indentPx,
    withTableBorder = true,
    withColumnBorders,
    highlightOnHover = true,
    striped,
  } = tree

  // Collapsed set. defaultExpanded=false starts every parent collapsed.
  const [collapsed, setCollapsed] = useState<Set<string>>(() => {
    if (defaultExpanded) return new Set()
    const rec = data as unknown as Record<string, unknown>[]
    return new Set(rec.map((r) => String(r[idField] ?? '')).filter(Boolean))
  })

  const rows = useMemo(
    () => flatten<T>(data, idField, parentField, collapsed),
    [data, idField, parentField, collapsed],
  )

  const toggle = (id: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const onRowClick = (row: T) => {
    if (!actionProcessor) return
    void actionProcessor({ action: ROW_CLICK_ACTION, item: row } as unknown as Parameters<UIActionProcessor>[0])
  }

  // The column that carries the tree indent + chevron (default: first column).
  const treeColId = tree.treeColumn ?? columns[0]?.id

  return (
    <Paper withBorder={withTableBorder}>
      <ScrollArea>
        <Table highlightOnHover={highlightOnHover} striped={striped} withColumnBorders={withColumnBorders}>
          <Table.Thead>
            <Table.Tr>
              {columns.map((column) => (
                <Table.Th key={column.id} style={{ textAlign: column.align ?? 'left' }}>
                  {column.renderHeader()}
                </Table.Th>
              ))}
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {rows.map(({ row, id, depth, hasChildren }, index) => (
              <Fragment key={`${id}#${index}`}>
                <Table.Tr
                  onClick={() => onRowClick(row)}
                  style={{ cursor: actionProcessor ? 'pointer' : 'default' }}
                >
                  {columns.map((column) => {
                    const isTreeCol = column.id === treeColId
                    const rec = row as Record<string, unknown>
                    // Column `kind` (declared on the ui-tree-table column) picks a
                    // rich cell renderer; the configured stateField defaults to a
                    // 'state' badge; everything else uses the ui-table renderer.
                    const kind = (column as { kind?: string }).kind ?? (column.id === stateField ? 'state' : undefined)
                    let cell = renderKindCell(kind, column, rec) ?? column.renderCell(row, index)
                    if (isTreeCol) {
                      cell = (
                        <Group gap={4} wrap="nowrap" style={{ paddingLeft: depth * indentPx }}>
                          {hasChildren ? (
                            <ActionIcon
                              variant="subtle"
                              size="sm"
                              onClick={(e) => {
                                e.stopPropagation()
                                toggle(id)
                              }}
                            >
                              {collapsed.has(id) ? <IconChevronRight size={14} /> : <IconChevronDown size={14} />}
                            </ActionIcon>
                          ) : (
                            <Box w={22} />
                          )}
                          <Box>{cell}</Box>
                        </Group>
                      )
                    }
                    return (
                      <Table.Td key={column.id} style={{ textAlign: column.align ?? 'left' }}>
                        {cell}
                      </Table.Td>
                    )
                  })}
                </Table.Tr>
              </Fragment>
            ))}
          </Table.Tbody>
        </Table>
      </ScrollArea>
    </Paper>
  )
}

function renderStateBadge(value: unknown) {
  const state = String(value ?? '')
  if (!state) return null
  return (
    <Badge color={STATE_COLORS[state] ?? 'gray'} variant="light" size="sm">
      {state}
    </Badge>
  )
}

// States that count as "clean / up-to-date" for the version check vs pencil.
const CLEAN_STATES = new Set(['', 'clean', 'up-to-date', 'uptodate', 'done', 'ok', 'synced'])

/** renderKindCell renders a cell by the column's declared `kind`:
 *   'state'     → a colored status badge
 *   'version'   → ✓ (clean) / ✎ (changed) + the version value, colored by the
 *                 companion state field (`column.stateField`)
 *   'direction' → sync-direction arrows (upload ▶▶ / download ◀◀ / skip ═)
 *  Returns null for unknown kinds so the caller falls back to the default cell. */
function renderKindCell(
  kind: string | undefined,
  column: { id: string; stateField?: string },
  row: Record<string, unknown>,
): ReactNode | null {
  const val = row[column.id]
  switch (kind) {
    case 'state':
      return renderStateBadge(val)
    case 'version':
      return renderVersion(val, row[(column as { stateField?: string }).stateField ?? ''])
    case 'direction':
      return renderDirection(val)
    default:
      return null
  }
}

function renderVersion(value: unknown, state: unknown): ReactNode {
  const v = value === undefined || value === null || value === '' ? '' : String(value)
  const st = String(state ?? '')
  const clean = CLEAN_STATES.has(st)
  const color = clean ? 'green' : 'yellow'
  return (
    <Group gap={4} wrap="nowrap">
      <ThemeIcon color={color} variant="light" size="sm" radius="xl">
        {clean ? <IconCheck size={13} /> : <IconPencil size={13} />}
      </ThemeIcon>
      {v && (
        <Text size="sm" c="dimmed">
          {v}
        </Text>
      )}
    </Group>
  )
}

function renderDirection(value: unknown): ReactNode {
  const dir = String(value ?? '').toLowerCase()
  if (dir === 'upload' || dir === 'push' || dir === 'create') {
    return <IconChevronsRight size={18} color="var(--mantine-color-teal-6)" />
  }
  if (dir === 'download' || dir === 'pull') {
    return <IconChevronsLeft size={18} color="var(--mantine-color-blue-6)" />
  }
  // skip / clean / equal
  return <IconEqual size={16} color="var(--mantine-color-gray-5)" />
}
