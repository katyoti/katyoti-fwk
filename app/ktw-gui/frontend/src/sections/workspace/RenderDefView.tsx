// SPDX-License-Identifier: Apache-2.0
/**
 * RenderDefView — a render definition (render.Def, p7p) shown through the one
 * document entry point.
 *
 * Every object tab that is not a raw field-group goes through here, and so
 * does anything else that holds a Def: an object panel's content, the result
 * of a panel's source method, and (the same shape) what the LLM displays. The
 * Def becomes a document (renderDefDocument): a ui-component — form, view,
 * table, page, or `extension`, a NAMED view registered below — or a document
 * of its own media type (markdown, a file…); the registry picks the engine.
 * What changes while it shows comes as a display panel (sections/panels),
 * never by polling here.
 */

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { Box, Center, Loader } from '@mantine/core'

import { DocumentView } from '../../components/document/DocumentView'
import { registerExtensionViews, type ExtensionView } from '../../components/document/extensionViews'
import { renderDefDocument } from '../../components/document/renderDef'
import type { RenderDef } from '../../generated/render-def'
import { PageHost } from '../../components/compositions/PageHost'
import { type UIPageNode } from '@ktw/display-lib/ui-builders/page/index.ts'
import { main } from '../../../wailsjs/go/models'
import { MethodPanel } from './MethodPanel'
import { AgentContextView } from './AgentContextView'
import { ObjectInformationView, ObjectLinksView } from './ObjectCorePanels'
import { ActionsPanel } from './ActionsPanel'
import { RequestsView } from './RequestsView'
import { SchedulerConsole } from '../scheduler/SchedulerConsole'
import { ContributionsView } from './ContributionsView'
import { AttachedFilesView } from './AttachedFilesView'
import { LifecycleView } from './LifecycleView'
import { DeckThemePanel } from '../../components/compositions/DeckThemePanel'
import { ContentTemplatePanel } from '../../components/compositions/ContentTemplatePanel'
import { usePanelRefresh, type PanelRefresh } from '../../hooks/usePanelRefresh'
import { useAppLanguage } from '../../i18n'
import { ct, ctSchema } from '../../i18n/contrib'
import { isEmptyAnswer } from './emptyState'
import { useDisplayLevel } from '../../hooks/useUniverseGui'
import { pageIdentity, resolvePanelTree } from './resolvePanel'
import { runMethod } from '../../components/compositions/hostServices'
import { LoadedPart, useHostLoad } from '../../hooks/useHostLoad'
import { SectionFailed } from '@ktw/display-lib/ui-builders/page/nodes/section-load.tsx'
import { DisplayPanel } from '../panels/DisplayPanel'
import { panelOfAnswer } from '../panels/panelAnswer'
import { namedTreeDef, needsResolve, partFetchOf, specCacheOf, templateFetchOf } from '../../lib/specCache'

export type { RenderDef } from '../../generated/render-def'

/** What an extension is mounted with: where it is, and its own input. */
export interface ExtensionContext {
  universeName: string
  objectId: string
  objectName: string
  /** The object detail the pane already holds — the core views (information,
   *  links, actions) read it instead of calling again. */
  detail?: main.WorkspaceObjectDetail
  /** Re-read the panel's source — a source panel's view calls it after an
   *  action it ran changed what the source answers. */
  refresh?: () => void
  input: unknown
}

/** UI_EXTENSIONS — the named views. An object-panel names one in
 *  `content: {kind: extension, extension: <name>}`; adding a view is adding a
 *  line here, and every panel, method result or display can then use it. */
