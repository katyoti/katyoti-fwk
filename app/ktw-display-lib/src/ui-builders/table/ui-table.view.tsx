// SPDX-License-Identifier: Apache-2.0
// UITableView — React component that renders a built UITable.
//
// Ported from _ktw_workspace/.../cpm-ui/ui-table/ui-table.view.tsx during
// gui-table Subtask 2 (2026-05-04). v1 trim per Q4 of ANALYZE: dropped
// bulkMenu / rowMenu / toolboxMenu render branches (those depend on the
// cpm-core/actions/menus renderers we cut in gui-form Subtask 1; v1.5
// follow-up todo `gui-table-features` covers them).
//
// Features in v1:
//   - Sort (per-column, click header to toggle, single-column at a time)
//   - Draws only what is seen: its first pageSize rows (from
//     the extension, or 10) and « Afficher plus » adding the next page, on
//     every host; `pagination: false` draws every row (a server-paged table)
//   - Row click → emits action via actionProcessor (host wires to
//     navigation / detail-pane open / etc.)
//   - Filter integration via `useUIFilters` (search / options / tabs)
//
// `data` is plain `T[]` (the harness passes `config.initialValue` as-is).
// No CRUD-layer ListQueryResult wrapper — that's host territory.

import {
  Alert,
  Box,
  Center,
  Group,
  Paper,
  ScrollArea,
  Stack,
  Table,
  Text,
} from '@mantine/core'
import { IconAlertCircle, IconChevronDown, IconChevronUp } from '@tabler/icons-react'
import { Fragment, useCallback, useMemo, useState, type ReactNode } from 'react'

import type { UIActionProcessor } from '../../ui-core/action/index.ts'
import { useUIFilters } from '../filter/index.ts'
import type { UIColumn, UITable } from './ui-table.types.ts'
import { CompactRow, compactGrid, compactListStyle, rowClickKind } from './ui-table.compact.tsx'
import { ShowMore, useWindow } from '../page/nodes/window.tsx'

const ROW_CLICK_ACTION = { id: 'click', scope: 'item' as const }

export type UITableViewProps<T = any> = {
  table: UITable<T>
  data: T[]
  isLoading?: boolean
  error?: { message: string } | null
  actionProcessor?: UIActionProcessor
  /** A click on a row opens it (the host's verb — a page table's `open`);
   *  called instead of the action processor's row click. */
  onRowOpen?: (row: T) => void
  /** Whether a row opens (its open action's `when`): a row it refuses
   *  neither opens on a click nor shows a pointer. */
  rowOpens?: (row: T) => boolean
  /** The row's own actions, drawn in a last column (a page table's
   *  `actions`). */
  rowActions?: (row: T) => ReactNode
  /** A click on a row selects it (a page table whose extension declares
   *  `select`); called before `onRowOpen`. */
  onRowSelect?: (row: T) => void
  /** Whether a row is the selected one — drawn highlighted. */
  isSelected?: (row: T) => boolean
  /** The row's menu (the extension's `menu`), drawn as its last cell. */
  rowMenu?: (row: T) => ReactNode
  /** Extra attributes on the root element (a page node's data-* keys); the view's own win. */
  attrs?: Record<string, string>
}

