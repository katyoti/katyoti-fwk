// SPDX-License-Identifier: Apache-2.0
/**
 * ObjectTab — the detail pane for ONE object.
 *
 * SAME SHELL AS THE NODE TAB, deliberately: `VerticalTabBody` with a left
 * strip, a slim header carrying the title on the left and the action icons on
 * the right, and the shared `ActionsPanel` card grid. Users learn one
 * navigation idiom; a second one invented here would be a second thing to
 * learn and a second thing to keep in step.
 *
 * ONE FETCH. `WorkspaceObjectDetailFetch` → `object-pane/Detail` answers the
 * whole pane in a single round trip, where the node pane needed six.
 *
 * THE STRIP IS THE MODEL'S, NOT THIS FILE'S. Entries come from
 * `detail.tabs` — the panels each effective capability DECLARES (zero, one or
 * many) plus the one derived for a capability that has fields and declares no
 * panel. A marker capability holds no fields and declares no
 * panel, so it draws no tab: the earlier version invented one tab per
 * capability, which is exactly how a "Tree node" tab with nothing in it
 * appeared on screen. Markers are still reported in `detail.capabilities`, so
 * nothing is hidden — they are just not pretending to be panels.
 */
import { objectActionId, objectSectionId, objectTabId } from '../../elements'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ActionIcon, Alert, Badge, Box, Button, Chip, CopyButton, Divider,
  Group, Loader, Menu, Modal, Paper, ScrollArea, Stack, TagsInput, Text, Textarea, TextInput, Title, Tooltip,
} from '@mantine/core'
import {
  IconAlertTriangle, IconArrowLeft, IconCheck,
  IconArchive, IconCopy, IconDots, IconInfoCircle, IconPencil, IconPlayerPlay, IconPlus, IconRefresh, IconTrash,
} from '@tabler/icons-react'
import {
  WorkspaceObjectArchive,
  WorkspaceObjectAspects,
  WorkspaceObjectChildTypes,
  WorkspaceObjectDelete,
  WorkspaceObjectDeleteDetach,
  WorkspaceObjectDetailFetch,
  WorkspaceObjectInbound,
  WorkspaceObjectUpdate,
} from '../../../wailsjs/go/main/App'
import { hardDeleteBlock } from './retire'
import { main } from '../../../wailsjs/go/models'
import { ObjectBadges } from './ObjectBadges'
import { useTabsStore, type OpenTab } from '../../store/tabsSlice'
import { useAppStore } from '../../store'
import { iconForType, colorForType, labelForType } from './type-style'
import { useTypeStyles } from '../../hooks/useTypeStyles'
import { NewMenu, pickOf, type NewPick } from './NewMenu'
import { PageHost } from '../../components/compositions/PageHost'
import { childCardsPage } from './childCards'
import { headerChips, headerParts, typeCarries, addedCapabilities } from './paneHeader'
import { ProjectMark } from './projectMark'
import { objectRef } from './objectRef'
import { VerticalTabBody, type StripEntry } from './VerticalTabBody'
import { drawsGenericHeader, headerPanelRefresh, isHeaderPanel } from './panelOf'
import { closeObjectTab, openObjectTab } from './openObjectTab'
import { backTarget, crumbPayload, crumbsOf, type ObjectTabStep } from './objectTabHistory'
import { objectTabLabel, retitledObjectPayload } from './objectTabTitle'
import { RenderDefView, SourcePanel, type RenderDef } from './RenderDefView'
import { pageIdentity } from './resolvePanel'
import type { PanelRefresh } from '../../hooks/usePanelRefresh'
import { tablerIcon } from '@ktw/display-lib/ui-core/components/icons/index.ts'
import { findFocus, FOCUS_TRIES, highlight } from '@ktw/display-lib/ui-builders/page/page-context.tsx'
import { panelTabBadge, type PaneFocus } from './paneFocus'
import { usePanelCounts, type CountedPanel } from '../../hooks/usePanelCounts'
import { ObjectFieldsForm } from './ObjectFieldsForm'
import { ObjectCreateModal, runCreateTask } from './ObjectCreateModal'
import { CapabilityBadges } from './ObjectCorePanels'
import { contribURI, ctActionLabel, useCT } from '../../i18n/contrib'
import { useT, tFor, type TFn } from '../../i18n'
import { errorText } from '../../i18n/errors'
import { noteRevision, revisionOf } from './objectRevision'
import { MORE_TOGGLE_ID, splitStrip } from './objectTabMore'
import { tabSurface } from '../shell/universeGui'
import { useDisplayLevel, useUniverseGui } from '../../hooks/useUniverseGui'
import { verifyObject } from './objectVerify'

export interface ObjectTabPayload {
  objectId: string
  objectTitle?: string
  /** Persisted strip selection, so switching tabs and coming back lands where
   *  the user left off — the same rule the Node tab has. */
  activeKind?: string
  /** The query a navigation opened a panel with (navigate {panel, query}):
   *  that panel's source is first read with it. */
  panelQuery?: { panel: string; query: Record<string, unknown> }
  /** The steps back to where the person was before each object replaced the
   *  last (objectTabHistory): closing the tab takes the last one. */
  back?: ObjectTabStep[]
  /** The person opened the tabs behind « More… » (objectTabMore). */
  showMore?: boolean
  /** The element a navigation asked to bring into view. */
  focus?: PaneFocus
}

