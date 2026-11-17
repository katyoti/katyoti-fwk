// SPDX-License-Identifier: Apache-2.0
// <UIPageView> — recursive renderer for the JSON DSL tree.
//
// Per the display module's decision on where the UI lives (modules/display/decisions/), a DSL artifact
// renders inside a SINGLE page iframe. This component IS that iframe's
// content: a single React tree, single switch on `node.kind`, schema-
// bound nodes inline (no nested iframes).
//
// Three node categories, dispatched uniformly:
//   - LAYOUT (9 kinds)        — recurses via render() into children (a
//                               section loads its own content through the host)
//   - ATOMIC (9 kinds)        — leaf renderers (S3 of display-dsl-framework)
//   - SCHEMA-BOUND (4 kinds)  — calls buildUIxxx() + UIxxxView (S4)
//
// S2 (this commit) ships layout. S3/S4 add atomic + schema-bound by
// inserting cases in this switch — no other file changes needed.

import { useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Drawer, Modal, Notification, Code, Stack } from '@mantine/core'

import type { UIPageNode } from './ui-page.types.ts'
import { applyDataBinding } from './nodes/data-binding.ts'
import { ChipsRenderer, SearchRenderer, SwitchRenderer, withFilter } from './nodes/filters.tsx'
import { ChoiceCardsRenderer, shownWhen, TextFieldRenderer } from './nodes/choice-cards.tsx'
import { TermsRenderer } from './nodes/terms.tsx'
import { SourceSearchRenderer } from './nodes/source-search.tsx'
import { ItemDetailRenderer } from './nodes/item-detail.tsx'
import { ActionBarRenderer } from './nodes/action-bar.tsx'
import { ProgressListRenderer } from './nodes/progress-list.tsx'
import { ProgressRenderer } from './nodes/progress.tsx'
import { LogRenderer } from './nodes/log.tsx'
import { PanelRenderer } from './nodes/panel.tsx'
import { GroupedListRenderer } from './nodes/grouped-list.tsx'
import { DrawioThumbnail } from './nodes/drawio-thumbnail.tsx'
import { usePageData } from './page-data.ts'
import { DismissContext } from '../form/dismiss.ts'
import {
  askTab, DataContext, enclosingPage, findFocus, FOCUS_TRIES, highlight, initialSelection, ItemContext, pageOwn, routePageAction, SelectionContext, TabRequestContext, useItem, usePageContext, useRunAction, useSelection,
  type PageSelection, type TabRequest,
} from './page-context.tsx'
import { PushedSection, SectionEmpty, SectionRendererLoad, sectionMode } from './nodes/section-load.tsx'
import { useHostServices, useProgressive } from '../../ui-core/host/index.tsx'
import { isPushedSection, type SectionProps } from './answer.ts'
import { pageNodeOverride } from './node-overrides.ts'
import { dataAttrs } from './nodes/data-attrs.ts'
import { DrawioView } from '../drawio/index.ts'
import type { UIPageActionEvent } from './ui-page.interactive.ts'
import {
  ButtonRenderer, GanttRenderer, GaugeRenderer, GraphRenderer, GridBoardRenderer, LanesRenderer, ListRenderer, TreeRenderer,
} from './nodes/interactive.tsx'
import { PickerRenderer } from './nodes/picker.tsx'
import { PickerProposalsRenderer } from './nodes/picker-proposals.tsx'
import { StepperRenderer } from './nodes/stepper.tsx'
import { BoardRenderer } from './nodes/board.tsx'
import { MenuRenderer, SelectRenderer } from './nodes/choice.tsx'
import { ObjectCardRenderer } from './nodes/object-card.tsx'
import {
  AccordionRenderer,
  BoxRenderer,
  CardRenderer,
  GridRenderer,
  HBoxRenderer,
  SectionRenderer,
  SplitRenderer,
  TabsRenderer,
  VBoxRenderer,
  type RenderNode,
} from './nodes/layout.tsx'
import {
  AudioRenderer,
  BadgeRenderer,
  CodeBlockRenderer,
  DividerRenderer,
  FileRenderer,
  HeadingRenderer,
  LegendRenderer,
  MonogramRenderer,
  ImageRenderer,
  KPIRenderer,
  LinkRenderer,
  MarkdownRenderer,
  TextRenderer,
  TimelineRenderer,
  VideoRenderer,
} from './nodes/atomic.tsx'
import { ChartRenderer } from './nodes/chart.tsx'
import {
  FormBoundRenderer,
  TableBoundRenderer,
  ViewBoundRenderer,
  WizardBoundRenderer,
  TimelineLanesBoundRenderer,
} from './nodes/schema-bound.tsx'

// =================================================================================================
// UIPageView — top-level recursive renderer
// =================================================================================================

