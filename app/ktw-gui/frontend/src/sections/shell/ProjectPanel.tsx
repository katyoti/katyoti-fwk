// SPDX-License-Identifier: Apache-2.0
/**
 * ProjectPanel — tabbed sidebar for the active workspace.
 *
 * Tabs: OBJECTS | TERMINALS.
 *
 * - WORKSPACE: the CWM context tree. Picking a node sets `activeNodeId` in
 *   the store, which makes MainArea show the node's task gallery. The
 *   tree contents come from CWM (today: still mock until `gui-workspace-
 *   items-tree` lands).
 * - The runs and the sessions live in Global → Runs (the running chats and
 *   the session history with them).
 * - TERMINALS: list of installed shells (cmd / powershell / git-bash / …),
 *   click to launch in an OS-native window at the workspace's HomePath.
 *
 * When no project is selected, an empty state replaces the tabs.
 */
import { useEffect, useMemo, useState } from 'react'
import {
  ActionIcon,
  Avatar,
  Button,
  Center,
  Divider,
  Group,
  Menu,
  Modal,
  NavLink,
  ScrollArea,
  Stack,
  Tabs,
  Text,
  Tooltip,
} from '@mantine/core'
import {
  IconArrowLeft,
  IconChecklist,
  IconBrandDebian,
  IconBrandWindows,
  IconDots,
  IconFileText,
  IconFolderOpen,
  IconFolderPlus,
  IconGitBranch,
  IconHelp,
  IconTopologyStar3,
  IconPlus,
  IconRefresh,
  IconStack,
  IconTerminal2,
  IconTrash,
} from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useTerminalSessionsStore } from '../../store/terminalSessionsSlice'
import type { ContextNode, SidebarTab } from '../../types/project'
import { ContextTree } from '../context/ContextTree'
import { NodeBreadcrumb } from '../context/NodeBreadcrumb'
import { WorkspaceObjectsTree } from '../workspace/WorkspaceObjectsTree'
import { NewAnywhere } from '../workspace/NewAnywhere'
import { TreeSettingsMenu } from '../workspace/TreeSettingsMenu'
import { GetInstalledTerminals } from '../../../wailsjs/go/main/App'
import { PROJECT_PANEL_TOPICS } from '../../guide/surface'
import { useGuideLauncher } from '../../hooks/useGuideLauncher'
import { useT } from '../../i18n'
import { headerIdentity } from './panelHeader'
import { panelMenuItems, type PanelMenuKey } from './panelMenu'
import { CHECK_INSTALL_ELEMENT, RESTART_UNIVERSE_ELEMENT } from '../../elements'
import { offersRestart } from './universeRuntime'
import { useReadyLauncher } from '../ready/ReadyBadge'
import { orchestratorClient } from '../../ktsp'
import { awaitServed } from '../univers/projectMount'
import { notServedText } from '../univers/serveWait'
import { ProjectFolderOpen } from '../../../wailsjs/go/main/App'
import { ConversionMark } from '../project/ConversionMark'

// Mirrors Go-side main.TerminalInfo. The generated wailsjs models are
// regenerated when ktw-gui rebuilds; declaring the shape locally avoids a
// circular chicken-and-egg before the first build picks up app_terminal.go.
interface TerminalInfo {
  id:    string
  label: string
  icon:  string
  path:  string
}

/** Migrate legacy SidebarTab values to canonical ones: every tree tab of the
 *  past ('work', 'chats', 'context', 'workspace') is the OBJECTS tree now, and
 *  the sessions moved to Global → Runs. */
function effectiveTab(tab: SidebarTab): 'objects' | 'terminals' {
  if (tab === 'terminals') return tab
  return 'objects'
}

/** Walk the tree to find the path (root → target). Empty if not found. */
function findPath(root: ContextNode, targetId: string): ContextNode[] {
  if (root.id === targetId) return [root]
  for (const child of root.children ?? []) {
    const sub = findPath(child, targetId)
    if (sub.length > 0) return [root, ...sub]
  }
  return []
}

