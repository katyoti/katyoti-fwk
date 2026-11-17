// SPDX-License-Identifier: Apache-2.0
// Schema-bound node renderers (5 kinds): form, view, table, wizard,
// timeline-lanes.
//
// These are the inline-delegation arms of the DSL — when a `<UIPageView>`
// hits a schema-bound node, it calls into the existing `buildUIxxx()`
// builder (cheap, no rendering yet) and mounts the corresponding view
// component INLINE inside the same iframe. NO nested iframes (per
// the display module's decision on where the UI lives (modules/display/decisions/)).
//
// Schema and data (the component vocabulary, render.Def):
// - `schema` is the inline canonical AST. A schema named by id (a data-type
//   URI) or by an action's input (`{action: <uri>}`) and an inline `ui` block
//   are resolved by the host before the page is shown (Go, the object pane's
//   ResolvePanel); one reaching this renderer unresolved means the page was
//   mounted without being resolved.
// - `variant` picks the `ui-<kind>` extension (default 'default').
// - `data` is the value, inline or bound from the page's data by `dataField`;
//   a bound form edits the page's working copy (page-data.ts).

import { Badge, Notification, Stack } from '@mantine/core'
import { useMemo, type ReactNode } from 'react'
import { dataAttrs } from './data-attrs.ts'
import { savesOnChange } from '../../form/form-line.ts'

import {
  buildUIForms,
  UIFormView,
  type UIForm,
} from '../../form/index.ts'
import {
  buildUIViews,
  UIViewView,
  type UIView,
} from '../../view/index.ts'
import {
  buildUITables,
  UITableView,
  type UITable,
} from '../../table/index.ts'
import {
  buildUIWizards,
  UIWizardView,
  type UIWizard,
} from '../../wizard/index.ts'
import {
  buildUITimelineLanes,
  UITimelineLanesView,
  type UITimelineLanes,
} from '../../timeline-lanes/index.ts'
import type { Schema } from '../../../schema/index.ts'
import type { UIActionParams, UIActionProcessor, UIActionRef } from '../../../ui-core/action/index.ts'
import { resolveJSONPath } from '../../../utils/json-path.ts'
import { ItemContext, pageOwn, usePageContext, useRunAction, useSelection } from '../page-context.tsx'
import type { UIMenuItem, UIPageAction } from '../ui-page.interactive.ts'
import { ActionButtons, actionShown } from './interactive.tsx'
import { RowMenu } from './choice.tsx'
import { PagedFooter, usePagedRows } from './paged-load.tsx'

import type {
  UIFormNode,
  UIPageNode,
  UISchemaBoundNode,
  UITableNode,
  UITimelineLanesNode,
  UIViewNode,
  UIWizardNode,
} from '../ui-page.types.ts'

// =================================================================================================
// Helpers — schema/value resolution + diagnostic for unsupported v1 cases
// =================================================================================================

function resolveSchema(node: UISchemaBoundNode): Schema | { error: string } {
  if (typeof node.schema === 'string') {
    return {
      error: `schema "${node.schema}" is named by id and was not resolved — show this page through the document entry point, which resolves ids.`,
    }
  }
  const action = (node.schema as { action?: unknown } | undefined)?.action
  if (typeof action === 'string') {
    return { error: `schema {action: "${action}"} (the input of an action) was not resolved — a host resolves it before the page renders.` }
  }
  if (node.ui) {
    return { error: `${node.kind} node carries an unresolved ui block — show this page through the document entry point.` }
  }
  if (node.schema) {
    const s = node.schema as Schema
    // A schema missing `root`/`types` would crash deep in SchemaQuery.getType
    // ('type' in undefined), white-screening the whole widget. Degrade to an
    // inline diagnostic so the rest of the page still renders and the author
    // sees exactly which node is malformed.
    if (!s || typeof s !== 'object' || !s.root || !s.types) {
      return {
        error: `${node.kind} node has a malformed schema — it must carry a "root" type name and a "types" map. Got: ${truncateJSON(s, 240)}`,
      }
    }
    return s
  }
  return { error: `${node.kind} node has no schema` }
}

function truncateJSON(v: unknown, max: number): string {
  let s: string
  try { s = JSON.stringify(v) } catch { s = String(v) }
  if (!s) return String(v)
  return s.length > max ? s.slice(0, max) + '…' : s
}