export interface UIPageViewProps {
  /** Root DSL node. Renders the whole tree recursively. */
  node: UIPageNode
  /** Optional outer data tree — the producer's. Nodes with a `dataField`
   *  resolve their bound field from it via JSONPath; a form bound by
   *  `dataField` edits the page's working copy of it (page-data.ts), so the
   *  nodes bound under the same path follow the form as the user types. */
  data?: unknown
  /** A form node submitted: its value and the `dataField` path it is bound
   *  to (absent for an unbound form). */
  onSubmit?: (value: unknown, meta: { path?: string; nodeId?: string }) => void
  /** The host runs a declared action (a button, an item's action, a drop on
   *  a lane, a click on a bar or a box): it resolves the arguments against
   *  the item and its scope, confirms when asked, runs and refreshes. Absent
   *  (a chat artifact): the page's actions show disabled. */
  onAction?: (event: UIPageActionEvent) => void
}

export function UIPageView({ node, data, onSubmit, onAction }: UIPageViewProps): ReactNode {
  const page = usePageData(data)
  // The page around this one (a section's page sees the page holding the
  // section): a node watching a path its own data lacks reads it there.
  const enclosing = enclosingPage(useContext(DataContext))
  // A `tab` action is the page's own: it shows that tab, the host never sees it.
  const [tab, setTab] = useState<TabRequest>()
  // The selection is the outermost page's (seeded from its data's
  // `selection`); a section's page reuses it. `revision` counts the outer
  // data's re-reads: the sections re-read with it.
  const outer = useContext(SelectionContext)
  const [own, setOwn] = useState(() => initialSelection(data))
  const reads = useRef({ data, n: 0 })
  if (reads.current.data !== data) reads.current = { data, n: reads.current.n + 1 }
  const select = useCallback((key: string, value: unknown) =>
    setOwn((s) => (Object.is(s[key], value) ? s : { ...s, [key]: value })), [])
  const mine = useMemo<PageSelection>(() => ({ selection: own, select, revision: reads.current.n }),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [own, select, reads.current.n])
  const sel = outer ?? mine
  // A `select` action is the page's own too: it sets its key of the selection.
  // A drawer holding a node is the page's own too: it draws
  // the node beside the page; an action run from it closes it.
  const [drawer, setDrawer] = useState<{ title?: string; node: UIPageNode; item?: unknown }>()
  // A modal holding a node is the page's own too.
  const [modal, setModal] = useState<{ title?: string; node: UIPageNode; size?: string; item?: unknown }>()
  const run = useMemo(() => (e: UIPageActionEvent) => routePageAction(e, {
    show: (id, focus) => setTab((t) => askTab(t, id, focus)),
    select: sel.select,
    host: (h) => onAction?.(h),
    selection: sel.selection,
    drawer: setDrawer,
    modal: setModal,
  }), [onAction, sel.select, sel.selection])
  // What a modal or a drawer holds may leave it (a form's « Annuler »).
  const closeDrawer = useCallback(() => setDrawer(undefined), [])
  const closeModal = useCallback(() => setModal(undefined), [])
  const inDrawer = useMemo(() => (e: UIPageActionEvent) => {
    run(e)
    if (!pageOwn(e.action)) setDrawer(undefined)
  }, [run])
  const inModal = useMemo(() => (e: UIPageActionEvent) => {
    run(e)
    if (!pageOwn(e.action)) setModal(undefined)
  }, [run])
  // A `tab` action's focus: once its tab is drawn, the
  // element it names is brought into view and highlighted - looked for a
  // little while, a section may still be loading it.
  const root = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const focus = tab?.focus
    if (!focus) return
    let tries = 0
    let timer: ReturnType<typeof setTimeout> | undefined
    const look = () => {
      const el = findFocus(root.current, focus)
      if (el) { highlight(el); return }
      if (++tries < FOCUS_TRIES) timer = setTimeout(look, 100)
    }
    timer = setTimeout(look, 0)
    return () => { if (timer) clearTimeout(timer) }
  }, [tab])
  return (
    <SelectionContext.Provider value={sel}>
      <DataContext.Provider value={{ ...page, onSubmit, onAction: run, hasHost: !!onAction, enclosing }}>
        <TabRequestContext.Provider value={tab}>
          <div ref={root} style={{ display: 'contents' }}>
          {renderNode(node, node.id ?? 'root')}
          </div>
          <Drawer opened={!!drawer} onClose={closeDrawer} position="right" size="lg"
            title={drawer?.title} data-guide="page.drawer">
            {drawer && (
              <DataContext.Provider value={{ ...page, onSubmit, onAction: inDrawer, hasHost: !!onAction, enclosing }}>
                <DismissContext.Provider value={closeDrawer}>
                  {renderNode(drawer.node, drawer.node.id ?? 'drawer')}
                </DismissContext.Provider>
              </DataContext.Provider>
            )}
          </Drawer>
          <Modal opened={!!modal} onClose={closeModal} centered size={modal?.size ?? 'md'}
            title={modal?.title} data-guide="page.modal">
            {modal && (
              <DataContext.Provider value={{ ...page, onSubmit, onAction: inModal, hasHost: !!onAction, enclosing }}>
                <ItemContext.Provider value={modal.item}>
                  <DismissContext.Provider value={closeModal}>
                    {renderNode(modal.node, modal.node.id ?? 'modal')}
                  </DismissContext.Provider>
                </ItemContext.Provider>
              </DataContext.Provider>
            )}
          </Modal>
        </TabRequestContext.Provider>
      </DataContext.Provider>
    </SelectionContext.Provider>
  )
}