// ── Main component ───────────────────────────────────────────────────────

export function ProjectPanel() {
  const activeProjectId       = useAppStore((s) => s.activeProjectId)
  const projects              = useAppStore((s) => s.projects)
  const sidebarTab            = useAppStore((s) => s.sidebarTab)
  const setSidebarTab         = useAppStore((s) => s.setSidebarTab)
  const activeNodeId          = useAppStore((s) => s.activeNodeId)
  const clearNode             = useAppStore((s) => s.clearNode)
  const selectNode            = useAppStore((s) => s.selectNode)
  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)
  const universes             = useAppStore((s) => s.universes)
  const { launchGuide }       = useGuideLauncher()
  const { launchReady }       = useReadyLauncher()
  const t                     = useT('shell')
  const tUniverses             = useT('universes')
  const displayedUniverse     = universes.find((u) => u.name === displayedUniverseName)

  const project = projects.find((p) => p.id === activeProjectId)

  // The project folders the displayed universe attaches: what the menu opens
  // and removes (none when the universe is its own project).
  const [attached, setAttached] = useState<string[]>([])
  const [removing, setRemoving] = useState<string | null>(null)
  const [menuError, setMenuError] = useState<string | null>(null)
  // The new-project dialog, opened from « … » (or Ctrl+Shift+N at the universe's level).
  const [newProject, setNewProject] = useState(false)
  useEffect(() => {
    if (!displayedUniverseName) return
    orchestratorClient().listProjects({ universe: displayedUniverseName })
      .then((out) => setAttached(out.projects ?? []))
      .catch(() => setAttached([]))
  }, [displayedUniverseName])

  const menuActions: Record<PanelMenuKey, () => void> = {
    checkInstall: () => launchReady(),
    restart: () => {
      if (displayedUniverseName) void useAppStore.getState().restartUniverse(displayedUniverseName)
    },
    openFolder: () => {
      setMenuError(null)
      void ProjectFolderOpen(displayedUniverseName ?? '', attached[0] ?? '').then((err) => setMenuError(err || null))
    },
    remove: () => setRemoving(attached[0] ?? null),
  }
  const confirmRemove = async () => {
    const dir = removing
    setRemoving(null)
    if (!dir || !displayedUniverseName) return
    try {
      // Detach takes the PROJECT, not the universe: a folder is attached to at most one.
      const out = await orchestratorClient().detachProject({ projectDir: dir })
      setAttached((list) => list.filter((p) => p !== dir))
      setMenuError(null)
      // The detached project is served no more; the universe may serve others.
      await awaitServed(displayedUniverseName, out, { gone: dir })
    } catch (err) {
      setMenuError(notServedText(err, tUniverses))
    }
  }

  // Migrate any persisted-but-now-deprecated tab value to 'objects' on
  // first render so the tab bar stays in sync with the canonical 2-tab
  // model (objects / terminals).
  useEffect(() => {
    if (sidebarTab === 'work' || sidebarTab === 'chats' || sidebarTab === 'context' || sidebarTab === 'workspace' || sidebarTab === 'sessions') {
      setSidebarTab('objects')
    }
  }, [sidebarTab, setSidebarTab])

  if (!project) {
    return (
      <Center style={{ height: '100%', padding: 16 }}>
        <Stack align="center" gap="md">
          <IconArrowLeft size={32} stroke={1.2} color="var(--mantine-color-dimmed)" />
          <Text size="xs" c="dimmed" ta="center">
            {t('project.pickProject')}
          </Text>
        </Stack>
      </Center>
    )
  }

  const tab = effectiveTab(sidebarTab)
  // The header names the universe on display, as the rail does; the project
  // is the tree's root.
  const header = headerIdentity(universes, displayedUniverseName, project)

  return (
    <Stack
      gap={0}
      style={{ height: '100%', width: '100%', minWidth: 0, overflow: 'hidden', background: 'var(--mantine-color-body)' }}
    >
      {/* Project header */}
      <Group gap="xs" px="sm" py={8} wrap="nowrap" style={{ flexShrink: 0 }}>
        <Avatar size={22} radius="md" color={header.color} variant="light" style={{ fontSize: 8, fontWeight: 700 }}>
          {header.icon}
        </Avatar>
        <Text size="sm" fw={600} truncate style={{ flex: 1 }}>
          {header.label}
        </Text>
        {/* Data older than KaTyoti: its mark and « Convertir ». */}
        {displayedUniverseName && <ConversionMark universe={displayedUniverseName} />}
        {/* No « + » here: an object's « + Nouveau » creates under it; the
          * project additions are in « … ». NewAnywhere keeps Ctrl+Shift+N and
          * the dialogs. */}
        {displayedUniverseName && <NewAnywhere universeName={displayedUniverseName} newProject={newProject} onNewProject={setNewProject} />}
        {/* The objects tree's display settings. */}
        {displayedUniverseName && tab === 'objects' && <TreeSettingsMenu />}
        <Tooltip label={t('project.guide')} withArrow>
          <ActionIcon variant="subtle" color="gray" size="xs" aria-label={t('project.guide')} onClick={() => launchGuide(PROJECT_PANEL_TOPICS)}>
            <IconHelp size={14} />
          </ActionIcon>
        </Tooltip>
        <Menu position="bottom-end" shadow="md" width={210}>
          <Menu.Target>
            <ActionIcon variant="subtle" color="gray" size="xs" data-guide="project.menu">
              <IconDots size={14} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            {/* The project additions: a new project, or one already in its own repository linked. */}
            {displayedUniverseName && (
              <>
                <Menu.Item leftSection={<IconFolderPlus size={14} />} onClick={() => setNewProject(true)} data-guide="project.add-project">
                  {t('project.addProject')}
                </Menu.Item>
                <Menu.Divider />
              </>
            )}
            {panelMenuItems(attached, offersRestart(displayedUniverse)).map((item) => (
              <Menu.Item
                key={item.key}
                leftSection={item.key === 'remove' ? <IconTrash size={14} /> : item.key === 'checkInstall' ? <IconChecklist size={14} /> : item.key === 'restart' ? <IconRefresh size={14} /> : <IconFolderOpen size={14} />}
                color={item.key === 'remove' ? 'red' : undefined}
                disabled={item.disabled}
                onClick={menuActions[item.key]}
                data-element={item.key === 'checkInstall' ? CHECK_INSTALL_ELEMENT : item.key === 'restart' ? RESTART_UNIVERSE_ELEMENT : undefined}
              >
                {t(`project.${item.key}`)}
              </Menu.Item>
            ))}
          </Menu.Dropdown>
        </Menu>
      </Group>
      {menuError && (
        <Text size="xs" c="red" px="sm" pb={4}>{menuError}</Text>
      )}

      <Modal opened={removing !== null} onClose={() => setRemoving(null)} title={t('project.removeTitle')} size="sm" centered>
        <Stack gap="sm">
          <Text size="sm">{t('project.removeBody', { project: removing ?? '', universe: displayedUniverseName ?? '' })}</Text>
          <Group justify="flex-end" gap="xs">
            <Button variant="default" size="xs" onClick={() => setRemoving(null)}>{t('project.cancel')}</Button>
            <Button color="red" size="xs" onClick={() => { void confirmRemove() }}>{t('project.remove')}</Button>
          </Group>
        </Stack>
      </Modal>

      <Divider />

      {/* Tabs: the OBJECTS tree, the terminals. */}
      <Tabs
        value={tab}
        onChange={(v) => setSidebarTab(v as SidebarTab)}
        variant="default"
        style={{ flexShrink: 0 }}
        styles={{
          list: { borderBottom: '1px solid var(--mantine-color-default-border)', flexWrap: 'nowrap' },
          tab:  { fontSize: 10, padding: '6px 8px', flexShrink: 1, minWidth: 0 },
        }}
      >
        <Tabs.List px={4} grow>
          <Tabs.Tab value="objects"   data-guide="project.tab" data-guide-name="objects"   leftSection={<IconTopologyStar3 size={11} />}>{t('project.objects')}</Tabs.Tab>
          <Tabs.Tab value="terminals" data-guide="project.tab" data-guide-name="terminals" leftSection={<IconTerminal2 size={11} />}>{t('project.terminals')}</Tabs.Tab>
        </Tabs.List>
      </Tabs>

      {/* Tab content.
        *
        * Each tab's content lives in its own always-mounted div with
        * display toggled by the active tab — equivalent to Mantine's
        * `keepMounted` on Tabs.Panel. Mounting them all the time means
        * universe-scoped panels (WorkspaceObjectsTree)
        * see the `displayedUniverseName` change immediately and re-
        * fetch via their useEffect — fixes the previous "empty until I
        * switch tabs and come back" symptom (unmounted tabs only re-
        * fetched on a fresh mount). */}
      <div style={{ flex: 1, minHeight: 0, overflow: 'hidden', position: 'relative' }}>
        <div style={{ display: tab === 'objects' ? 'block' : 'none', height: '100%' }}>
          {displayedUniverseName ? (
            <ScrollArea style={{ height: '100%' }} scrollbarSize={4}>
              <WorkspaceObjectsTree />
            </ScrollArea>
          ) : (
            <ScrollArea style={{ height: '100%' }} scrollbarSize={4}>
              <ContextTree tree={project.contextTree} />
            </ScrollArea>
          )}
        </div>
        <div style={{ display: tab === 'terminals' ? 'block' : 'none', height: '100%' }}>
          <ScrollArea style={{ height: '100%' }} scrollbarSize={4}><TerminalsTab /></ScrollArea>
        </div>
      </div>

    </Stack>
  )
}