function SchemaErrorNotification({ kind, error, attrs }: { kind: string; error: string; attrs?: Record<string, string> }) {
  return (
    <Notification {...attrs} color="yellow" withCloseButton={false} title={`Cannot render ${kind} node`}>
      {error}
    </Notification>
  )
}

// =================================================================================================
// FormBoundRenderer
// =================================================================================================

export interface FormBoundProps {
  node: UIFormNode
  /** The user edited the form (called only once it is dirty — the first
   *  values carry defaults, not edits). */
  onChange?: (value: unknown) => void
  onSubmit?: (value: unknown) => void
  /** The producer changed the value this form is editing. */
  changedUpstream?: boolean
}

/** How a form node draws its form: create or edit, the value it edits, and
 *  whether it goes without its own heading and toolbar (`hideTitle`: the page
 *  around it titles it). */
export function formViewOptions(node: UIFormNode): { isNew: boolean; item?: Record<string, unknown>; hideTitle: boolean } {
  return {
    isNew: node.mode === 'create',
    item: node.data as Record<string, unknown> | undefined,
    hideTitle: node.hideTitle === true,
  }
}

/** A form carrying an id draws it as its element id (`form:<id>`), whether
 *  it draws its form or why it cannot - what a GUI test finds an answer form
 *  or a modal's form by. */
export function FormBoundRenderer(props: FormBoundProps) {
  const own = dataAttrs(props.node)
  const body = <FormBoundBody {...props} attrs={props.node.id ? undefined : own} />
  return props.node.id ? <div {...own} data-element={`form:${props.node.id}`}>{body}</div> : body
}

/** The form's own actions, run by the form itself: a submit runs the node's
 *  `submit` only, never an extra action. */
const FORM_OWN_ACTIONS = new Set(['submit', 'cancel', 'reset'])

/** formActionProcessor is how a page form runs its extra ui-actions:
 *  one carrying a verb (`uri`) is handed to the host once,
 *  its input the form's value with the action's args laid over - the value's
 *  fields passed as `${value.<field>}`, so what the person typed is laid as
 *  it is, never templated, and the args resolved by the host (`${person}`).
 *  One with no `uri` that is the page's own - it sets the page selection
 *  (`select`), shows a tab, opens a modal or a drawer - is
 *  routed to the page, the form's value its item and `{value}` its extra, so
 *  `${value.<field>}` in its select fills from what was typed. The form's
 *  own actions (submit, cancel, reset) and an action naming neither a method
 *  nor a page's own move run nothing here; undefined when no extra action
 *  can run. */
export function formActionProcessor(
  defs: UIActionRef[] | undefined,
  runOf: ReturnType<typeof useRunAction>,
): UIActionProcessor | undefined {
  const runs = new Map<string, (value: unknown) => void>()
  for (const def of defs ?? []) {
    if (!def || typeof def !== 'object' || !('type' in def) || def.type !== 'ui-action') continue
    const d = def as unknown as { id?: string; label?: unknown; icon?: unknown; uri?: unknown; args?: unknown } & Pick<UIPageAction, 'select' | 'tab' | 'focus' | 'modal' | 'drawer'>
    if (!d.id || FORM_OWN_ACTIONS.has(d.id)) continue
    const shown = { ...(typeof d.label === 'string' ? { label: d.label } : {}), ...(typeof d.icon === 'string' ? { icon: d.icon } : {}) }
    if (typeof d.uri !== 'string' || d.uri === '') {
      // The page's own: run by the page, the form's value its item.
      const own: UIPageAction = {
        id: d.id, ...shown,
        ...(d.select ? { select: d.select } : {}), ...(d.tab ? { tab: d.tab, ...(d.focus ? { focus: d.focus } : {}) } : {}),
        ...(d.modal ? { modal: d.modal } : {}), ...(d.drawer ? { drawer: d.drawer } : {}),
      }
      if (!pageOwn(own)) continue
      runs.set(d.id, (value) => { runOf(own)?.(value, { value }) })
      continue
    }
    const args = d.args && typeof d.args === 'object' && !Array.isArray(d.args) ? d.args as Record<string, unknown> : {}
    const base: UIPageAction = { id: d.id, uri: d.uri, ...shown }
    const id = d.id
    runs.set(id, (value) => {
      const fields = value && typeof value === 'object' && !Array.isArray(value) ? Object.keys(value as Record<string, unknown>) : []
      const action: UIPageAction = { ...base, args: { ...Object.fromEntries(fields.map((k) => [k, `\${value.${k}}`])), ...args } }
      runOf(action)?.(value, { value })
    })
  }
  if (runs.size === 0) return undefined
  return async (params: UIActionParams) => {
    const run = runs.get(params.action.id)
    if (run) run('item' in params ? params.item : undefined)
  }
}

