// SPDX-License-Identifier: Apache-2.0
/**
 * WorkspaceObjectsTree — the CWM OBJECT graph for the displayed universe,
 * rendered in the left-pane OBJECTS tab.
 *
 * It runs BESIDE the WORKSPACE tab, which reads nodes and items. The object
 * model is what the method, the grader and the method-overview panel already
 * consume; the node model is what the workspace is still authored through.
 * Keeping both visible during the cut means the two can be compared against
 * each other instead of one being replaced blind.
 *
 * A CLICK OPENS THE OBJECT TAB (the GUI port, 2026-09-25). It used to open
 * nothing, because the only detail pane fetched through `NodeGet`, which takes
 * a node UUID where an object id is `<module>.<local>`. `ObjectTab` reads the
 * object model through `object-pane/Detail`, so the click lands in a pane that
 * speaks this tree's id space — the two are never crossed.
 *
 * Backed by `App.WorkspaceObjectsTree(universeName)` → `cwm/ObjectTree` with
 * `DrawnOnly`, so what is drawn is decided by the `tree:` of
 * each TYPE, never by a list of type names here.
 */
import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Badge,
  Button,
  Center,
  Group,
  Loader,
  Stack,
  Text,
  Tree,
  useTree,
  type RenderTreeNodePayload,
  type TreeNodeData,
} from '@mantine/core'
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react'
import { useDisplayLevel, useUniverseGui } from '../../hooks/useUniverseGui'
import { visibleObjects } from './systemModules'
import { useAppStore } from '../../store'
import { WorkspaceObjectsTree as WorkspaceObjectsTreeFn } from '../../../wailsjs/go/main/App'
import type { main } from '../../../wailsjs/go/models'
import { iconForType, colorForType } from './type-style'
import { treeRowBadges } from './treeSettings'
import { useTreePrefs } from '../../hooks/useTreePrefs'
import { useTypeStyles } from '../../hooks/useTypeStyles'
import { useHostLoad } from '../../hooks/useHostLoad'
import { SectionFailed } from '@ktw/display-lib/ui-builders/page/nodes/section-load.tsx'
import { openObjectTab } from './openObjectTab'
import { contribURI, useCT } from '../../i18n/contrib'
import { useT } from '../../i18n'
import { orchestratorClient } from '../../ktsp'
import { useTabsStore } from '../../store/tabsSlice'
import { openObjectId } from './treeSelection'
import { groupModulesByType, groupsToOpen, isGroupId } from './treeGroups'
import { projectRefusal, retryUnlessRefused } from './projectRefusal'
import { ProjectMark } from './projectMark'
import { objectRef } from './objectRef'

interface ObjTreeNode extends TreeNodeData {
  children: ObjTreeNode[]
}

/** Rebuild parent/child structure from the depth-first flat list, the same
 *  contract the node tree uses — one helper shape, two models. */
export function buildObjectTree(flat: main.WorkspaceObject[]): ObjTreeNode[] {
  if (flat.length === 0) return []
  const roots: ObjTreeNode[] = []
  const stack: { depth: number; node: ObjTreeNode }[] = []
  for (const o of flat) {
    const node: ObjTreeNode = {
      // Its id as sent: qualified when the universe names the
      // project holding it - two projects may hold one bare id.
      value: objectRef(o.id, o.project),
      label: o.title || o.id,
      nodeProps: { type: o.type, module: o.module, id: o.id, depth: o.depth, hasErrors: !!o.hasErrors, project: o.project },
      children: [],
    }
    while (stack.length > 0 && stack[stack.length - 1].depth >= o.depth) stack.pop()
    if (stack.length === 0) roots.push(node)
    else stack[stack.length - 1].node.children.push(node)
    stack.push({ depth: o.depth, node })
  }
  return roots
}

const NO_OBJECTS: main.WorkspaceObject[] = []