export function UITableView<T = any>({
  table,
  data,
  isLoading = false,
  error = null,
  actionProcessor,
  onRowOpen,
  rowOpens,
  rowActions,
  onRowSelect,
  isSelected,
  rowMenu,
  attrs,
}: UITableViewProps<T>) {
  const [sortColumn, setSortColumn] = useState<string | null>(null)
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc')

  const {
    filters,
    columns,
    rowId,
    texts,
    withTableBorder = true,
    withColumnBorders,
    withRowBorders,
    highlightOnHover = true,
    striped,
    density = 'normal',
    pageSize = 10,
    stickyHeader,
    maxHeight,
  } = table
  const compact = density === 'compact'
  // A compact table windows only when asked; it draws a header only when asked.
  const pagination = table.pagination ?? !compact
  const withHeader = table.header ?? !compact

  const trailing = !!(rowActions || rowMenu)
  const nbColumns = columns.length + (trailing ? 1 : 0)
  const on = { select: onRowSelect, open: onRowOpen, processor: actionProcessor, opens: rowOpens }
  const rowClickable = (row: T) => rowClick(on, row) !== undefined

  // ============================================================================
  // Sorting
  // ============================================================================

  const comparator = useCallback(
    (a: T, b: T): number => {
      if (!sortColumn) return 0
      const aVal = (a as Record<string, unknown>)[sortColumn]
      const bVal = (b as Record<string, unknown>)[sortColumn]
      if (aVal === bVal) return 0
      if (aVal == null) return 1
      if (bVal == null) return -1
      const result = (aVal as number | string) < (bVal as number | string) ? -1 : 1
      return sortDirection === 'asc' ? result : -result
    },
    [sortColumn, sortDirection],
  )

  // ============================================================================
  // Filtering
  // ============================================================================

  const { filter, isFiltered, FilterToolbar } = useUIFilters({
    filters,
    comparator,
  })

  const filteredData = useMemo(
    () => allRows(data, filters && isFiltered ? (rows) => filter(rows) as T[] : undefined, sortColumn ? comparator : undefined),
    [data, filter, isFiltered, filters, sortColumn, comparator],
  )

  const handleSort = (columnId: string) => {
    if (sortColumn === columnId) {
      setSortDirection((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortColumn(columnId)
      setSortDirection('asc')
    }
  }

  // ============================================================================
  // Window — the first page of the filtered, sorted rows; « Afficher plus »
  // adds the next one
  // ============================================================================

  const win = useWindow(filteredData.length, pageSize, true)
  const shown = pagination ? win.shown : filteredData.length
  const paginatedData = useMemo(() => filteredData.slice(0, shown), [filteredData, shown])

  // ============================================================================
  // Row click → action emit
  // ============================================================================

  const handleRowClick = (row: T) => {
    const kind = rowClick(on, row)
    if (kind === 'select') { onRowSelect!(row); return }
    if (kind === 'open') { onRowOpen!(row); return }
    if (kind !== 'processor') return
    void actionProcessor!({
      action: ROW_CLICK_ACTION,
      item: row,
    } as unknown as Parameters<UIActionProcessor>[0])
  }

  // ============================================================================
  // Sentinel states (loading + error)
  // ============================================================================

  if (isLoading) {
    return (
      <Paper {...attrs} withBorder={withTableBorder} p="md">
        <Text c="dimmed" size="sm">
          {texts.t('table.loading', { defaultValue: 'Loading…' })}
        </Text>
      </Paper>
    )
  }

  if (error) {
    return (
      <Paper {...attrs} withBorder={withTableBorder} p="md">
        <Alert
          icon={<IconAlertCircle size={16} />}
          title={texts.t('table.errorTitle', { defaultValue: 'Error loading table' })}
          color="red"
        >
          {error.message}
        </Alert>
      </Paper>
    )
  }

  const isEmpty = paginatedData.length === 0
  const isNotFoundDueToFilter = isEmpty && isFiltered
  const trailingCell = (row: T): ReactNode => (trailing ? <>{rowActions?.(row)}{rowMenu?.(row)}</> : undefined)
  const emptyText = isNotFoundDueToFilter
    ? texts.t('table.empty.filtered', { defaultValue: 'No rows match the current filters.' })
    : texts.t('table.empty.noData', { defaultValue: 'No data to display.' })

  const pager = filteredData.length > shown && (
    <Group p="sm" data-ktw-table-more="">
      <ShowMore rest={filteredData.length - shown} onMore={win.more} />
    </Group>
  )

  // ============================================================================
  // Render — compact rows
  // ============================================================================

  if (compact) {
    const grid = compactGrid(columns, trailing)
    return (
      <Stack {...attrs} gap="xs">
        {FilterToolbar && <FilterToolbar />}
        <div style={compactListStyle} data-ktw-table="compact">
          {withHeader && !isEmpty && (
            <div style={{ display: 'grid', gridTemplateColumns: grid, gap: 10, padding: '6px 10px', fontSize: 12, fontWeight: 600, borderBottom: '1px solid var(--mantine-color-default-border)' }}>
              {columns.map((c) => <div key={c.id}>{c.renderHeader()}</div>)}
              {trailing && <div />}
            </div>
          )}
          {isEmpty ? (
            <Text c="dimmed" size="sm" p="sm">{emptyText}</Text>
          ) : (
            paginatedData.map((row, index) => (
              <CompactRow key={rowId(row) || `row-${index}`} row={row} index={index} columns={columns} grid={grid}
                selected={isSelected?.(row) ?? false} clickable={rowClickable(row)} onClick={() => handleRowClick(row)}
                trailing={trailingCell(row)} last={index === paginatedData.length - 1} />
            ))
          )}
        </div>
        {pager}
      </Stack>
    )
  }

  // ============================================================================
  // Render
  // ============================================================================

  return (
    <Stack {...attrs} gap="md">
      {/* Filter chrome (renders only when at least one filter is configured) */}
      {FilterToolbar && <FilterToolbar />}

      {/* Table */}
      <Paper withBorder={withTableBorder}>
        <TableScroll sticky={!!stickyHeader} maxHeight={maxHeight}>
          <Table
            stickyHeader={!!stickyHeader}
            highlightOnHover={highlightOnHover}
            striped={striped}
            withTableBorder={withTableBorder}
            withColumnBorders={withColumnBorders}
            withRowBorders={withRowBorders}
          >
            {withHeader && (
              <Table.Thead>
                <Table.Tr>
                  {columns.map((column) => renderHeaderCell(column, sortColumn, sortDirection, handleSort))}
                  {trailing && <Table.Th style={{ width: 1 }} aria-label="Actions" />}
                </Table.Tr>
              </Table.Thead>
            )}
            <Table.Tbody>
              {isEmpty ? (
                <Table.Tr>
                  <Table.Td colSpan={nbColumns}>
                    <Center py="xl">
                      <Stack gap="xs" align="center">
                        <IconAlertCircle size={48} color="var(--mantine-color-gray-5)" />
                        <Text c="dimmed" size="sm">
                          {isNotFoundDueToFilter
                            ? texts.t('table.empty.filtered', {
                                defaultValue: 'No rows match the current filters.',
                              })
                            : texts.t('table.empty.noData', {
                                defaultValue: 'No data to display.',
                              })}
                        </Text>
                      </Stack>
                    </Center>
                  </Table.Td>
                </Table.Tr>
              ) : (
                paginatedData.map((row, index) => {
                  const id = rowId(row) || `row-${index}`
                  return (
                    <Fragment key={id}>
                      <Table.Tr
                        onClick={() => handleRowClick(row)}
                        data-row-selected={isSelected?.(row) ? 'true' : undefined}
                        style={{
                          cursor: rowClickable(row) ? 'pointer' : 'default',
                          background: isSelected?.(row) ? 'var(--mantine-color-default-hover)' : undefined,
                        }}
                      >
                        {columns.map((column) => (
                          <Table.Td
                            key={column.id}
                            style={{ textAlign: column.align ?? 'left' }}
                          >
                            {column.renderCell(row, index)}
                          </Table.Td>
                        ))}
                        {trailing && (
                          <Table.Td style={{ whiteSpace: 'nowrap' }} onClick={(e) => e.stopPropagation()}>
                            {trailingCell(row)}
                          </Table.Td>
                        )}
                      </Table.Tr>
                    </Fragment>
                  )
                })
              )}
            </Table.Tbody>
          </Table>
        </TableScroll>

        {/* « Afficher plus » */}
        {pager}
      </Paper>
    </Stack>
  )
}

/** rowClick is what a click on this row runs (rowClickKind), none when it
 *  would open a row its open action's `when` refuses (`opens`). */
export function rowClick<T>(on: { select?: unknown; open?: unknown; processor?: unknown; opens?: (row: T) => boolean }, row: T): 'select' | 'open' | 'processor' | undefined {
  const kind = rowClickKind(on)
  if (kind === 'open' && on.opens && !on.opens(row)) return undefined
  return kind
}

/** TableScroll holds the table's rows: a sticky head (`stickyHeader`) stays
 *  at the top of its scroll box - a box `maxHeight` high, else the page
 *  itself (no box of its own, which would hold the head in place); a table
 *  without one scrolls sideways in its own box. */
function TableScroll({ sticky, maxHeight, children }: { sticky: boolean; maxHeight?: number | string; children: ReactNode }) {
  if (maxHeight !== undefined && maxHeight !== null && maxHeight !== '') {
    return <ScrollArea.Autosize mah={maxHeight} data-ktw-table-scroll="">{children}</ScrollArea.Autosize>
  }
  if (sticky) return <>{children}</>
  return <ScrollArea>{children}</ScrollArea>
}

/** allRows is every row the table holds, filtered then sorted, before its
 *  window takes the first pages: filter and sort act over all of them. The
 *  filter (useUIFilters's) sorts inside; without one the comparator sorts. */
export function allRows<T>(data: T[], filter?: (rows: T[]) => T[], comparator?: (a: T, b: T) => number): T[] {
  if (filter) return filter(data)
  return comparator ? [...data].sort(comparator) : data
}

// ============================================================================
// Header cell helper
// ============================================================================

function renderHeaderCell<T = any>(
  column: UIColumn<T>,
  sortColumn: string | null,
  sortDirection: 'asc' | 'desc',
  handleSort: (id: string) => void,
) {
  const isSortable = column.sortable !== false // default sortable
  const isSorted = column.id === sortColumn
  return (
    <Table.Th
      key={column.id}
      style={{
        textAlign: column.align ?? 'left',
        width: column.width,
        cursor: isSortable ? 'pointer' : 'default',
        userSelect: 'none',
      }}
      onClick={() => isSortable && handleSort(column.id)}
    >
      <Group
        gap="xs"
        justify={
          column.align === 'right'
            ? 'flex-end'
            : column.align === 'center'
              ? 'center'
              : 'flex-start'
        }
      >
        <Box>{column.renderHeader()}</Box>
        {isSortable && isSorted &&
          (sortDirection === 'asc' ? <IconChevronUp size={14} /> : <IconChevronDown size={14} />)}
      </Group>
    </Table.Th>
  )
}