function FormBoundBody({ node, onChange, onSubmit, changedUpstream, attrs }: FormBoundProps & { attrs?: Record<string, string> }) {
  const runOf = useRunAction()
  const schemaOrError = resolveSchema(node)
  const schema = 'error' in schemaOrError ? undefined : schemaOrError

  const built = useMemo<{ forms?: Record<string, UIForm>; error?: string }>(() => {
    if (!schema) return { error: (schemaOrError as { error: string }).error }
    try { return { forms: buildUIForms({ schema }) } }
    catch (e) { return { error: e instanceof Error ? e.message : String(e) } }
  }, [schema])

  if (built.error) {
    return <SchemaErrorNotification attrs={attrs} kind="form" error={built.error} />
  }
  const forms = built.forms!
  const form = forms[node.variant ?? 'default'] ?? forms['default']
  if (!form) {
    return (
      <SchemaErrorNotification
        attrs={attrs}
        kind="form"
        error={`No form named "${node.variant ?? 'default'}" found in schema "${schema?.name}".`}
      />
    )
  }

  const { isNew, item, hideTitle } = formViewOptions(node)
  const actionProcessor = formActionProcessor(form.definition.actions, runOf)

  const view = (
    <UIFormView
      form={form}
      item={item}
      isNew={isNew}
      hideTitle={hideTitle}
      keepEdits
      onChange={onChange ? (values, meta) => { if (meta.dirty) onChange(values) } : undefined}
      onSubmit={onSubmit}
      actionProcessor={actionProcessor}
      attrs={changedUpstream ? undefined : attrs}
    />
  )
  // A form saved at each change hears its own saves come back: no badge.
  if (!changedUpstream || savesOnChange(form.definition)) return view
  return (
    <Stack {...attrs} gap={4}>
      <Badge color="yellow" variant="light" title="The producer sent a new value for what you are editing; submitting keeps yours.">
        changed upstream
      </Badge>
      {view}
    </Stack>
  )
}

// =================================================================================================
// ViewBoundRenderer
// =================================================================================================

export function ViewBoundRenderer({ node }: { node: UIViewNode }) {
  const schemaOrError = resolveSchema(node)
  const schema = 'error' in schemaOrError ? undefined : schemaOrError

  const built = useMemo<{ views?: Record<string, UIView>; error?: string }>(() => {
    if (!schema) return { error: (schemaOrError as { error: string }).error }
    try { return { views: buildUIViews({ schema }) } }
    catch (e) { return { error: e instanceof Error ? e.message : String(e) } }
  }, [schema])

  if (built.error) {
    return <SchemaErrorNotification attrs={dataAttrs(node)} kind="view" error={built.error} />
  }
  const views = built.views!
  const view = views[node.variant ?? 'default'] ?? views['default']
  if (!view) {
    return (
      <SchemaErrorNotification
        attrs={dataAttrs(node)}
        kind="view"
        error={`No view named "${node.variant ?? 'default'}" found in schema "${schema?.name}".`}
      />
    )
  }

  const item = (node.data ?? {}) as Record<string, unknown>

  return (
    <UIViewView
      attrs={dataAttrs(node)}
      view={view}
      item={item}
      defaultEmptyStrategy={node.emptyStrategy ?? 'em-dash'}
    />
  )
}

// =================================================================================================
// TableBoundRenderer
// =================================================================================================