// ── Terminals tab ─────────────────────────────────────────────────────────
//
// Lists shells discovered on the host (cmd, powershell, git bash, wsl, ...).
// Clicking an entry asks the Go side to spawn the shell as a detached
// external window at the displayed universe's `homePath`, so the user lands
// in their workspace ready to type. Phase 2 (gui-embedded-terminal) will
// replace the external window with an in-app xterm.js pane.

function terminalIcon(name: string) {
  switch (name) {
    case 'brand-windows': return <IconBrandWindows size={14} />
    case 'git-branch':    return <IconGitBranch    size={14} />
    case 'brand-debian':  return <IconBrandDebian  size={14} />
    case 'terminal-2':
    default:              return <IconTerminal2    size={14} />
  }
}

/** TERMINALS tab — running terminals + Start picker.
 *  Both log-tails AND interactive shells render in-app via xterm.js.
 *  Shells run as PTY-backed sessions (ConPTY on Windows, creack/pty on
 *  Unix), see `app_shell_pty.go` + `XtermView.tsx`. */
function TerminalsTab() {
  const t = useT('shell')
  const universes             = useAppStore((s) => s.universes)
  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)
  const setMainPaneRoute      = useAppStore((s) => s.setMainPaneRoute)
  const setSidebarTab         = useAppStore((s) => s.setSidebarTab)
  const mainPaneRoute         = useAppStore((s) => s.mainPaneRoute)
  const displayed             = universes.find((u) => u.name === displayedUniverseName)
  const cwd                   = displayed?.homePath ?? ''

  const sessions = useTerminalSessionsStore((s) => s.sessions)
  const startLog = useTerminalSessionsStore((s) => s.startLog)
  const startShell = useTerminalSessionsStore((s) => s.startShell)
  const setActive = useTerminalSessionsStore((s) => s.setActive)

  const [installed, setInstalled] = useState<TerminalInfo[]>([])
  const [installedError, setInstalledError] = useState<string | null>(null)

  // Discover installed shells once. Used to populate the Start picker.
  useEffect(() => {
    let cancelled = false
    GetInstalledTerminals()
      .then((list) => { if (!cancelled) setInstalled(list ?? []) })
      .catch((e: unknown) => { if (!cancelled) setInstalledError(String(e)) })
    return () => { cancelled = true }
  }, [])

  const sessionList = useMemo(
    () => Array.from(sessions.values()).sort((a, b) => a.startedAt - b.startedAt),
    [sessions],
  )

  const onSwitchTo = (id: string) => {
    setActive(id)
    setSidebarTab('terminals')
    setMainPaneRoute({ kind: 'terminal', id })
  }

  const onStartLog = async (source: 'ktw-gui-log') => {
    const id = await startLog(source)
    if (id) onSwitchTo(id)
  }

  const onStartShell = async (shellId: string, label: string) => {
    const id = await startShell(shellId, label, cwd)
    if (id) onSwitchTo(id)
  }

  const activeId = mainPaneRoute.kind === 'terminal' ? mainPaneRoute.id : null
  // One key for the sentence; the icon's slot is cut out of the translation.
  const [emptyBefore, emptyAfter = ''] = t('terminalsTab.empty', { icon: '\u0001' }).split('\u0001')

  return (
    <Stack gap={0} py={4}>
      {/* Header: Start picker */}
      <Group justify="space-between" px="sm" py={4}>
        <Text size="xs" c="dimmed">{t('terminalsTab.running', { count: sessionList.length })}</Text>
        <Menu shadow="md" width={220} position="bottom-end">
          <Menu.Target>
            <ActionIcon size="sm" variant="default" aria-label={t('terminalsTab.start')}>
              <IconPlus size={14} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            <Menu.Label>{t('terminalsTab.logs')}</Menu.Label>
            <Menu.Item
              leftSection={<IconFileText size={14} />}
              onClick={() => void onStartLog('ktw-gui-log')}
            >
              {t('terminalsTab.guiLogs')}
            </Menu.Item>
            {installed.length > 0 && (
              <>
                <Menu.Divider />
                <Menu.Label>{t('terminalsTab.shells')}</Menu.Label>
                {installed.map((term) => (
                  <Menu.Item
                    key={term.id}
                    leftSection={terminalIcon(term.icon)}
                    onClick={() => void onStartShell(term.id, term.label)}
                  >
                    {term.label}
                  </Menu.Item>
                ))}
              </>
            )}
            {installedError && (
              <Menu.Item disabled c="red">
                {t('terminalsTab.error', { error: installedError })}
              </Menu.Item>
            )}
          </Menu.Dropdown>
        </Menu>
      </Group>

      <Divider mx="sm" my={2} />

      {sessionList.length === 0 && (
        <Text size="xs" c="dimmed" px="sm" py="xs">
          {emptyBefore}<IconPlus size={10} style={{ verticalAlign: 'middle' }} />{emptyAfter}
        </Text>
      )}

      {sessionList.map((s) => (
        <NavLink
          key={s.id}
          active={s.id === activeId}
          label={<Text size="xs">{s.label}</Text>}
          description={`${s.kind === 'log' ? t('terminalsTab.kindLog') : t('terminalsTab.kindShell')} · ${s.status}`}
          leftSection={s.kind === 'log' ? <IconFileText size={14} /> : <IconTerminal2 size={14} />}
          onClick={() => onSwitchTo(s.id)}
          styles={{
            root:        { padding: '6px 12px', borderRadius: 4, margin: '0 4px', cursor: 'pointer' },
            description: { fontSize: 10 },
          }}
        />
      ))}

      {cwd && (
        <Text size="xs" c="dimmed" px="sm" py={6} style={{ fontStyle: 'italic' }} mt={4}>
          {t('terminalsTab.cwd', { cwd })}
        </Text>
      )}
    </Stack>
  )
}