export function WorkspaceObjectsTree() {
  const universeName = useAppStore((s) => s.displayedUniverseName)
  // Each type's declared icon and colour; the hashed look for a
  // type that declares none.
  const typeStyles = useTypeStyles(universeName)
  // The rows' type badge: a surface the universe can hide, and a setting of
  // the person's (« Show kinds »). No row shows its module.
  const hideBadges = useUniverseGui(universeName).hidden('badge:tree')
  const badges = treeRowBadges(useTreePrefs().settings, hideBadges)
  const treeRevision = useAppStore((s) => s.workspaceTreeRevision)
  const universeRev  = useAppStore((s) => s.universeRevision)
  // The object open in the Object tab: its node is marked.
  const openId = useTabsStore((s) => openObjectId(s.getForUniverse(universeName ?? '')))
  // A type's label in the GUI language; its name when nothing translates it.
  const ct = useCT()
  const t = useT('workspace')

  // Objects are served by the universe's ktw-agents. A universe without it —
  // the admin universe, which manages the others — holds none: say so rather
  // than ask a service that is not there.
  const [servesObjects, setServesObjects] = useState<boolean | null>(null)
  useEffect(() => {
    setServesObjects(null)
    if (!universeName) return
    orchestratorClient().getUniverse({ name: universeName })
      .then((u) => setServesObjects((u.sidecars ?? []).some((s) => s.name === 'ktw-agents')))
      .catch(() => setServesObjects(true)) // unknown: ask, and show what the call says
  }, [universeName, universeRev])

  // The tree through the host's one load, re-read on each write
  // with the tree kept on screen. An unreachable core rejects: retried while
  // it starts, then drawn as the error card it is, with « Réessayer » —
  // never as "No objects." A project the universe refuses is
  // not asked again: its reason is shown at once.
  const tree = useHostLoad<main.WorkspaceObject[]>({
    universe: universeName, key: 'objects-tree', revision: `${treeRevision}|${universeRev}`,
    enabled: servesObjects === true,
    load: async () => (await WorkspaceObjectsTreeFn(universeName!)) ?? [],
    isEmpty: (arr) => arr.length === 0,
    retryErrors: retryUnlessRefused,
  })
  const objects = tree.data ?? NO_OBJECTS
  const { loading, error } = tree

  const level = useDisplayLevel()
  // The universe's linked projects side by side: each project's
  // root object is a root of the tree - one repository, one project - and a
  // click on any node opens its object in place, in the Object tab; nothing
  // switches what the universe serves. A project's modules grouped by type
  // under their plural label, in the order its type gives.
  const data = useMemo(() => groupModulesByType(buildObjectTree(visibleObjects(objects, level)), typeStyles,
    (type) => ct(contribURI('object-type', type), 'label_plural', typeStyles[type]?.labelPlural ?? type)),
  [objects, level, typeStyles, ct])

  // Roots only. A module opens to its goals and use-cases on click — with
  // hundreds of objects, expanding everything would bury the thing the user
  // came to look at.
  const initialExpandedState = useMemo(() => {
    const exp: Record<string, boolean> = {}
    for (const root of data) {
      exp[root.value] = false
      // A group header is open: it is a heading, not a folder to dig in.
      for (const child of root.children) if (isGroupId(child.value)) exp[child.value] = true
    }
    return exp
  }, [data])

  const treeState = useTree({ initialExpandedState })
  // useTree keeps the expanded state of its first render - usually before
  // the objects arrive - so a header is opened when it first shows.
  const openedGroups = useRef(new Set<string>())
  const { expand } = treeState
  useEffect(() => {
    for (const value of groupsToOpen(data, openedGroups.current)) expand(value)
  }, [data, expand])

  if (loading || (universeName && servesObjects === null)) {
    return (
      <Center p="md">
        <Loader size="sm" />
      </Center>
    )
  }
  if (servesObjects === false) {
    return (
      <Stack p="md" gap={4}>
        <Text size="sm" c="dimmed">{t('tree.noObjectsService')}</Text>
      </Stack>
    )
  }
  const refused = projectRefusal(error)
  if (refused) {
    return <ProjectRefused universe={universeName ?? ''} reason={refused} />
  }
  if (error) {
    return (
      <Stack p="md" gap={4}>
        <SectionFailed id="objects-tree" message={`${t('tree.unreadable')} ${error}`} onRetry={tree.retry} />
      </Stack>
    )
  }
  if (data.length === 0) {
    return <EmptyTree universe={universeName ?? ''} />
  }

  return (
    <Tree
      data={data}
      tree={treeState}
      levelOffset={18}
      renderNode={({ node, expanded, hasChildren, elementProps }: RenderTreeNodePayload) => {
        if (node.nodeProps?.group) {
          // A group header: the type's plural label and how many.
          return (
            <Group gap={4} wrap="nowrap" {...elementProps} data-guide="tree.group" data-guide-name={node.value} py={2}
              style={{ ...elementProps.style, cursor: 'pointer', paddingInlineStart: `calc(${6 + (node.nodeProps?.depth as number ?? 0) * 14}px)`, paddingInlineEnd: 6 }}>
              {expanded ? <IconChevronDown size={12} /> : <IconChevronRight size={12} />}
              <Text size="xs" c="dimmed" tt="uppercase" fw={600} style={{ letterSpacing: '0.04em' }}>{node.label}</Text>
              <Text size="xs" c="dimmed">{String(node.nodeProps?.count ?? '')}</Text>
            </Group>
          )
        }
        const type = (node.nodeProps?.type as string) ?? ''
        const Icon = iconForType(type, typeStyles[type])
        // An object CWM found in error is drawn red; its errors are in its
        // Information tab.
        const inError = !!node.nodeProps?.hasErrors
        // The node of the object open in the pane: bold, on the primary light
        // background (it follows the light and dark themes).
        const selected = node.value === openId
        return (
          <Group
            gap={4}
            wrap="nowrap"
            {...elementProps}
            data-guide="tree.node"
            data-guide-name={node.value}
            aria-selected={selected}
            data-selected={selected || undefined}
            py={2}
            style={{
              ...elementProps.style,
              cursor: 'pointer',
              background: selected ? 'var(--mantine-primary-color-light)' : elementProps.style?.background,
              borderRadius: selected ? 4 : undefined,
              // INDENT COMES FROM THE DEPTH, and it has to be added to what
              // Mantine's Tree already computed — setting `px` here replaced
              // that padding outright, which is why every level rendered at
              // the same x and the tree read as a flat list.
              paddingInlineStart: `calc(${6 + (node.nodeProps?.depth as number ?? 0) * 14}px)`,
              paddingInlineEnd: 6,
            }}
            onClick={(e) => {
              elementProps.onClick?.(e)
              if (universeName) openObjectTab(universeName, { id: node.value, title: String(node.label ?? '') })
            }}
          >
            {hasChildren
              ? (expanded ? <IconChevronDown size={12} /> : <IconChevronRight size={12} />)
              : <span style={{ width: 12, display: 'inline-block' }} />}
            <Icon size={13} color={`var(--mantine-color-${inError ? 'red' : colorForType(type, typeStyles[type])}-5)`} />
            <Text size="xs" fw={selected ? 700 : undefined} truncate="end" c={inError ? 'red' : undefined} style={{ flex: 1, minWidth: 0 }}
              title={inError ? t('tree.hasErrors') : undefined}>{node.label}</Text>
            {/* The project holding it, when the universe serves several. */}
            <ProjectMark project={node.nodeProps?.project as string | undefined} guide="tree.project-mark" />
            {badges.type && (
              <Badge size="xs" variant="light" data-guide="tree.type-badge" color={colorForType(type, typeStyles[type])} style={{ textTransform: 'none' }}>
                {ct(contribURI('object-type', type), 'label', type)}
              </Badge>
            )}
          </Group>
        )
      }}
    />
  )
}