export function TableBoundRenderer({ node }: { node: UITableNode }) {
  // A row's click runs the node's `open` on it - a row its `when` refuses
  // does not open -, its actions in a last column.
  // A table declaring `select` selects its rows, its `menu` on each row.
  const runOf = useRunAction()
  const open = runOf(node.open)
  const { selection } = useSelection()
  const schemaOrError = resolveSchema(node)
  const schema = 'error' in schemaOrError ? undefined : schemaOrError

  // Build the tables inside a try/catch: a malformed row type (no fields, an
  // unknown type ref, etc.) throws deep in SchemaQuery and would otherwise
  // white-screen the whole widget. Degrade to an inline diagnostic instead.
  // Both hooks run unconditionally (hooks-order safe); conditional returns come
  // after.
  // Tables expect Array data. Defensive: if data is a single
  // object, wrap; if missing, use [].
  // A table paging from the server holds the rows it loaded.
  const paged = usePagedRows(node.data)
  const data: unknown[] = useMemo(() => {
    if (paged.on) return paged.rows
    const v = node.data
    if (Array.isArray(v)) return v
    if (v && typeof v === 'object' && !('load' in v)) return [v]
    return []
  }, [node.data, paged.on, paged.rows])

  const built = useMemo<{ tables?: Record<string, UITable>; error?: string }>(() => {
    if (!schema) return { error: (schemaOrError as { error: string }).error }
    try {
      // Pass `data` so a thin/placeholder schema (row type with no fields) still
      // renders columns inferred from the data keys instead of erroring.
      return { tables: buildUITables({ schema, data }) }
    } catch (e) {
      return { error: e instanceof Error ? e.message : String(e) }
    }
  }, [schema, data])

  if (built.error) {
    return <SchemaErrorNotification attrs={dataAttrs(node)} kind="table" error={built.error} />
  }
  const tables = built.tables!
  const table = tables[node.variant ?? 'default'] ?? tables['default']
  if (!table) {
    return (
      <SchemaErrorNotification
        attrs={dataAttrs(node)}
        kind="table"
        error={`No table named "${node.variant ?? 'default'}" found in schema "${schema?.name}".`}
      />
    )
  }

  const selectKey = table.definition.select
  const selectRun = selectKey ? runOf(selectAction(selectKey)) : undefined
  const { onRowSelect, isSelected } = tableSelection(table, selectRun, selection)
  const menu = table.definition.menu
  const view = <UITableView attrs={paged.on ? undefined : dataAttrs(node)} table={paged.on ? { ...table, pagination: false } : table} data={data} onRowOpen={open ? (row) => open(row) : undefined}
    rowOpens={node.open?.when ? (row) => actionShown(node.open!, row) : undefined}
    onRowSelect={onRowSelect} isSelected={isSelected}
    rowActions={node.actions?.length ? (row) => <ActionButtons actions={node.actions} item={row} /> : undefined}
    rowMenu={menu?.length ? (row) => (
      <ItemContext.Provider value={row}>
        <RowMenu id={`${node.id ?? 'table'}.menu`} entries={menu as unknown as UIMenuItem[]} />
      </ItemContext.Provider>
    ) : undefined} />
  if (!paged.on) return view
  const total = paged.page?.total
  return (
    <Stack {...dataAttrs(node)} gap={0}>
      {view}
      <PagedFooter rest={total !== undefined ? Math.max(0, total - data.length) : undefined} done={paged.done}
        loading={paged.loading} error={paged.error} onMore={paged.more} name={node.id} />
    </Stack>
  )
}

/** tableSelection is how a table whose extension declares `select: <key>`
 *  selects: a row's click runs the page's `select` verb —
 *  `{key, value: ${item.id}}` — and the row whose id is the selection's
 *  value is the selected one. Without `select`, or without a page to run the
 *  verb, a row neither selects nor shows selected. */
export function tableSelection(
  table: { definition: { select?: string }; rowId: (row: any) => string },
  run: ((item?: unknown, extra?: Record<string, unknown>) => void) | undefined,
  selection: Record<string, unknown>,
): { onRowSelect?: (row: unknown) => void; isSelected?: (row: unknown) => boolean } {
  const key = table.definition.select
  if (!key) return {}
  const current = selection[key]
  return {
    onRowSelect: run ? (row) => run(row) : undefined,
    isSelected: (row) => current !== undefined && current !== null && current !== '' && String(current) === table.rowId(row),
  }
}

/** selectAction is the page action a table's `select` key runs on a row. */
export function selectAction(key: string): UIPageAction {
  return { id: `select-${key}`, select: { key, value: '${item.id}' } }
}

// =================================================================================================
// TimelineLanesBoundRenderer
// =================================================================================================

/** The lane timeline: the schema is the lane type, `data`
 *  the lanes. Its variant's `select` key makes a click on a card run the
 *  page's `select` verb — `{key, value: ${item.id}}` — and outlines the card
 *  whose id is the selection's value (the compact table's selection,
 *  tableSelection). */