const UI_EXTENSIONS: Record<string, (ctx: ExtensionContext) => ReactNode> = {
  // Its input is the source's answer (object-context's AgentContext) — the view calls nothing.
  'agent-context': ({ objectId, input }) => (<AgentContextView input={input} objectId={objectId} />),
  // The pane's core views, declared in ktw-agents' object-panels like any other.
  'object-information': ({ universeName, detail }) => (detail ? <ObjectInformationView detail={detail} universeName={universeName} /> : null),
  'object-links': ({ universeName, objectId, detail }) => (
    <ObjectLinksView links={detail?.links ?? []} universeName={universeName} objectId={objectId} />
  ),
  'object-actions': ({ universeName, objectId, detail }) => (
    <Box p="md" style={{ height: '100%' }}>
      <ActionsPanel actions={detail?.actions ?? []} universeName={universeName} nodeId={objectId} />
    </Box>
  ),
  // Where the object stands and where it may go: its lifecycles, the gates on
  // each move, the history. The view calls CWM itself and reloads after a move.
  'object-lifecycle': ({ universeName, objectId }) => (<LifecycleView universeName={universeName} objectId={objectId} />),
  // A deck theme stored in the project: its sample deck and its colours, fonts and sizes.
  'deck-theme': ({ universeName, objectId }) => (<DeckThemePanel universeName={universeName} objectId={objectId} />),
  // A content template (a « trame »): its parts in order, included section groups expanded and marked.
  'content-template': ({ universeName, objectId }) => (<ContentTemplatePanel universeName={universeName} objectId={objectId} />),
  // The work asked of a module (planification): the view calls RequestService itself.
  requests: ({ universeName, objectId }) => (<RequestsView universeName={universeName} moduleId={objectId} />),
  // The scheduler console (Global → Scheduler): the runs, the groups, the to-dos — it calls the services itself.
  'scheduler-console': ({ universeName }) => (<SchedulerConsole universeName={universeName} />),
  // The files the object attaches (its attaches edges): its input is the
  // object pane's AttachedFiles answer; a file is shown through DocumentView.
  'attached-files': ({ universeName, objectId, input }) => (
    <AttachedFilesView universeName={universeName} objectId={objectId} input={input} />
  ),
  // What the project publishes under its provider token, by kind (the
  // Contributions panel of sys-contributions): its input is
  // project-contributions/Panel's answer; it re-reads on catalog.changed.
  contributions: ({ universeName, input, refresh }) => (
    <ContributionsView universeName={universeName} input={input} refresh={refresh} />
  ),
  // The generic descriptor renderer: its input IS the descriptor.
  'method-panel': ({ universeName, objectId, input }) => (
    <MethodPanel universeName={universeName} nodeId={objectId} descriptor={input} />
  ),
}

// The ui-components engine mounts them by name; the object context it hands
// them is the pane's (its detail typed here).
registerExtensionViews(UI_EXTENSIONS as unknown as Record<string, ExtensionView>)

interface RenderDefViewProps {
  def: RenderDef
  universeName: string
  objectId: string
  objectName: string
  detail?: main.WorkspaceObjectDetail
  /** A page's actions read it (`${id}`, `${target}`); the object's id when absent. */
  scope?: Record<string, unknown>
  /** Re-read the panel's data (after a page's action ran). */
  onRefresh?: () => void
  /** Re-read the panel's source with these args over its input (a page's
   *  query action). */
  onQuery?: (args: Record<string, unknown>) => void
  /** Which panel this is (its id, or the source that answers it): with the
   *  universe, the object and the scope, the page's identity — a tree
   *  resolved for another page is never kept on screen. */
  panel?: string
  /** A declared panel's refresh: a change it matches
   *  reads the page's data again, so its loaded sections load again without
   *  the page leaving the screen. Its place: the object and `module`. */
  refresh?: PanelRefresh
  module?: string
}

/** reread is a page's definition read again for the `n`th time: the same
 *  tree, its data a new value - the page counts it as a re-read (revision)
 *  and its sections load again. The first read is the definition itself. */
export function reread(def: RenderDef, n: number): RenderDef {
  if (n === 0) return def
  const data = def.data
  const again = Array.isArray(data) ? [...data] : data && typeof data === 'object' ? { ...(data as object) } : {}
  return { ...def, data: again }
}