export function ObjectTab({ tab, universeName }: { tab: OpenTab; universeName: string }) {
  const payload = (tab.payload ?? {}) as ObjectTabPayload
  const objectId = payload.objectId ?? ''
  const universeRev = useAppStore((s) => s.universeRevision)
  const bumpWorkspaceTree = useAppStore((s) => s.bumpWorkspaceTree)
  const updateTabPayload = useTabsStore((s) => s.updateTabPayload)
  // The create-task display: a create task's form opens with its parent filled;
  // the pane draws one header only.
  // The source model's one header and form first: for every user.
  const formFirst = true
  // "+ New" (the type picked in the dialog, or preset by a children tab's Add).
  const [creating, setCreating] = useState<{ type?: string } | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [deleteError, setDeleteError] = useState('')
  const [deleting, setDeleting] = useState(false)
  // « Retirer » archives; the hard delete sits under « ⋯ », offered only when
  // nothing points to the object.
  const [confirmRetire, setConfirmRetire] = useState(false)
  const [retireError, setRetireError] = useState('')
  const [retiring, setRetiring] = useState(false)
  const [inbound, setInbound] = useState<string[] | undefined>(undefined)
  const [inboundError, setInboundError] = useState('')

  const [detail, setDetail] = useState<main.WorkspaceObjectDetail | null>(null)
  // The id every call about this object sends: qualified
  // `<key>:<module>.<local>` when the detail names the project holding it (a
  // universe serving several), the tab's id until it is read.
  const sendId = detail && !detail.error && detail.id ? objectRef(detail.id, detail.project) : objectId
  // Contribution texts (panel names, the type's label, action labels) in the
  // GUI language, each falling back to the contribution's own (i18n/contrib.ts).
  const ct = useCT()
  const t = useT('workspace')
  const { expert, hidden } = useUniverseGui(universeName)
  // What the generic header draws at the person's level: mock-up v6's
  // non-expert header at Essentiel, the whole object-levels one above.
  const parts = headerParts(useDisplayLevel())
  const typeStyles = useTypeStyles(universeName)
  const [loading, setLoading] = useState(true)
  const [checkResult, setCheckResult] = useState('')
  // A check runs as the long method it is: its panel drawn under the check.
  const [checkPanel, setCheckPanel] = useState('')
  // The header's edit mode: the object core (name, description, icon, tags).
  // The type's fields and the body are edited in the type's section, each
  // capability's fields in its own.
  const [core, setCore] = useState<{ name: string; description: string; icon: string; tags: string[] } | null>(null)
  const [coreError, setCoreError] = useState('')
  const [savingCore, setSavingCore] = useState(false)

  const load = useCallback(async () => {
    if (!objectId || !universeName) return
    setLoading(true)
    try {
      const d = await WorkspaceObjectDetailFetch(universeName, objectId)
      noteRevision(objectId, d.revision)
      // Opened bare, answered with its project: its saves go by the
      // qualified id, on the revision read here.
      const ref = objectRef(d.id || objectId, d.project)
      if (ref !== objectId) noteRevision(ref, d.revision)
      setDetail(d)
      // Opened by id alone (a page navigating, the parent link) or renamed
      // since: the tab takes the object's name now that it is read — unless
      // it moved on to another object meanwhile. Opened bare and answered
      // with its project, it takes the qualified id too: one
      // key per object, the tree's; the chat's target follows.
      const tabs = useTabsStore.getState()
      const cur = tabs.getForUniverse(universeName).openTabs.find((t) => t.id === tab.id)
      const next = retitledObjectPayload(cur?.payload as ObjectTabPayload | undefined, objectId, d?.title, d.error ? undefined : ref)
      if (next) {
        tabs.updateTabPayload(universeName, tab.id, next, objectTabLabel(next.objectTitle, tFor('workspace')))
        const app = useAppStore.getState()
        if (app.activeNodeId === objectId) {
          if (next.objectId !== objectId) app.selectNode(next.objectId)
          app.setActiveNodeName(next.objectTitle ?? null)
        }
      }
    } finally {
      setLoading(false)
    }
  }, [objectId, universeName, tab.id])

  // A write anywhere (a field saved, a link added, an agent's tool) bumps the
  // tree revision; the pane re-reads with it.
  const treeRev = useAppStore((s) => s.workspaceTreeRevision)
  useEffect(() => { void load() }, [load, universeRev, treeRev])

  // ── the aspect picker ───────────────────────────────────────────────────
  // An aspect is a named capability list added in one move; the picker offers
  // every declared one the object does not carry yet, and a click on a carried
  // chip removes it. Both write the object's `aspects:` list and reload, so
  // the tabs its capabilities bring appear (or go) at once.
  const [declaredAspects, setDeclaredAspects] = useState<main.WorkspaceObjectAspect[]>([])
  const [aspectError, setAspectError] = useState('')
  useEffect(() => {
    if (!universeName) return
    WorkspaceObjectAspects(universeName).then((a) => setDeclaredAspects(a ?? [])).catch(() => setDeclaredAspects([]))
  }, [universeName, universeRev])
  const setAspects = useCallback(async (names: string[]) => {
    setAspectError('')
    const r = await WorkspaceObjectUpdate(universeName, sendId, '', JSON.stringify(names), revisionOf(sendId))
    noteRevision(sendId, r.revision)
    if (r.error) { setAspectError(errorText(r)); return }
    await load()
  }, [universeName, sendId, load])

  // ── the strip ────────────────────────────────────────────────────────────
  // Every entry comes from the detail: declared panels (information, links,
  // actions, agent context, a type's or an extension's own) and one children
  // tab per child type not drawn in the tree. The pane declares no tab of its
  // own. Information — where an object's errors are shown — carries a red
  // "error" tag when it has any. When an aspect lists the object's main tabs,
  // the others (flagged `more`) wait behind a last « More… (n) » entry.
  const split = useMemo(() => splitStrip(
    // A section (the Identity editor, the type's fields, a capability's) is
    // drawn inside Information, not as a tab of its own.
    // A technical tab is a surface the universe or the level can hide - the
    // tab on screen never.
    // A header panel is drawn above the strip, never a tab.
    (detail?.tabs ?? []).filter((tb) => !tb.section && !isHeaderPanel(tb) && (tb.id === payload.activeKind || !hidden(tabSurface(tb.id)))),
    { expert, showMore: !!payload.showMore, activeId: payload.activeKind },
  ), [detail, expert, hidden, payload.showMore, payload.activeKind])
  // A navigation's element: once the panel it opened draws it,
  // it is brought into view and highlighted - looked for a little while, a
  // source panel reads its page first.
  const paneRoot = useRef<HTMLDivElement>(null)
  const focusAsk = payload.focus
  useEffect(() => {
    if (!focusAsk) return
    let tries = 0
    let timer: ReturnType<typeof setTimeout> | undefined
    const look = () => {
      const el = findFocus(paneRoot.current, focusAsk.element)
      if (el) { highlight(el); return }
      if (++tries < FOCUS_TRIES * 3) timer = setTimeout(look, 100)
    }
    timer = setTimeout(look, 0)
    return () => { if (timer) clearTimeout(timer) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusAsk?.n, focusAsk?.element])
  // A declared panel's count action is its tab's badge
  // when the object stores none: called with {id} laid over the panel's
  // input, re-read with every reload of the detail and with the panel's
  // refresh.
  const countedTabs = useMemo<CountedPanel[]>(() => (detail ? split.shown
    .filter((tb) => tb.count && !tb.badge?.label)
    .map((tb) => ({ id: tb.id, count: tb.count, args: { ...(tb.input ?? {}), id: sendId }, refresh: tb.refresh as PanelRefresh | undefined })) : []),
  [detail, split, sendId])
  // Counts are kept per object as sent: two projects' homonyms never share one.
  const countObject = detail ? sendId : undefined
  const countPlace = useMemo(() => ({ universe: universeName ?? '', object: countObject, module: detail?.module }), [universeName, countObject, detail?.module])
  const tabCounts = usePanelCounts(universeName, countedTabs, detail, countPlace)
  const stripEntries = useMemo<StripEntry[]>(() => {
    if (!detail) return []
    const inError = (detail.errors?.length ?? 0) > 0
    const entries = split.shown.map((tb): StripEntry => {
      if (tb.kind === 'children') {
        const group = (detail.children ?? []).find((g) => g.type === tb.childType)
        const Icon = iconForType(tb.childType ?? '')
        // The type names the list (its plural label, else its label), in the
        // GUI language when its translations give it.
        const typeURI = contribURI('object-type', tb.childType)
        return {
          id: tb.id, label: ct(typeURI, 'label_plural', ct(typeURI, 'label', tb.name || tb.id)), icon: <Icon size={14} />,
          accent: colorForType(tb.childType ?? ''), badge: group ? String(group.count) : undefined,
        }
      }
      // Name, icon and tooltip are the DECLARATION's (an object-panel's, or a
      // capability's label for a derived fields tab) — never invented here.
      const Declared = tablerIcon(tb.icon, tb.verify ? IconPlayerPlay : IconInfoCircle)
      const panelURI = tb.origin === 'panel' ? contribURI('object-panel', tb.id.replace(/^tab:/, '')) : undefined
      return {
        id: tb.id,
        label: ct(panelURI, 'name', tb.name || tb.id),
        description: tb.description ? ct(panelURI, 'description', tb.description) : undefined,
        icon: <Declared size={14} />,
        alert: inError && isInformation(tb) ? t('objectTab.errorTag') : undefined,
        accent: tb.origin === 'object' ? 'grape' : tb.origin === 'panel' ? 'teal' : 'gray',
        // A declared panel's tab badge, read from the object,
        // else its count action's.
        ...panelTabBadge(tb.badge, tabCounts[tb.id]),
      }
    })
    if (split.toggle) {
      entries.push({
        id: MORE_TOGGLE_ID,
        label: split.revealed ? t('objectTab.less') : t('objectTab.more', { count: split.moreCount }),
        accent: 'gray', icon: <IconDots size={14} />,
      })
    }
    return entries
  }, [detail, split, ct, t, tabCounts])

  // The detail the panels render, its action labels in the GUI language.
  const shown = useMemo(() => {
    if (!detail) return detail
    const declarers = [contribURI('object-type', detail.type), ...(detail.capabilities ?? []).map((c) => contribURI('capability', c))]
    const tr = (as: main.WorkspaceQuickAction[] | undefined) =>
      as?.map((a) => (a.label ? Object.assign(Object.create(Object.getPrototypeOf(a)), a, { label: ctActionLabel(ct, declarers, a.label) }) : a))
    // A section or an action the universe's view leaves out is not drawn
    // (the view decides them only while its switch is on).
    const kept = (as: main.WorkspaceQuickAction[] | undefined) => as?.filter((a) => !hidden(objectActionId(a.uri)))
    return Object.assign(Object.create(Object.getPrototypeOf(detail)), detail, {
      tabs: (detail.tabs ?? []).filter((tb) => !tb.section || !hidden(objectSectionId(tb.id))),
      actions: tr(kept(detail.actions)) ?? [],
      quickActions: tr(kept(detail.quickActions)),
    }) as main.WorkspaceObjectDetail
  }, [detail, ct, hidden])

  // A tab saved under its former id (`panel:<stem>`) reads as its tab id.
  const persisted = payload.activeKind && objectTabId(payload.activeKind)
  const activeKind = persisted && stripEntries.some((e) => e.id === persisted)
    ? persisted
    : stripEntries[0]?.id || ''
  const setActiveKind = (id: string) => {
    if (id === MORE_TOGGLE_ID) {
      // « Fewer » on a More tab goes back to the first main tab.
      const leaving = split.revealed && split.shown.some((tb) => tb.more && tb.id === activeKind)
      updateTabPayload(universeName, tab.id, { ...payload, showMore: !split.revealed, ...(leaving ? { activeKind: undefined } : {}) })
      return
    }
    updateTabPayload(universeName, tab.id, { ...payload, activeKind: id })
  }
  // A panel's query (a filter, a saved view) is kept on the tab, so going back
  // to this object shows the panel as the person left it.
  const keepQuery = (panel: string) => (query: Record<string, unknown>) => {
    const cur = useTabsStore.getState().getForUniverse(universeName).openTabs.find((x) => x.id === tab.id)
    const p = (cur?.payload ?? {}) as ObjectTabPayload
    if (p.objectId !== objectId) return
    updateTabPayload(universeName, tab.id, { ...p, panelQuery: { panel, query } })
  }
  // Where closing the tab returns: the previous object, or the tab the person
  // opened this one from (a Global panel, the Inbox).
  const back = backTarget(useTabsStore.getState().getForUniverse(universeName), tab.id)
  const backLabel = back ? (back.from === 'home' ? tFor('shell')('tabs.global') : back.label ?? back.from) : ''

  if (loading && !detail) {
    return <Box p="xl" ta="center"><Loader size="sm" /></Box>
  }
  if (!detail) {
    return <Box p="md"><Text size="sm" c="dimmed">{t('objectTab.selectObject')}</Text></Box>
  }
  if (detail.error) {
    return (
      <Box p="md">
        <Alert color="red" icon={<IconAlertTriangle size={16} />} title={t('objectTab.unreadable')}>
          {errorText(detail)}
        </Alert>
      </Box>
    )
  }

  // The look its type declares, else the hashed one.
  const typeStyle = typeStyles[detail.type]
  const TypeIcon = iconForType(detail.type, typeStyle)
  const typeColor = colorForType(detail.type, typeStyle)
  const typeName = ct(contribURI('object-type', detail.type), 'label', detail.typeLabel || labelForType(detail.type, typeStyle))
  const chips = headerChips(detail, typeStyle)
  const carried = typeCarries(detail.capabilityInfo?.length ? detail.capabilityInfo : (detail.capabilities ?? []).map((id) => ({ id } as main.WorkspaceCapabilityInfo)))

  const editCore = () => {
    setCoreError('')
    setCore({ name: detail.title || '', description: detail.description || '', icon: detail.icon || '', tags: detail.tags ?? [] })
  }
  const saveCore = async () => {
    if (!core) return
    setSavingCore(true); setCoreError('')
    try {
      // The core is saved as the `identity` group; the body is left as it is.
      const r = await WorkspaceObjectUpdate(universeName, sendId, JSON.stringify({ identity: core }), '', revisionOf(sendId))
      noteRevision(sendId, r.revision)
      if (r.error) { setCoreError(errorText(r)); return }
      setCore(null)
      bumpWorkspaceTree()
      await load()
    } finally {
      setSavingCore(false)
    }
  }

  // What « + Nouveau » does with a pick: a type opens its creation, a create
  // task runs.
  const pickNew = (pick: NewPick) => pick.kind === 'create'
    ? setCreating({ type: pick.type })
    : runCreateTask(universeName, pick.choice, sendId, detail.title || detail.id, t('create.openingAssistant', { what: pick.choice.label || pick.choice.name }), formFirst)
  // A children tab's shortcut opens the same « + Nouveau » creation of its
  // type.
  const addChild = async (type: string) => {
    const choices = await WorkspaceObjectChildTypes(universeName, sendId).catch(() => [] as main.WorkspaceNodeTypeChoice[])
    const choice = (choices ?? []).find((c) => c.name === type)
    pickNew(choice ? pickOf(choice) : { kind: 'create', type })
  }
  // The breadcrumb back through the objects the person opened this one from:
  // holder › object, each crumb restoring its holder on
  // the tab it showed.
  const payloadNow = tab.payload as ObjectTabPayload | undefined
  // Drawn once the detail shown is the object opened - never over the holder
  // it was opened from, while the child still loads.
  const isOpened = detail.id === objectId || sendId === objectId
  const crumbs = isOpened ? crumbsOf(payloadNow, tab.id) : []
  const crumbBar = crumbs.length > 0 && payloadNow ? (
    <Group gap={4} px="md" pt="xs" wrap="wrap" data-guide="object.breadcrumb" aria-label={t('objectTab.breadcrumb')}>
      {crumbs.map((c) => (
        <Group key={c.step} gap={4} wrap="nowrap">
          <Button size="compact-xs" variant="subtle" color="gray" data-guide="object.crumb" data-guide-name={c.objectId}
            onClick={() => {
              const restored = crumbPayload(payloadNow, c.step)
              if (restored) updateTabPayload(universeName, tab.id, restored, c.title)
            }}>
            {c.title}
          </Button>
          <Text size="xs" c="dimmed">›</Text>
        </Group>
      ))}
      <Text size="xs" fw={600} data-guide="object.crumb-here">{detail.title || detail.id}</Text>
    </Group>
  ) : null

  // ── the header: title left, action icons right ───────────────────────────
  const actionIcons = (
    <Group gap={4}>
      {back && (
        <Tooltip label={t('objectTab.back', { label: backLabel })} withArrow>
          <ActionIcon variant="subtle" onClick={() => closeObjectTab(universeName, tab.id)} aria-label={t('objectTab.back', { label: backLabel })}>
            <IconArrowLeft size={16} />
          </ActionIcon>
        </Tooltip>
      )}
      <Tooltip label={t('objectTab.editTip')} withArrow>
        <ActionIcon variant="subtle" onClick={editCore} aria-label={t('objectTab.edit')} disabled={core !== null} data-guide="object.edit-core">
          <IconPencil size={16} />
        </ActionIcon>
      </Tooltip>
      {/* « + New »: « + » renamed, its list a menu opened on press. */}
      <NewMenu universeName={universeName} parentId={sendId} styles={typeStyles} allows={(id) => !hidden(id)}
        onPick={pickNew} />
      <Tooltip label={t('objectTab.retireTip')} withArrow>
        <ActionIcon variant="subtle" onClick={() => { setRetireError(''); setConfirmRetire(true) }} aria-label={t('objectTab.retire')}
          data-guide="object.retire">
          <IconArchive size={16} />
        </ActionIcon>
      </Tooltip>
      <Menu shadow="md" position="bottom-end" withinPortal onOpen={() => void askInbound()}>
        <Menu.Target>
          <ActionIcon variant="subtle" aria-label={t('objectTab.moreMenu')} data-guide="object.more">
            <IconDots size={16} />
          </ActionIcon>
        </Menu.Target>
        <Menu.Dropdown>
          {(() => {
            const block = hardDeleteBlock(inbound, inboundError, (detail.children ?? []).reduce((n, g) => n + (g.count ?? 0), 0))
            const item = (
              <Menu.Item color="red" leftSection={<IconTrash size={14} />} disabled={block !== undefined}
                onClick={() => { setDeleteError(''); setConfirmDelete(true) }}
                data-guide="object.hard-delete" data-blocked={block}>
                {t('objectTab.hardDelete')}
              </Menu.Item>
            )
            return block === undefined ? item : (
              <Tooltip label={t(`objectTab.hardDeleteBlocked.${block}`, { count: inbound?.length ?? 0, from: (inbound ?? []).slice(0, 3).join(', ') })} withArrow position="left">
                <div>{item}</div>
              </Tooltip>
            )
          })()}
        </Menu.Dropdown>
      </Menu>
      <Tooltip label={t('common.reload')} withArrow>
        <ActionIcon variant="subtle" onClick={() => void load()} aria-label={t('common.reload')}>
          <IconRefresh size={16} />
        </ActionIcon>
      </Tooltip>
    </Group>
  )

  // The header (the object-levels mock-up): its type's
  // icon large, its type as the eyebrow over its name, the chips of its level,
  // the solution it requires, its parent and its aspects, and a « Capabilities »
  // box with what its type carries.
  const header = (
    <Stack gap={0} data-guide="object.header">
      {crumbBar}
      <Box px="md" pt="sm" pb="sm">
        <Group gap="md" align="flex-start" wrap="nowrap">
        <Box data-guide="object.type-icon" style={{
          width: 44, height: 44, borderRadius: 10, display: 'grid', placeItems: 'center', flex: '0 0 auto',
          border: `1.5px solid var(--mantine-color-${typeColor}-6)`, color: `var(--mantine-color-${typeColor}-6)`,
        }}>
          <TypeIcon size={26} />
        </Box>
        <Stack gap="xs" style={{ flex: 1, minWidth: 0 }}>
          <Text data-guide="object.type-eyebrow" size="xs" c="dimmed" {...(parts.eyebrowCaps ? { ff: 'monospace', tt: 'uppercase', style: { letterSpacing: '.06em' } } : {})}>
            {typeName}
          </Text>
          <Group gap="xs" align="center" wrap="nowrap">
            {core ? (
              <TextInput style={{ flex: 1, minWidth: 0 }} size="sm" value={core.name} aria-label={t('objectTab.name')}
                placeholder={t('objectTab.name')} onChange={(e) => setCore({ ...core, name: e.currentTarget.value })} />
            ) : (
              <Title order={3} style={{ flex: 1, minWidth: 0 }} lineClamp={1}>
                {detail.title || detail.id}
              </Title>
            )}
            {(detail.badges?.length ?? 0) > 0
              ? <ObjectBadges badges={detail.badges} objectType={detail.type} guide="object.badges" />
              : detail.status && <Badge size="sm" variant="light" radius="xl" data-guide="object.state" style={{ textTransform: 'none' }}>{detail.status}</Badge>}
            {actionIcons}
          </Group>
          {/* Its level, the solution it requires, its parent, its aspects. */}
          {parts.chips && <Group gap={6} data-guide="object.chips">
            {chips.map((c) => (
              <Badge key={c.kind} size="sm" variant="outline" color="gray" radius="xl" ff="monospace"
                data-guide={`object.chip-${c.kind}`} style={{ textTransform: 'none', ...(c.opens ? { cursor: 'pointer' } : {}) }}
                onClick={c.opens ? () => openObjectTab(universeName, { id: objectRef(c.opens!, detail.project) }) : undefined}
                title={c.opens ? t('objectTab.openParent') : undefined}>
                {t(`objectTab.chip.${c.kind}`, { value: c.value })}
              </Badge>
            ))}
            {!hidden('badge:aspect') && (detail.aspects ?? []).map((tr) => (
              <Tooltip
                key={tr.name}
                withArrow
                label={tr.declared
                  ? t('objectTab.aspectAdds', { what: ct(contribURI('aspect', tr.name), tr.description ? 'description' : 'label', tr.description || tr.label || tr.name), caps: (tr.capabilities ?? []).join(', ') })
                  : t('objectTab.aspectUndeclared')}
              >
                <Chip
                  checked size="xs" color={tr.declared ? 'blue' : 'orange'} variant="outline"
                  styles={{ label: { borderStyle: 'dashed' } }}
                  onChange={() => void setAspects((detail.aspects ?? []).map((a) => a.name).filter((n) => n !== tr.name))}
                >
                  {t('objectTab.chip.aspect', { value: ct(contribURI('aspect', tr.name), 'label', tr.label || tr.name) })}
                </Chip>
              </Tooltip>
            ))}
          </Group>}
          {core && (
            <Stack gap="xs">
              <Textarea size="xs" autosize minRows={2} value={core.description} label={t('objectTab.description')}
                onChange={(e) => setCore({ ...core, description: e.currentTarget.value })} />
              <Group gap="xs" align="flex-end" grow>
                <TextInput size="xs" label={t('objectTab.icon')} placeholder={t('objectTab.iconPlaceholder')} value={core.icon}
                  onChange={(e) => setCore({ ...core, icon: e.currentTarget.value })} />
                <TagsInput size="xs" label={t('objectTab.tags')} value={core.tags} onChange={(tags) => setCore({ ...core, tags })} />
              </Group>
              {coreError && <Alert color="red" variant="light" icon={<IconAlertTriangle size={14} />} p="xs">{coreError}</Alert>}
              <Group justify="flex-end" gap="xs">
                <Button size="xs" variant="default" onClick={() => setCore(null)} disabled={savingCore}>{t('common.cancel')}</Button>
                <Button size="xs" onClick={() => void saveCore()} loading={savingCore} data-guide="object.save-core">{t('common.save')}</Button>
              </Group>
            </Stack>
          )}
          {/* Same fixed-height description slot as the Node tab, so the strip
              below sits at the SAME position whatever the description is. */}
          {!core && parts.description && <Box
            style={{
              height: 'calc(1.4em * 2)', fontSize: 13, lineHeight: 1.4, overflow: 'hidden',
              display: '-webkit-box', WebkitBoxOrient: 'vertical', WebkitLineClamp: 2,
            }}
          >
            <Text size="xs" c="dimmed">{detail.description}</Text>
          </Box>}
          {parts.idLine && <Group gap={6} data-guide="object.id-line">
            {/* The id, copied with the icon right after it — like a session id. */}
            <CopyButton value={detail.id}>
              {({ copied, copy }) => (
                <Tooltip label={copied ? t('objectTab.copied') : t('objectTab.copyId')} withArrow>
                  <Group gap={4} align="center" onClick={copy} style={{ cursor: 'pointer' }}>
                    <Text size="xs" c="dimmed" ff="monospace">{detail.id}</Text>
                    {copied
                      ? <IconCheck size={12} color="var(--mantine-color-green-6)" />
                      : <IconCopy size={12} color="var(--mantine-color-dimmed)" />}
                  </Group>
                </Tooltip>
              )}
            </CopyButton>
            {/* The project holding it, when the universe serves several. */}
            <ProjectMark project={detail.project} guide="object.project-mark" />
            {detail.module && <Badge size="xs" variant="light" color="gray">{detail.module}</Badge>}
            {(detail.tags ?? []).map((t) => (
              <Badge key={t} size="xs" variant="default" style={{ textTransform: 'none' }}>{t}</Badge>
            ))}
            {originLine(detail, t) && <Text size="xs" c="dimmed">· {originLine(detail, t)}</Text>}
            {/* What an aspect or the object adds, then "+ Aspect" to add more. */}
            {!hidden('badge:capability') && <CapabilityBadges detail={detail} only={addedCapabilities} />}
            {(() => {
              const carried = new Set((detail.aspects ?? []).map((a) => a.name))
              const offer = declaredAspects.filter((a) => !carried.has(a.name))
              if (offer.length === 0) return null
              return (
                <Menu shadow="md" withinPortal position="bottom-start">
                  <Menu.Target>
                    <Badge size="xs" variant="outline" color="gray" style={{ cursor: 'pointer' }}
                           leftSection={<IconPlus size={10} />} aria-label={t('objectTab.addAspect')}>
                      {t('objectTab.aspect')}
                    </Badge>
                  </Menu.Target>
                  <Menu.Dropdown>
                    {offer.map((a) => (
                      <Menu.Item key={a.name}
                                 onClick={() => void setAspects([...carried, a.name])}>
                        <Text size="xs" fw={500}>{ct(contribURI('aspect', a.name), 'label', a.label || a.name)}</Text>
                        <Text size="xs" c="dimmed">{t('objectTab.adds', { caps: (a.capabilities ?? []).join(', ') })}</Text>
                      </Menu.Item>
                    ))}
                  </Menu.Dropdown>
                </Menu>
              )
            })()}
            {aspectError && <Text size="xs" c="red">{aspectError}</Text>}
          </Group>}
        </Stack>
        {!hidden('badge:capability') && carried.length > 0 && (
          <Paper withBorder radius="md" px="sm" py={8} data-guide="object.capabilities" style={{ flex: '0 0 auto', maxWidth: 260 }}>
            <Stack gap={6}>
              <Text size="xs" fw={700}>{t('objectTab.capabilitiesBox')}</Text>
              <Group gap={4}><CapabilityBadges detail={detail} only={typeCarries} /></Group>
            </Stack>
          </Paper>
        )}
        </Group>
      </Box>
      <Divider />
    </Stack>
  )

  // ── content per strip entry ──────────────────────────────────────────────
  const activeTab = (detail.tabs ?? []).find((t) => t.id === activeKind)
  const activeGroup = activeTab?.kind === 'children'
    ? (detail.children ?? []).find((g) => g.type === activeTab.childType)
    : undefined

  let content: React.ReactNode = null
  if (activeGroup) {
    content = (
      <ChildrenPanel group={activeGroup} universeName={universeName} project={detail.project}
        onRefresh={() => void load()}
        onAdd={() => void addChild(activeGroup.type)} />
    )
  } else if (activeTab && activeTab.kind === 'content' && activeTab.content) {
    // ONE renderer for every declared panel: content is a render definition.
    content = (
      <RenderDefView
        key={pageIdentity(universeName, sendId, undefined, activeTab.id)} panel={activeTab.id}
        def={activeTab.content as unknown as RenderDef}
        universeName={universeName} objectId={sendId} objectName={detail.title || detail.id}
        detail={shown ?? detail}
      />
    )
  } else if (activeTab && activeTab.kind === 'source' && activeTab.source) {
    content = (
      <SourcePanel
        source={activeTab.source}
        content={activeTab.content as unknown as RenderDef | undefined}
        refresh={(activeTab as { refresh?: PanelRefresh }).refresh}
        input={(activeTab as { input?: Record<string, unknown> }).input}
        initialQuery={payload.panelQuery?.panel === activeTab.id ? payload.panelQuery.query : undefined}
        onQueried={keepQuery(activeTab.id)}
        module={detail.module}
        universeName={universeName} objectId={sendId} objectName={detail.title || detail.id}
        detail={shown ?? detail}
      />
    )
  } else if (activeTab) {
    content = (
      <ObjectFieldsForm
        universeName={universeName}
        objectId={sendId}
        tab={activeTab}
        onSaved={() => void load()}
        result={checkResult}
        checkPanel={checkPanel || undefined}
        onRunCheck={activeTab.verify
          ? async () => {
              const o = await verifyObject(universeName, sendId)
              setCheckPanel('panel' in o ? o.panel : '')
              setCheckResult('text' in o ? o.text : '')
            }
          : undefined}
      />
    )
  }

  // CWM refuses to delete a linked object (naming the links); the second
  // button deletes it and removes those links.
  const doDelete = async (detach = false) => {
    setDeleting(true)
    setDeleteError('')
    try {
      const r = detach
        ? await WorkspaceObjectDeleteDetach(universeName, sendId)
        : await WorkspaceObjectDelete(universeName, sendId)
      if (r.error) { setDeleteError(errorText(r)); return }
      setConfirmDelete(false)
      bumpWorkspaceTree()
      closeObjectTab(universeName, tab.id)
    } finally {
      setDeleting(false)
    }
  }
  const childCount = (detail.children ?? []).reduce((n, g) => n + (g.count ?? 0), 0)
  // « Retirer »: the object archived, its tab closed.
  const doRetire = async () => {
    setRetiring(true)
    setRetireError('')
    try {
      const r = await WorkspaceObjectArchive(universeName, sendId)
      if (r.error) { setRetireError(errorText(r)); return }
      setConfirmRetire(false)
      bumpWorkspaceTree()
      closeObjectTab(universeName, tab.id)
    } finally {
      setRetiring(false)
    }
  }
  // What points to the object, asked as its « ⋯ » menu opens.
  const askInbound = async () => {
    setInbound(undefined)
    setInboundError('')
    const r = await WorkspaceObjectInbound(universeName, sendId)
    if (r.error) setInboundError(r.error)
    else setInbound(r.from ?? [])
  }

  // The object's header panels: declared panels drawn above
  // the strip, under the object's own heading - a title, chips, its
  // workflow's stepper, its next action.
  const headerPanels = (detail.tabs ?? []).filter(isHeaderPanel)
  // The one-header display: one header only - a declared
  // header panel in place of the generic one, the pane's actions kept
  // beside it, and re-read on its object's own events at least.
  const generic = drawsGenericHeader(formFirst, headerPanels.length, core !== null)
  const fullHeader = headerPanels.length === 0 ? header : (
    <Stack gap={0}>
      {generic ? header : (
        <Group justify="flex-end" px="md" pt={4} data-guide="object.header-actions">{actionIcons}</Group>
      )}
      {headerPanels.map((p) => {
        // Its declared refresh - its own object's events at least behind
        // its switch - re-reads it, a source panel or a page whose
        // sections load.
        const refresh = formFirst
          ? headerPanelRefresh((p as { refresh?: PanelRefresh }).refresh) as PanelRefresh
          : (p as { refresh?: PanelRefresh }).refresh
        return (
        <Box key={p.id} data-guide="object.header-panel" data-guide-name={p.id}>
          {p.kind === 'source' && p.source ? (
            <SourcePanel
              source={p.source}
              content={p.content as unknown as RenderDef | undefined}
              refresh={refresh}
              input={(p as { input?: Record<string, unknown> }).input}
              module={detail.module}
              universeName={universeName} objectId={sendId} objectName={detail.title || detail.id}
              detail={shown ?? detail}
            />
          ) : p.content ? (
            <RenderDefView
              key={pageIdentity(universeName, sendId, undefined, p.id)} panel={p.id}
              def={p.content as unknown as RenderDef}
              refresh={refresh} module={detail.module}
              universeName={universeName} objectId={sendId} objectName={detail.title || detail.id}
              detail={shown ?? detail}
            />
          ) : null}
          <Divider />
        </Box>
        )
      })}
    </Stack>
  )

  return (
    <>
      <div ref={paneRoot} style={{ display: 'contents' }}>
      <VerticalTabBody
        // A type whose tabs are its whole pane (pane.only) draws its head in
        // its own panels: no object header.
        header={detail.bare ? null : fullHeader}
        entries={stripEntries}
        activeId={activeKind}
        onChange={setActiveKind}
      >
        {content}
      </VerticalTabBody>
      </div>
      <ObjectCreateModal
        opened={creating !== null}
        onClose={() => setCreating(null)}
        universeName={universeName}
        parentId={sendId}
        parentTitle={detail.title || detail.id}
        presetType={creating?.type}
      />
      <Modal opened={confirmRetire} onClose={() => setConfirmRetire(false)} centered
        title={<Text fw={600}>{t('objectTab.retireTitle', { name: detail.title || detail.id })}</Text>}>
        <Stack gap="sm" data-guide="object.retire-dialog">
          <Text size="sm">{t('objectTab.retireBody')}</Text>
          <Text size="xs" c="dimmed" ff="monospace">{detail.id}</Text>
          {retireError && <Alert icon={<IconAlertTriangle size={16} />} color="red" variant="light">{retireError}</Alert>}
          <Group justify="flex-end" gap="sm">
            <Button variant="default" onClick={() => setConfirmRetire(false)} disabled={retiring}>{t('common.cancel')}</Button>
            <Button onClick={() => void doRetire()} loading={retiring} data-guide="object.retire-confirm">{t('objectTab.retire')}</Button>
          </Group>
        </Stack>
      </Modal>
      <Modal opened={confirmDelete} onClose={() => setConfirmDelete(false)} centered
        title={<Text fw={600}>{t('objectTab.deleteTitle', { name: detail.title || detail.id })}</Text>}>
        <Stack gap="sm">
          <Text size="sm">
            {childCount > 0 ? t('objectTab.deleteBodyChildren', { count: childCount }) : t('objectTab.deleteBody')}
          </Text>
          <Text size="xs" c="dimmed" ff="monospace">{detail.id}</Text>
          {deleteError && <Alert icon={<IconAlertTriangle size={16} />} color="red" variant="light">{deleteError}</Alert>}
          <Group justify="flex-end" gap="sm">
            <Button variant="default" onClick={() => setConfirmDelete(false)} disabled={deleting}>{t('common.cancel')}</Button>
            {deleteError.includes('is linked from') && (
              <Button color="red" variant="light" onClick={() => void doDelete(true)} loading={deleting}>
                {t('objectTab.deleteDetach')}
              </Button>
            )}
            <Button color="red" onClick={() => void doDelete()} loading={deleting}>{t('common.delete')}</Button>
          </Group>
        </Stack>
      </Modal>
    </>
  )
}

/** isInformation — the declared panel drawn by the Information view. */
function isInformation(t: main.WorkspaceObjectTab): boolean {
  return (t.content as unknown as RenderDef | undefined)?.extension === 'object-information'
}

/** originLine — how the object came to be, in one line: when it was made and
 *  when it last changed (derived by the journal, never stored on the object),
 *  and what it was derived from (its derived-from edge). */
function originLine(d: main.WorkspaceObjectDetail, t: TFn): string {
  const day = (iso?: string) => (iso ? new Date(iso).toLocaleDateString() : '')
  const parts: string[] = []
  if (d.createdAt) parts.push(t('objectTab.created', { day: day(d.createdAt) }))
  if (d.updatedAt && d.updatedAt !== d.createdAt) parts.push(t('objectTab.changed', { day: day(d.updatedAt) }))
  if (d.derivedFrom) parts.push(t('objectTab.derivedFrom', { from: d.derivedFrom }))
  return parts.join(' · ')
}

function ChildrenPanel({ group, universeName, project, onAdd, onRefresh }: {
  group: main.WorkspaceObjectChildGroup; universeName: string; onAdd: () => void
  onRefresh?: () => void
  /** The project holding the parent: its children, written bare, are of it. */
  project?: string
}) {
  const t = useT('workspace')
  const page = childCardsPage(group, project, {
    open: t('objectTab.childOpen'), retire: t('objectTab.retire'), retireConfirm: t('objectTab.retireBody'), empty: t('objectTab.childrenEmpty'),
  })
  return (
    <ScrollArea style={{ height: '100%' }}>
      <Box p="md" data-guide="object.children-cards" data-guide-name={group.type}>
        <Stack gap="xs">
          <Group justify="space-between">
            <Text size="xs" c="dimmed">{group.drawn ? t('objectTab.alsoInTree') : ''}</Text>
            <Button size="xs" variant="light" leftSection={<IconPlus size={14} />} onClick={onAdd}
              data-guide="object.children-new" data-guide-name={group.type}>
              {t('objectTab.newButton')}
            </Button>
          </Group>
          <PageHost node={page} universeName={universeName} scope={{}} onRefresh={onRefresh} />
        </Stack>
      </Box>
    </ScrollArea>
  )
}