export function TimelineLanesBoundRenderer({ node, now }: { node: UITimelineLanesNode; now?: Date }) {
  const runOf = useRunAction()
  const { selection } = useSelection()
  const schemaOrError = resolveSchema(node)
  const schema = 'error' in schemaOrError ? undefined : schemaOrError
  const data: unknown[] = Array.isArray(node.data) ? node.data : []

  const built = useMemo<{ timeline?: UITimelineLanes; error?: string }>(() => {
    if (!schema) return { error: (schemaOrError as { error: string }).error }
    try {
      const timeline = buildUITimelineLanes({ schema, ref: node.variant ?? 'default' })
      return timeline ? { timeline } : { error: `No ui-timeline-lanes named "${node.variant ?? 'default'}" found in schema "${schema.name}".` }
    } catch (e) { return { error: e instanceof Error ? e.message : String(e) } }
  }, [schema, node.variant])

  if (built.error) return <SchemaErrorNotification attrs={dataAttrs(node)} kind="timeline-lanes" error={built.error} />
  const timeline = built.timeline!
  const key = timeline.definition.select
  const run = key ? runOf(selectAction(key)) : undefined
  const { onRowSelect, isSelected } = tableSelection({ definition: timeline.definition, rowId: timeline.itemId }, run, selection)
  return <UITimelineLanesView attrs={dataAttrs(node)} timeline={timeline} data={data} onItemSelect={onRowSelect} isSelected={isSelected} now={now} />
}

// =================================================================================================
// WizardBoundRenderer
// =================================================================================================

/** The wizard node; `render` is the page's own node renderer, which draws a
 *  step made of any page node. */
export function WizardBoundRenderer({ node, render }: { node: UIWizardNode; render?: (node: UIPageNode, key: string) => ReactNode }) {
  const { draft } = usePageContext()
  const runOf = useRunAction()
  const schemaOrError = resolveSchema(node)
  const schema = 'error' in schemaOrError ? undefined : schemaOrError

  const built = useMemo<{ wizards?: Record<string, UIWizard>; error?: string }>(() => {
    if (!schema) return { error: (schemaOrError as { error: string }).error }
    try { return { wizards: buildUIWizards({ schema }) } }
    catch (e) { return { error: e instanceof Error ? e.message : String(e) } }
  }, [schema])

  if (built.error) {
    return <SchemaErrorNotification attrs={dataAttrs(node)} kind="wizard" error={built.error} />
  }
  const wizards = built.wizards!
  const wizard = wizards[node.variant ?? 'default'] ?? wizards['default']
  if (!wizard) {
    return (
      <SchemaErrorNotification
        attrs={dataAttrs(node)}
        kind="wizard"
        error={`No wizard named "${node.variant ?? 'default'}" found in schema "${schema?.name}".`}
      />
    )
  }

  const item = node.data as Record<string, unknown> | undefined
  const { complete, cancel } = wizardActions(node, runOf)

  return <UIWizardView attrs={dataAttrs(node)} wizard={wizard} item={item}
    layout={node.layout} current={wizardCurrent(node, draft)}
    onComplete={complete} onCancel={cancel}
    labels={{ complete: node.complete?.label, cancel: node.cancel?.label }}
    renderNode={render ? (n) => render(n, n.id ?? 'wizard-step') : undefined} />
}

/** wizardCurrent is the step a wizard is on when the page says it:
 *  its `current`, else the value at `currentField` in the
 *  page's data; undefined lets the wizard move by « Suivant ». */
export function wizardCurrent(node: Pick<UIWizardNode, 'current' | 'currentField'>, data: unknown): string | undefined {
  if (node.current) return node.current
  if (!node.currentField) return undefined
  const v = resolveJSONPath(data, node.currentField)
  return v === undefined || v === null || v === '' ? undefined : String(v)
}

/** wizardActions are what finishing and leaving run: each
 *  declared action handed to the host with the accumulated value as its
 *  element and `${value}`; undefined when it is not declared or cannot run. */
export function wizardActions(node: Pick<UIWizardNode, 'complete' | 'cancel'>, runOf: ReturnType<typeof useRunAction>): {
  complete?: (value: unknown) => void
  cancel?: () => void
} {
  const done = runOf(node.complete)
  const leave = runOf(node.cancel)
  return {
    complete: done ? (value) => done(value, { value }) : undefined,
    cancel: leave ? () => leave(undefined) : undefined,
  }
}