// =================================================================================================
// renderNode — the single switch. Called recursively via the RenderNode
// closure passed down to layout components.
// =================================================================================================

const renderNode: RenderNode = (node, key) => {
  // Re-export the dispatcher with a key wrapper so children get stable
  // React keys without the layout components having to manage them.
  return <SectionGate key={key} node={node} />
}

// A section on a progressive page loads its content through the host,
// and a load-only section (an empty box fed by its load)
// wherever the host can load; every other node, and every node with the
// host's progressive flag off, draws as declared.
function SectionGate({ node }: { node: UIPageNode }) {
  const progressive = useProgressive()
  const { loadSection } = useHostServices()
  const mode = sectionMode(node, progressive, !!loadSection)
  if (mode === 'load') {
    return <SectionRendererLoad node={node as UIPageNode & SectionProps} Page={UIPageView} />
  }
  if (mode === 'empty') return <SectionEmpty attrs={dataAttrs(node)} id={(node as { id: string }).id} empty={(node as { empty?: string }).empty} />
  // A section a page panel's producer pushes draws its placeholder until
  // its replace arrives.
  if (isPushedSection(node)) return <PushedSection attrs={dataAttrs(node)} id={node.id} placeholder={node.placeholder} />
  return <NodeDispatch node={node} />
}