export function RenderDefView({ def, universeName, objectId, objectName, detail, scope, onRefresh, onQuery, panel, refresh, module }: RenderDefViewProps) {
  const [reads, setReads] = useState(0)
  usePanelRefresh(refresh, { universe: universeName, object: objectId, module }, () => setReads((n) => n + 1))
  const effective: RenderDef = useMemo(() => reread(def, reads), [def, reads])
  // A declared panel's page renders in-process, its actions run by the host
  // (PageHost) — once the object pane made its tree self-contained (schemas
  // named by id or by an action's input, ui blocks, options). A chat
  // artifact's page keeps the iframe (UIPage).
  const tree = (effective as { dslTree?: unknown }).dslTree
  const isPage = (effective.kind === 'page' || !effective.kind) && !!tree && typeof tree === 'object'
  const identity = pageIdentity(universeName, objectId, scope ?? { id: objectId }, panel)
  const resolved = useResolvedTree(universeName, identity, isPage ? tree : undefined)
  if (isPage) {
    if (resolved.error) return <SectionFailed id={`resolve:${panel ?? objectId}`} message={resolved.error} onRetry={resolved.retry ?? (() => {})} />
    if (!resolved.tree) return <Center p="md"><Loader size="sm" /></Center>
    return (
      <PageHost node={resolved.tree as UIPageNode} data={effective.data} universeName={universeName}
        scope={scope ?? { id: objectId }} onRefresh={onRefresh} onQuery={onQuery} />
    )
  }
  return (
    <DocumentView
      doc={renderDefDocument(effective)}
      universe={universeName}
      nodeId={objectId}
      host={{ fillHeight: true, context: { universeName, objectId, objectName, detail, refresh: onRefresh } }}
    />
  )
}

/** useResolvedTree answers a page tree as the object pane resolves it
 *  (object-pane/ResolvePanel), through the host's one load: the same tree
 *  when nothing in it needs the pane (needsResolve: no schema by reference,
 *  no ui block, no computed choices - a page answering its forms already
 *  resolved, as « À faire » does, costs no second round trip),
 *  else the resolved one once it arrives
 *  - re-asked only when the tree itself changes, not on each new data. While
 *  a changed tree resolves, the last resolved one stays shown - for the same
 *  page (`identity`) only: a refresh never unmounts the page behind a loader,
 *  and another page never shows the last one's tree. */
function useResolvedTree(universeName: string, identity: string, tree: unknown): { tree?: unknown; error?: string; retry?: () => void } {
  const key = tree === undefined ? '' : JSON.stringify(tree)
  const needs = tree !== undefined && needsResolve(tree)
  const part = useHostLoad<unknown>({
    universe: universeName, identity: `resolve|${identity}`, key: `resolve|${key}`, enabled: needs,
    load: () => resolvePanelTree(universeName, tree),
  })
  if (tree === undefined) return {}
  if (!needs) return { tree }
  return part.error ? { error: part.error, retry: part.retry } : { tree: part.data }
}

/** askedInput is the input a source is called with: the same input as the
 *  last call - a re-read after a change, an action or a retry - carries
 *  `fresh: true`, so a source answering clicks from a kept read (« À faire »)
 *  computes again; another input - a click, a new
 *  part - goes as it is. A source without a `fresh` input ignores it. */
export function askedInput(input: string, last: string | undefined): string {
  if (input !== last) return input
  try {
    const parsed: unknown = JSON.parse(input)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) return JSON.stringify({ ...parsed, fresh: true })
  } catch { /* not an object: as it is */ }
  return input
}

/** SourcePanel — a panel whose content comes from a method, called with the
 *  panel's declared `input`, the object id — or `args` where there is no
 *  object (a global panel's target) —, the person's `language` and the page's
 *  last query (a filter, a saved view: a query action replaces it), again
 *  whenever `revision` changes, after an action its page ran,
 *  and when an object in its `refresh` scope changes (usePanelRefresh). An
 *  answer that names its `kind` IS the component; otherwise it is the `data`
 *  the declared content shows; one naming a display panel (render.Answer
 *  {panel}) mounts DisplayPanel in place while its
 *  switch is on. */