/**
 * ProjectRefused — the universe refuses its project:
 * its type needs a solution the universe lacks. The reason says which and what
 * to do; « Open the universe's page » leads to its Solution section, which
 * gives a universe with none a solution.
 */
function ProjectRefused({ universe, reason }: { universe: string; reason: string }) {
  const t = useT('workspace')
  const setMainPaneRoute = useAppStore((s) => s.setMainPaneRoute)
  return (
    <Stack p="md" gap={6} align="flex-start" data-guide="tree.project-refused">
      <Text size="sm" fw={600}>{t('tree.projectRefused')}</Text>
      <Text size="sm" c="dimmed" data-guide="tree.project-refused-reason" style={{ wordBreak: 'break-word' }}>{reason}</Text>
      <Button size="xs" variant="light" data-guide="tree.project-refused-open-universe"
        onClick={() => setMainPaneRoute({ kind: 'edit', name: universe })}>
        {t('tree.openUniversePage')}
      </Button>
    </Stack>
  )
}

/**
 * EmptyTree — what an empty object tree means. A universe holds no object of
 * its own: they live in the project it attaches. With none attached, it says so
 * and opens the universe's page, where its Projects panel attaches one.
 */
function EmptyTree({ universe }: { universe: string }) {
  const t = useT('workspace')
  const setMainPaneRoute = useAppStore((s) => s.setMainPaneRoute)
  const [projects, setProjects] = useState<string[] | null>(null)
  useEffect(() => {
    if (!universe) return
    orchestratorClient().listProjects({ universe })
      .then((out) => setProjects(out.projects ?? []))
      .catch(() => setProjects(null))
  }, [universe])
  if (projects !== null && projects.length === 0) {
    return (
      <Stack p="md" gap={6} align="flex-start">
        <Text size="sm" c="dimmed">{t('tree.noProject')}</Text>
        <Button size="xs" variant="light" onClick={() => setMainPaneRoute({ kind: 'edit', name: universe })}>
          {t('tree.attachProject')}
        </Button>
      </Stack>
    )
  }
  return (
    <Stack p="md" gap={4}>
      <Text size="sm" c="dimmed">{t('tree.empty')}</Text>
      <Text size="xs" c="dimmed">
        {t('tree.readFromBefore')} <code>{'modules/'}</code> {t('tree.readFromAfter')} {/* i18n-ignore: a folder name */}
      </Text>
    </Stack>
  )
}