function NodeDispatch({ node: rawNode }: { node: UIPageNode }) {
  // Resolve any `dataField` reference against the outer data tree
  // BEFORE switching on kind. The bound value replaces the node's
  // "main" field per its kind (heading.text, kpi.value, etc.) — see
  // applyDataBinding for the per-kind mapping.
  const page = usePageContext()
  // A form's `submit` is an action the host runs with the form's value.
  const submitRun = useRunAction()(rawNode.kind === 'form' ? rawNode.submit : undefined)
  const item = useItem()
  // A form reads the seed (it holds its own edits); every other node reads
  // the working copy, which carries the forms' edits.
  // A node's `filter` keeps the rows passing it against the page selection
  // the page filters set.
  const { selection } = useSelection()
  const node = withFilter(applyDataBinding(rawNode, rawNode.kind === 'form' ? page.seed : page.draft), selection)
  // A node's `when` draws it only while the page selection holds what it
  // names - a create page's note under the card chosen.
  if (!shownWhen(node.when, selection)) return null

  // A host's own renderer for this kind (registerPageNodeRenderer) wins.
  const Override = pageNodeOverride(node.kind)
  if (Override) return <Override node={node} />

  switch (node.kind) {
    // ── Layout (9) ────────────────────────────────────────────────────
    case 'vbox':
      return <VBoxRenderer node={node} render={renderNode} />
    case 'hbox':
      return <HBoxRenderer node={node} render={renderNode} />
    case 'grid':
      return <GridRenderer node={node} render={renderNode} />
    case 'box':
      return <BoxRenderer node={node} render={renderNode} />
    case 'card':
      return <CardRenderer node={node} render={renderNode} />
    case 'section':
      return <SectionRenderer node={node} render={renderNode} />
    case 'tabs':
      return <TabsRenderer node={node} render={renderNode} />
    case 'accordion':
      return <AccordionRenderer node={node} render={renderNode} />
    case 'split':
      return <SplitRenderer node={node} render={renderNode} />

    // ── Atomic (10) ────────────────────────────────────────────────────
    case 'heading':
      return <HeadingRenderer node={node} />
    case 'text':
      return <TextRenderer node={node} />
    case 'divider':
      return <DividerRenderer node={node} />
    case 'badge':
      return <BadgeRenderer node={node} />
    case 'link':
      return <LinkRenderer node={node} />
    case 'image':
      return <ImageRenderer node={node} />
    case 'video':
      return <VideoRenderer node={node} />
    case 'audio':
      return <AudioRenderer node={node} />
    case 'file':
      return <FileRenderer node={node} />
    case 'timeline':
      return <TimelineRenderer node={node} />
    case 'kpi':
      return <KPIRenderer node={node} render={renderNode} />
    case 'code-block':
      return <CodeBlockRenderer node={node} />
    case 'markdown':
      return <MarkdownRenderer node={node} />
    case 'chart':
      return <ChartRenderer node={node} />
    case 'legend':
      return <LegendRenderer node={node} />
    case 'drawio':
      return node.thumbnail
        ? <DrawioThumbnail node={node} />
        : <DrawioView attrs={dataAttrs(node)} value={{ xml: node.xml }} minHeight={node.height ?? 520} />

    // ── Schema-bound (4) — inline delegation, no nested iframe ─────────
    case 'form': {
      const path = rawNode.dataField
      const bound = path !== undefined && !path.includes('*') && page.write !== undefined
      return (
        <FormBoundRenderer
          node={node}
          onChange={bound ? (value) => page.write!(path, value) : undefined}
          onSubmit={submitRun ? (value) => submitRun(item, { value })
            : page.onSubmit ? (value) => page.onSubmit!(value, { path, nodeId: rawNode.id }) : undefined}
          changedUpstream={bound && page.upstream!.has(path)}
        />
      )
    }
    case 'view':
      return <ViewBoundRenderer node={node} />
    case 'table':
      return <TableBoundRenderer node={node} />
    case 'wizard':
      return <WizardBoundRenderer node={node} render={renderNode} />
    case 'timeline-lanes':
      return <TimelineLanesBoundRenderer node={node} />

    // ── Interactive (11) — they act through the host ───────────────────
    case 'button':
      return <ButtonRenderer node={node} />
    case 'list':
      // Grouped by a field of its elements, with its counts.
      return node.groupBy ? <GroupedListRenderer node={node} render={renderNode} /> : <ListRenderer node={node} render={renderNode} />
    case 'tree':
      return <TreeRenderer node={node} render={renderNode} />
    case 'lanes':
      return <LanesRenderer node={node} render={renderNode} />
    case 'gauge':
      return <GaugeRenderer node={node} />
    case 'gantt':
      return <GanttRenderer node={node} />
    case 'graph':
      return <GraphRenderer node={node} />
    case 'stepper':
      return <StepperRenderer node={node} />
    case 'board':
      return <BoardRenderer node={node} render={renderNode} />
    case 'grid-board':
      return <GridBoardRenderer node={node} render={renderNode} />
    case 'picker':
      // The proposal region: a tickable picker declaring it.
      return node.tickable && node.proposals && node.layout !== 'tree' ? <PickerProposalsRenderer node={node} render={renderNode} /> : <PickerRenderer node={node} render={renderNode} />
    case 'select':
      return <SelectRenderer node={node} />
    case 'chips':
      return <ChipsRenderer node={node} />
    case 'search':
      return <SearchRenderer node={node} />
    case 'switch':
      return <SwitchRenderer node={node} />
    case 'choice-cards':
      return <ChoiceCardsRenderer node={node} />
    case 'text-field':
      return <TextFieldRenderer node={node} />
    case 'terms':
      return <TermsRenderer node={node} />
    case 'source-search':
      return <SourceSearchRenderer node={node} />
    case 'item-detail':
      return <ItemDetailRenderer node={node} render={renderNode} />
    case 'monogram':
      return <MonogramRenderer node={node} />
    case 'action-bar':
      return <ActionBarRenderer node={node} />
    case 'progress-list':
      return <ProgressListRenderer node={node} />
    case 'progress':
      return <ProgressRenderer node={node} />
    case 'log':
      return <LogRenderer node={node} />
    case 'panel':
      return <PanelRenderer node={node} Page={UIPageView} />
    case 'menu':
      return <MenuRenderer node={node} />
    case 'object-card':
      return <ObjectCardRenderer node={node} />

    default: {
      // Exhaustiveness: when a new kind is added to UIPageNode, this
      // arm becomes a TS error until handled. Catches drift from the
      // discriminated union in `ui-page.types.ts`.
      const _exhaustive: never = node
      void _exhaustive
      return <UnknownNodeFallback raw={node} />
    }
  }
}

// =================================================================================================
// Unknown-kind fallback — shows the offending JSON for diagnostic
// =================================================================================================

function UnknownNodeFallback({ raw }: { raw: unknown }) {
  return (
    <Notification color="red" withCloseButton={false} title="Unknown node kind">
      <Stack gap={4}>
        <Code block style={{ fontSize: 11, maxHeight: 120, overflow: 'auto' }}>
          {(() => {
            try {
              return JSON.stringify(raw, null, 2)
            } catch {
              return String(raw)
            }
          })()}
        </Code>
      </Stack>
    </Notification>
  )
}