export function SourcePanel({ source, content, args, input: declared, initialQuery, onQueried, onAnswer, revision = 0, refresh, module, universeName, objectId, objectName, detail, empty }: {
  source: string; content?: RenderDef; args?: Record<string, unknown>; revision?: number
  /** The panel's declared empty state, drawn instead of its page when the
   *  source answers nothing to show. */
  empty?: ReactNode
  /** The query the panel was opened with (a navigate action's `query`): the
   *  page's last query until the page makes its own. */
  initialQuery?: Record<string, unknown>
  /** Told each query the page makes, so its holder keeps it (the Object tab
   *  on its payload, a Global panel across a tab switch). */
  onQueried?: (query: Record<string, unknown>) => void
  /** Told each answer of the source (« À faire » takes the rail's count
   *  from its page's answer). */
  onAnswer?: (result: unknown) => void
  /** The panel's declared input, under the call's own. */
  input?: Record<string, unknown>
  /** The panel's declared refresh; `module` is the module it is about (the
   *  object's, or a global panel's target — empty for every module). */
  refresh?: PanelRefresh; module?: string
  universeName: string; objectId: string; objectName: string
  detail?: main.WorkspaceObjectDetail
}) {
  // The person's language rides with the call, so a page built by a Go
  // source answers in it — and re-reads when it changes. A source whose input
  // has no `language` ignores it.
  const language = useAppLanguage()
  // The person's display level rides with it too: a page may offer less at
  // Essentiel (the Objects panel's Advanced search). A source without a
  // `level` input ignores it.
  const level = useDisplayLevel()
  const base = JSON.stringify({ ...(args ?? { id: objectId }), language, level })
  // The page's last query: its args over the source's input, until the next —
  // and only for the object (the target) it was made on.
  const at = `${source}|${base}`
  const [asked, setAsked] = useState<{ at: string; query: Record<string, unknown> }>({ at: '', query: {} })
  const query = asked.at === at ? asked.query : (initialQuery ?? {})
  const onQuery = useCallback((q: Record<string, unknown>) => {
    setAsked({ at, query: q })
    onQueried?.(q)
  }, [at, onQueried])
  const input = JSON.stringify({ ...(declared ?? {}), ...query, ...(args ?? { id: objectId }), language, level })
  // The declared content by value: the pane re-reads its detail on every
  // write, which must not re-read the source — its refresh scope says when.
  const contentKey = content === undefined ? '' : JSON.stringify(content)
  // Loaded through the host's one load: a new source or input
  // shows the placeholder; a re-read (after an action, a refresh, a query)
  // keeps the current page on screen - its open tab, its outcome message -
  // until the new answer replaces its data. A failed load is the error card
  // with « Réessayer ».
  // The input last asked: asked again, it is a re-read (a change, an action,
  // a retry), sent `fresh` so a source keeping its last answer for clicks
  // computes it again.
  const lastAsked = useRef<string | undefined>(undefined)
  const answer = useHostLoad<{ result: unknown; def: RenderDef }>({
    universe: universeName, identity: `source|${source}|${base}`, key: `source|${source}|${input}|${contentKey}`, revision,
    load: async (signal) => {
      const asked = askedInput(input, lastAsked.current)
      lastAsked.current = input
      // A tree named by {uri, hash} - a page template - drawn from the spec
      // cache, Template asked once on a miss.
      const answered = (await runMethod(universeName, source, asked, signal)) ?? {}
      // Its parts read in the person's language (their data types' texts
      // translated); the cache keeps them as answered.
      const localize = (u: string, part: Record<string, unknown>) => ctSchema((uu, path, fb) => ct(uu, path, fb, language), u, part)
      const result = await namedTreeDef(answered, specCacheOf(universeName), templateFetchOf(universeName), partFetchOf(universeName), localize)
      // An answer naming its kind IS the component; otherwise it is the
      // data the declared content shows.
      const whole = result && typeof result === 'object' && typeof (result as { kind?: unknown }).kind === 'string'
      return { result, def: (whole || !content ? result : { ...content, data: result }) as RenderDef }
    },
  })
  // An action run from the page, or a change in the refresh scope, re-reads the source.
  usePanelRefresh(refresh, { universe: universeName, object: objectId, module }, answer.reread)
  const told = useRef(onAnswer)
  told.current = onAnswer
  useEffect(() => { if (answer.data) told.current?.(answer.data.result) }, [answer.data])
  return (
    <LoadedPart id={`source:${source}`} part={answer}>{({ result, def }) => {
      // A display panel draws itself: its open event carries the skeleton.
      const panelId = panelOfAnswer(result)
      if (panelId) return <DisplayPanel universeName={universeName} panelId={panelId} variant="tab" />
      if (empty && isEmptyAnswer(result)) return <>{empty}</>
      return <RenderDefView def={def} universeName={universeName} objectId={objectId} objectName={objectName} detail={detail}
        panel={source} scope={args ?? { id: objectId }} onRefresh={answer.reread} onQuery={onQuery} />
    }}</LoadedPart>
  )
}
