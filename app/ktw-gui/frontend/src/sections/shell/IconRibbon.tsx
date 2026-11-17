// SPDX-License-Identifier: Apache-2.0
import { useEffect, useRef, useState } from 'react'
import {
  ActionIcon,
  Avatar,
  Badge,
  Box,
  Divider,
  Group,
  Indicator,
  Menu,
  Stack,
  Text,
  Tooltip,
} from '@mantine/core'
import {
  IconBell,
  IconBug,
  IconBuildingStore,
  IconInbox,
  IconCamera,
  IconCameraOff,
  IconCheck,
  IconChevronRight,
  IconCreditCard,
  IconFolderPlus,
  IconHelp,
  IconKey,
  IconLayoutSidebarLeftCollapse,
  IconLayoutSidebarLeftExpand,
  IconMessageCircle,
  IconMoon,
  IconPalette,
  IconPencil,
  IconPlayerPlay,
  IconPlayerStop,
  IconPlus,
  IconRecordMail,
  IconSettings2,
  IconShieldLock,
  IconSparkles,
  IconTrash,
  IconUser,
  IconVideo,
} from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { orchestratorClient } from '../../ktsp'
import { listUniversesWithStatus } from '../../ktsp/universes'
import { useMediaStore } from '../../store/mediaSlice'
import { useDevStore } from '../../store/devStore'
import { IconBook, IconChecklist, IconLayoutDashboard, IconMessageReport, IconRefresh, IconTerminal2, IconUserQuestion } from '@tabler/icons-react'
import { CHECK_INSTALL_ELEMENT, RESTART_UNIVERSE_ELEMENT } from '../../elements'
import { offersRestart, saysHubKeepsRunning } from './universeRuntime'
import { useReadyLauncher } from '../ready/ReadyBadge'
import { displayName, initials, isPlaceholder, useProfileStore } from '../../store/profileStore'
import { useChatLauncher } from '../../hooks/useChatLauncher'
import { activateOnKey, menuOpenAfter, pressedInGuide } from '../../guide/anchors'
import { useInboxLauncher } from '../../hooks/useInboxLauncher'
import { badgeOf, RAIL_BADGE, RAIL_WIDTH, useRailCount, useRailScreen } from './RailScreen'
import { useMarketplaceLauncher } from '../../hooks/useMarketplaceLauncher'
import { useTabsStore } from '../../store/tabsSlice'
import { useDebugLauncher } from '../../hooks/useDebugLauncher'
import { useGuideLauncher } from '../../hooks/useGuideLauncher'
import { useGuideT } from '../../guide/i18n'
import { CONCEPTS_TOPIC } from '../../guide/recipes'
import { useT } from '../../i18n'
import { profileEntryAvailable } from './profileMenuEntries'
import { useFeedbackStore } from '../../store/feedbackStore'
import type { Universe } from '../../types/universe'
import { railUniverses } from './workUniverses'
import { useDisplayLevel, useSetDisplayLevel, useUniverseGui } from '../../hooks/useUniverseGui'
import { DISPLAY_LEVELS } from '../../types/settings'

/**
 * 44px vertical ribbon — the top-level navigation surface.
 *
 * Universe icons are double-click to activate (single click no-ops to
 * prevent accidental switches). Right-click opens the per-universe
 * context menu (Activate / Edit / Delete). Hover shows the flyout with
 * details + the "Double-click to activate" hint.
 *
 * The `+` icon at the end of the ribbon opens the creation wizard
 * (rendered as a modal — see Stage 3).
 *
 * Every rail control is an element (elements.ts, `rail:<name>`), carried as
 * `data-element`. The displayed universe's Univers.yaml `gui.hide` and its
 * view leave out chat, inbox, marketplace, debug, dev-console, camera and
 * record unless the person's display level is Expert (picked in the profile
 * menu, once for all universes); the others always show.
 */
export function IconRibbon() {
  const projectPaneOpen   = useAppStore((s) => s.projectPaneOpen)
  const settingsModalOpen = useAppStore((s) => s.settingsModalOpen)
  const toggleProjectPane = useAppStore((s) => s.toggleProjectPane)
  const openSettingsModal = useAppStore((s) => s.openSettingsModal)
  const universes              = useAppStore((s) => s.universes)
  const displayedUniverseName  = useAppStore((s) => s.displayedUniverseName)
  const setMainPaneRoute      = useAppStore((s) => s.setMainPaneRoute)
  const openCreationModal      = useAppStore((s) => s.openCreationModal)
  const activateUniverse       = useAppStore((s) => s.activateUniverse)
  const deactivateUniverse     = useAppStore((s) => s.deactivateUniverse)
  const { launchReady }        = useReadyLauncher()

  const cameraOn = useMediaStore((s) => s.cameraStream !== null)
  const recording = useMediaStore((s) => s.recording)
  const toggleCamera = useMediaStore((s) => s.toggleCamera)
  const openFeedback = useFeedbackStore((s) => s.openFeedback)
  const capturingFeedback = useFeedbackStore((s) => s.capturing)
  const guideT = useGuideT()
  const startRecording = useMediaStore((s) => s.startRecording)
  const stopRecording = useMediaStore((s) => s.stopRecording)

  const { launchChat, canLaunch: chatCanLaunch } = useChatLauncher()
  const { launchInbox, canLaunch: inboxCanLaunch } = useInboxLauncher()
  // What the Inbox opens and its badge are the backend's to name:
  // a page and its count, else the built-in Inbox.
  const railScreen = useRailScreen(displayedUniverseName)
  const railNamed = !!railScreen.page
  const todoBadge = badgeOf(useRailCount(displayedUniverseName, railScreen.count))
  // The panels screen: the open display panels.
  const openPanels = () => {
    if (displayedUniverseName) useTabsStore.getState().openTab(displayedUniverseName, { id: 'panels', kind: 'panels', label: t('ribbon.panels'), closeable: true, payload: {} })
  }
  const { launchMarketplace, canLaunch: marketplaceCanLaunch } = useMarketplaceLauncher()
  const { launchDebug, canLaunch: debugCanLaunch }             = useDebugLauncher()

  const devOpen = useDevStore((s) => s.open)
  const toggleDev = useDevStore((s) => s.toggle)
  const t = useT('shell')
  // What the displayed universe hides from the rail, at the person's level.
  const universeGui = useUniverseGui(displayedUniverseName)
  // A rail element the universe or the level hides, or its view leaves out.
  const hidden = (id: string) => universeGui.hidden(id)

  return (
    <Stack
      align="center"
      gap={4}
      style={{
        width: RAIL_WIDTH,
        flexShrink: 0,
        paddingTop: 8,
        paddingBottom: 8,
        height: '100%',
        background: 'var(--mantine-color-body)',
        borderRight: '1px solid var(--mantine-color-default-border)',
      }}
    >
      {/* Sidebar collapse/expand */}
      <Tooltip label={projectPaneOpen ? t('ribbon.closeSidebar') : t('ribbon.openSidebar')} position="right" withArrow>
        <ActionIcon data-element="rail:sidebar"
          variant="subtle"
          color="gray"
          size="md"
          onClick={toggleProjectPane}
          aria-label={t('ribbon.toggleSidebar')}
        >
          {projectPaneOpen
            ? <IconLayoutSidebarLeftCollapse size={18} />
            : <IconLayoutSidebarLeftExpand  size={18} />}
        </ActionIcon>
      </Tooltip>

      <Divider style={{ width: 28 }} />

      {/* Universe icons — the work universes; the admin universe is reached
          through « Gérer les univers ». */}
      {railUniverses(universes).map((u) => (
        <UniverseIcon
          key={u.name}
          universe={u}
          displayed={u.name === displayedUniverseName}
          onActivate={() => { void activateUniverse(u.name) }}
          onDeactivate={() => { void deactivateUniverse(u.name) }}
          onRestart={() => useAppStore.getState().restartUniverse(u.name)}
          onEdit={() => setMainPaneRoute({ kind: 'edit',   name: u.name })}
          onCheckInstall={() => { void activateUniverse(u.name); launchReady(u.name) }}
          onDelete={() => setMainPaneRoute({ kind: 'delete', name: u.name })}
        />
      ))}

      {/* + add or create universe - an element a universe's tags can leave out */}
      {!hidden('rail:add-universe') && <AddUniverseMenu onCreate={openCreationModal} />}

      {/* Push bottom items down */}
      <div style={{ flex: 1 }} />

      {/* Chat — opens the workspace chat task (system:/tasks/chat). Requires
          an active universe. Ctrl+I has the same effect (handler in App.tsx).
          The chat task auto-binds the cwm plugin (skill-enabled + cwm), so the
          agent starts with file/bash/skill tools + the 26 cwm-* tools + the
          current-node block when a node is selected (Phase 3 wires the
          activeNodeId → _target envelope). */}
      {!hidden('rail:chat') && (
        <Tooltip
          label={chatCanLaunch ? t('ribbon.chat') : t('ribbon.chatNeedsUniverse')}
          position="right"
          withArrow
        >
          <ActionIcon data-element="rail:chat"
            variant="subtle"
            color="gray"
            size="md"
            onClick={() => { void launchChat() }}
            disabled={!chatCanLaunch}
            aria-label={t('ribbon.chatAria')}
            data-guide="ribbon.chat"
          >
            <IconMessageCircle size={18} />
          </ActionIcon>
        </Tooltip>
      )}

      {/* Inbox — opens the inbox tab listing pending approval-request
          items the agent wrote via the inbox channel. Resolving an item
          triggers the agent's suspended task to resume. Opens as a tab
          (like Chat), not a full-screen route. */}
      {!hidden('rail:inbox') && (
        <Tooltip
          label={inboxCanLaunch ? (railNamed ? t('ribbon.todo') : t('ribbon.inbox')) : t('ribbon.inboxNeedsUniverse')}
          position="right"
          withArrow
        >
          {/* The count wraps the button: inside it, the button's overflow
              would cut the bubble. */}
          <Indicator label={todoBadge} size={RAIL_BADGE.size} offset={RAIL_BADGE.offset} color="blue"
            disabled={!todoBadge} data-element="rail:todo-count"
            styles={{ indicator: { fontSize: RAIL_BADGE.fontSize, paddingInline: RAIL_BADGE.padding, fontWeight: 600, pointerEvents: 'none' } }}>
            <ActionIcon data-element="rail:inbox"
              variant="subtle"
              color="gray"
              size="md"
              onClick={() => { launchInbox() }}
              disabled={!inboxCanLaunch}
              aria-label={railNamed ? t('ribbon.todo') : t('ribbon.inbox')}
              data-guide="ribbon.inbox"
            >
              <IconInbox size={18} />
            </ActionIcon>
          </Indicator>
        </Tooltip>
      )}

      {/* Panels — the open display panels and the one selected, as the
          approved display-panels mock-up. */}
      {!hidden('rail:panels') && (
        <Tooltip label={displayedUniverseName ? t('ribbon.panels') : t('ribbon.panelsNeedsUniverse')} position="right" withArrow>
          <ActionIcon data-element="rail:panels"
            variant="subtle"
            color="gray"
            size="md"
            onClick={openPanels}
            disabled={!displayedUniverseName}
            aria-label={t('ribbon.panels')}
            data-guide="ribbon.panels"
          >
            <IconLayoutDashboard size={18} />
          </ActionIcon>
        </Tooltip>
      )}

      {/* Marketplace — everything that can be installed, in five sections
          (Infra, Sidecars, Config-sets, Seeds, Solutions). Opens as a tab under the
          active universe; the universe actions target that universe. */}
      {!hidden('rail:marketplace') && (
        <Tooltip
          label={marketplaceCanLaunch ? t('ribbon.marketplace') : t('ribbon.marketplaceNeedsUniverse')}
          position="right"
          withArrow
        >
          <ActionIcon data-element="rail:marketplace"
            variant="subtle"
            color="gray"
            size="md"
            onClick={() => { launchMarketplace('sidecars') }}
            disabled={!marketplaceCanLaunch}
            aria-label={t('ribbon.marketplace')}
            data-guide="ribbon.marketplace"
          >
            <IconBuildingStore size={18} />
          </ActionIcon>
        </Tooltip>
      )}

      {/* Debug — UI demos, KTSP browser, resources, bus trace, workspace
          state. Opens as a tab under the active universe (like Marketplace). */}
      {!hidden('rail:debug') && (
        <Tooltip
          label={debugCanLaunch ? t('ribbon.debug') : t('ribbon.debugNeedsUniverse')}
          position="right"
          withArrow
        >
          <ActionIcon data-element="rail:debug"
            variant="subtle"
            color="gray"
            size="md"
            onClick={() => { launchDebug() }}
            disabled={!debugCanLaunch}
            aria-label={t('ribbon.debug')}
          >
            <IconBug size={18} />
          </ActionIcon>
        </Tooltip>
      )}

      {/* Feedback — the Feedback form (text + a screenshot of the window), on the
          rail so it is reachable whatever panel is open. Never hidden by a
          universe's gui section: feedback must always be possible. Disabled
          tooltip while capturing, so it is not in the screenshot. */}
      <Tooltip label={guideT('feedback.button')} position="right" withArrow disabled={capturingFeedback}>
        <ActionIcon data-element="rail:feedback"
          variant="subtle"
          color="gray"
          size="md"
          onClick={() => void openFeedback()}
          loading={capturingFeedback}
          aria-label={guideT('feedback.button')}
          data-guide="ribbon.feedback"
        >
          <IconMessageReport size={18} />
        </ActionIcon>
      </Tooltip>

      {/* Dev console — toggles the in-app DevPanel (console + IPC + errors). */}
      {!hidden('rail:dev-console') && (
        <Tooltip label={devOpen ? t('ribbon.hideDevConsole') : t('ribbon.devConsole')} position="right" withArrow>
          <ActionIcon data-element="rail:dev-console"
            variant={devOpen ? 'light' : 'subtle'}
            color={devOpen ? 'grape' : 'gray'}
            size="md"
            onClick={toggleDev}
            aria-label={t('ribbon.devConsole')}
          >
            <IconTerminal2 size={18} />
          </ActionIcon>
        </Tooltip>
      )}

      {/* Camera toggle — when on, draggable PiP overlay (CameraOverlay) shows
          the live video feed at root level. The PiP stays visible during
          recording so the captured video includes the presenter's face. */}
      {!hidden('rail:camera') && (
        <Tooltip label={cameraOn ? t('ribbon.cameraOff') : t('ribbon.cameraOn')} position="right" withArrow>
          <ActionIcon data-element="rail:camera" data-guide="ribbon.camera"
            variant={cameraOn ? 'light' : 'subtle'}
            color={cameraOn ? 'teal' : 'gray'}
            size="md"
            onClick={() => void toggleCamera()}
            aria-label={cameraOn ? t('ribbon.cameraOff') : t('ribbon.cameraOn')}
          >
            {cameraOn ? <IconCamera size={18} /> : <IconCameraOff size={18} />}
          </ActionIcon>
        </Tooltip>
      )}

      {/* Record toggle — getDisplayMedia + MediaRecorder. The browser
          shows its own picker for what to capture (this window / a tab /
          the whole screen). Stop → automatic .webm download. */}
      {!hidden('rail:record') && (
        <Tooltip label={recording ? t('ribbon.stopRecording') : t('ribbon.startRecording')} position="right" withArrow>
          <ActionIcon data-element="rail:record"
            variant={recording ? 'filled' : 'subtle'}
            color={recording ? 'red' : 'gray'}
            size="md"
            onClick={() => (recording ? stopRecording() : void startRecording())}
            aria-label={recording ? t('ribbon.stopRecordingAria') : t('ribbon.startRecordingAria')}
            data-guide="ribbon.record"
            data-guide-label={recording ? t('ribbon.stopRecording') : t('ribbon.startRecording')}
          >
            {recording ? <IconRecordMail size={18} /> : <IconVideo size={18} />}
          </ActionIcon>
        </Tooltip>
      )}

      {/* App Settings */}
      <Tooltip label={t('ribbon.appSettings')} position="right" withArrow>
        <ActionIcon data-element="rail:settings"
          variant={settingsModalOpen ? 'light' : 'subtle'}
          color={settingsModalOpen ? 'blue' : 'gray'}
          size="md"
          onClick={() => openSettingsModal('appearance')}
          aria-label={t('ribbon.appSettings')}
        >
          <IconSettings2 size={18} />
        </ActionIcon>
      </Tooltip>

      {/* Profile avatar */}
      <ProfileMenu />
    </Stack>
  )
}

// ── Universe icon ─────────────────────────────────────────────────────────
//
// Single click  → no-op (prevents accidental switches when many universes are running)
// Double click  → onActivate (display this universe)
// Right click   → context menu (Activate / Edit / Delete)
// Hover         → flyout with display name, description, status + double-click hint

interface UniverseIconProps {
  universe:     Universe
  displayed:    boolean
  onActivate:   () => void
  onDeactivate: () => void
  onRestart:    () => Promise<boolean>
  onEdit:       () => void
  onCheckInstall: () => void
  onDelete:     () => void
}

function UniverseIcon({ universe, displayed, onActivate, onDeactivate, onRestart, onEdit, onCheckInstall, onDelete }: UniverseIconProps) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [restarting, setRestarting] = useState(false)
  const t = useT('shell')
  const restart = () => {
    setRestarting(true)
    void onRestart().finally(() => setRestarting(false))
  }
  const accent = `var(--mantine-color-${universe.color}-6)`

  return (
    <Menu
      opened={menuOpen}
      onChange={setMenuOpen}
      position="right-start"
      shadow="md"
      width={240}
      closeOnItemClick
    >
      {/* Hover flyout — separate Tooltip so it doesn't collide with right-click menu */}
      <Tooltip
        position="right-start"
        withArrow
        openDelay={200}
        closeDelay={100}
        offset={6}
        disabled={menuOpen}
        label={
          <Box style={{ minWidth: 200 }}>
            <Group gap="xs" wrap="nowrap" mb={4}>
              <Avatar size={20} radius="md" color={universe.color} variant="light" style={{ fontSize: 8, fontWeight: 700 }}>
                {universe.icon}
              </Avatar>
              <Text size="sm" fw={600} truncate>{universe.displayName || universe.name}</Text>
              {universe.protected && (
                <IconShieldLock size={12} style={{ opacity: 0.6 }} />
              )}
            </Group>
            {universe.description && (
              <Text size="xs" c="dimmed" mb={4} style={{ whiteSpace: 'normal' }}>
                {universe.description}
              </Text>
            )}
            {universe.busRunning && (
              <Group gap={4} mt={6}>
                <Box style={{ width: 6, height: 6, borderRadius: '50%', background: 'var(--mantine-color-teal-6)' }} />
                <Text size="xs" c="teal">{t('universe.running')}</Text>
              </Group>
            )}
            {saysHubKeepsRunning(universe) && (
              <Text size="xs" c="dimmed" mt={4} style={{ whiteSpace: 'normal' }} data-guide="universe.hub-keeps-running">
                {t('universe.hubKeepsRunning')}
              </Text>
            )}
            <Text size="xs" c="dimmed" mt={6} style={{ fontStyle: 'italic' }}>
              {universe.busRunning
                ? t('universe.hintDisplay')
                : t('universe.hintActivate')}
            </Text>
          </Box>
        }
      >
        <Menu.Target>
          <Box
            // single click does NOT activate (UX safety)
            onClick={(e) => { e.preventDefault() }}
            onDoubleClick={(e) => { e.preventDefault(); onActivate() }}
            role="button"
            tabIndex={0}
            aria-label={t('universe.aria', { name: universe.displayName || universe.name })}
            onKeyDown={activateOnKey(onActivate)}
            data-guide="ribbon.universe"
            data-guide-name={universe.name}
            onContextMenu={(e) => {
              e.preventDefault()
              setMenuOpen(true)
            }}
            style={{
              cursor: 'pointer',
              borderRadius: 8,
              padding: 2,
              borderLeft: displayed ? `3px solid ${accent}` : '3px solid transparent',
              transition: 'border-color 120ms ease',
            }}
          >
            <Avatar
              size={30}
              radius="md"
              color={universe.color}
              variant={displayed ? 'filled' : 'light'}
              style={{ fontSize: 9, fontWeight: 700 }}
            >
              {universe.icon}
            </Avatar>
            {/* Running indicator (Stage 4 wires busRunning) */}
            {universe.busRunning && (
              <Box
                style={{
                  width: 6,
                  height: 6,
                  borderRadius: '50%',
                  background: 'var(--mantine-color-teal-6)',
                  position: 'relative',
                  marginTop: -8,
                  marginLeft: 22,
                  boxShadow: '0 0 0 1.5px var(--mantine-color-body)',
                }}
              />
            )}
          </Box>
        </Menu.Target>
      </Tooltip>

      <Menu.Dropdown>
        <Box px="sm" py="xs">
          <Group gap="xs" wrap="nowrap">
            <Avatar size={24} radius="md" color={universe.color} variant="light" style={{ fontSize: 8, fontWeight: 700 }}>
              {universe.icon}
            </Avatar>
            <div style={{ flex: 1, minWidth: 0 }}>
              <Text size="sm" fw={600} truncate>{universe.displayName || universe.name}</Text>
              <Text size="xs" c="dimmed" truncate>{universe.name}</Text>
            </div>
            {universe.busRunning && (
              <Badge size="xs" color="teal" variant="light">{t('universe.runningBadge')}</Badge>
            )}
          </Group>
        </Box>

        {saysHubKeepsRunning(universe) && (
          <Text size="xs" c="dimmed" px="sm" pb="xs" style={{ whiteSpace: 'normal' }} data-guide="universe.hub-keeps-running">
            {t('universe.hubKeepsRunning')}
          </Text>
        )}

        <Menu.Divider />

        <Menu.Item leftSection={<IconPlayerPlay size={14} />} onClick={onActivate}>
          {universe.busRunning ? t('universe.display') : t('universe.activate')}
        </Menu.Item>
        {universe.busRunning && (
          <Menu.Item
            leftSection={<IconPlayerStop size={14} />}
            onClick={onDeactivate}
            disabled={universe.protected}
            title={universe.protected ? t('universe.protectedNoDeactivate') : undefined}
          >
            {t('universe.deactivate')}
          </Menu.Item>
        )}
        {offersRestart(universe) && (
          <Menu.Item
            leftSection={<IconRefresh size={14} />}
            onClick={restart}
            disabled={restarting}
            data-element={RESTART_UNIVERSE_ELEMENT}
          >
            {restarting ? t('universe.restarting') : t('universe.restart')}
          </Menu.Item>
        )}
        <Menu.Item leftSection={<IconPencil size={14} />} onClick={onEdit} data-guide="universe-menu.edit">
          {t('universe.edit')}
        </Menu.Item>
        {/* Always there, never hidden by a level or a tag's view. */}
        <Menu.Item leftSection={<IconChecklist size={14} />} onClick={onCheckInstall} data-element={CHECK_INSTALL_ELEMENT}>
          {t('project.checkInstall')}
        </Menu.Item>
        <Menu.Divider />
        <Menu.Item
          leftSection={<IconTrash size={14} />}
          color="red"
          onClick={onDelete}
          disabled={universe.protected}
        >
          {universe.protected ? t('universe.deleteProtected') : t('universe.delete')}
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  )
}

// ── Profile menu ──────────────────────────────────────────────────────────

function ProfileMenu() {
  const { launchMarketplace, canLaunch: canOpenKeys } = useMarketplaceLauncher()
  const openSettingsModal = useAppStore((s) => s.openSettingsModal)
  const profile = useProfileStore((s) => s.profile)
  const openProfile = useProfileStore((s) => s.openModal)
  const { launchGuide } = useGuideLauncher()
  const openFeedback = useFeedbackStore((s) => s.openFeedback)
  const guideT = useGuideT()
  const t = useT('shell')
  const unset = isPlaceholder(profile)
  const level = useDisplayLevel()
  const setLevel = useSetDisplayLevel()
  // The menu stays open through a press on a playing guide:
  // « Suivant » is a click outside it, and the next step points inside it.
  const [menuOpen, setMenuOpen] = useState(false)
  const lastPressInGuide = useRef(false)
  useEffect(() => {
    const onPress = (e: Event) => { lastPressInGuide.current = pressedInGuide(e.target) }
    document.addEventListener('mousedown', onPress, true)
    document.addEventListener('touchstart', onPress, true)
    // A key (Escape) is no press on the guide: it closes the menu.
    const onKey = () => { lastPressInGuide.current = false }
    document.addEventListener('keydown', onKey, true)
    return () => {
      document.removeEventListener('keydown', onKey, true)
      document.removeEventListener('mousedown', onPress, true)
      document.removeEventListener('touchstart', onPress, true)
    }
  }, [])
  const onMenuChange = (next: boolean) => {
    const open = menuOpenAfter(next, lastPressInGuide.current)
    if (open !== null) setMenuOpen(open)
  }

  // An entry that works stays as it is; one that does not yet is greyed and
  // unclickable, « Bientôt disponible » in its tooltip (profileMenuEntries).
  const soonItem = (id: string, icon: JSX.Element) => profileEntryAvailable(id)
    ? <Menu.Item leftSection={icon} data-guide={`profile-menu.${id}`}>{t(`profile.${id}`)}</Menu.Item>
    : (
      <Tooltip label={t('profile.soon')} position="left" withArrow>
        <div><Menu.Item leftSection={icon} disabled data-guide={`profile-menu.${id}`} data-unavailable="true">{t(`profile.${id}`)}</Menu.Item></div>
      </Tooltip>
    )

  return (
    <Menu
      position="right-end"
      shadow="lg"
      width={260}
      offset={8}
      opened={menuOpen}
      onChange={onMenuChange}
    >
      <Menu.Target>
        <Indicator color="orange" size={8} offset={3} disabled={!unset} withBorder>
          <Avatar
            size={30}
            radius="xl"
            color="violet"
            variant="light"
            style={{ cursor: 'pointer', fontWeight: 700, fontSize: 11 }}
            aria-label={t('profile.aria', { name: displayName(profile) })}
            data-element="rail:profile"
            data-guide="profile.open"
          >
            {initials(profile)}
          </Avatar>
        </Indicator>
      </Menu.Target>

      <Menu.Dropdown>
        {/* Profile header */}
        <Box px="sm" py="xs">
          <Group gap="sm" wrap="nowrap">
            <Avatar size={36} radius="xl" color="violet" variant="light" style={{ fontWeight: 700, fontSize: 13 }}>
              {initials(profile)}
            </Avatar>
            <div style={{ flex: 1, minWidth: 0 }}>
              <Text size="sm" fw={600} truncate>{displayName(profile)}</Text>
              <Text size="xs" c="dimmed" truncate>{profile.email}</Text>
            </div>
          </Group>
        </Box>

        {unset && (
          <Menu.Item
            leftSection={<IconUserQuestion size={14} />}
            color="orange"
            onClick={openProfile}
          >
            {t('profile.setUp')}
          </Menu.Item>
        )}

        <Menu.Divider />

        {/* Account */}
        <Menu.Label>{t('profile.account')}</Menu.Label>
        <Menu.Item leftSection={<IconUser size={14} />} onClick={openProfile} data-guide="profile-menu.profile">{t('profile.profile')}</Menu.Item>
        {/* The keys: collected and imported where a config-set applies. */}
        <Menu.Item leftSection={<IconKey size={14} />} disabled={!canOpenKeys} onClick={() => launchMarketplace('configsets')}>{t('profile.apiKeys')}</Menu.Item>
        {soonItem('usage', <IconCreditCard size={14} />)}

        <Menu.Divider />

        {/* The display level: once for all universes, one click. Picking one
            keeps the menu open: the level shows checked, and
            « Utiliser KaTyoti » goes on to « Aide et retours » in this menu. */}
        <Menu.Label>{t('profile.level.title')}</Menu.Label>
        {DISPLAY_LEVELS.map((l) => (
          <Menu.Item
            key={l}
            closeMenuOnClick={false}
            leftSection={level === l ? <IconCheck size={14} /> : <Box w={14} />}
            onClick={() => setLevel(l)}
            title={t(`profile.level.hint.${l}`)}
            data-guide="profile-menu.level"
            data-guide-name={l}
            aria-checked={level === l}
          >
            {t(`profile.level.${l}`)}
          </Menu.Item>
        ))}

        <Menu.Divider />

        {/* Preferences */}
        <Menu.Label>{t('profile.preferences')}</Menu.Label>
        <Menu.Item
          leftSection={<IconPalette size={14} />}
          rightSection={<IconChevronRight size={12} color="var(--mantine-color-dimmed)" />}
          onClick={() => openSettingsModal('appearance')}
          data-guide="profile-menu.appearance"
        >
          {t('profile.appearance')}
        </Menu.Item>
        {soonItem('notifications', <IconBell size={14} />)}
        {soonItem('theme', <IconMoon size={14} />)}

        <Menu.Divider />

        <Menu.Item leftSection={<IconHelp size={14} />} onClick={() => launchGuide()} data-guide="profile.help">{t('profile.help')}</Menu.Item>
        <Menu.Item leftSection={<IconBook size={14} />} onClick={() => launchGuide([CONCEPTS_TOPIC])} data-guide="profile.concepts">{t('profile.concepts')}</Menu.Item>
        <Menu.Item leftSection={<IconMessageReport size={14} />} onClick={() => void openFeedback()} data-guide="profile-menu.feedback">
          {guideT('feedback.button')}
        </Menu.Item>
        {/* « Se déconnecter » does nothing yet: not shown until it does (§3.2). */}
      </Menu.Dropdown>
    </Menu>
  )
}

// ── Add-universe menu (ribbon `+` button) ─────────────────────────────────
//
// Two entry points:
//   • Create New… → opens the existing UniverseCreationModal (form + folder
//     picker). Used for fresh universes (no .katyoti/ on the chosen folder).
//   • Add Existing… → folder picker + UniverseAddExisting. Used to register
//     a workspace that was cloned-and-go (e.g. from git) — already has its
//     own Univers.yaml. No filesystem mutation; pure registry add.
//
// After Add Existing succeeds, the universe list is refreshed so the new
// icon appears in the ribbon immediately.

function AddUniverseMenu({ onCreate }: { onCreate: () => void }) {
  const setUniverses = useAppStore((s) => s.setUniverses)
  const setMainPaneRoute = useAppStore((s) => s.setMainPaneRoute)
  const t = useT('shell')

  const onAddExisting = async () => {
    type GoBindings = {
      go?: {
        main?: {
          App?: {
            PickExistingUniverseFolder?: () => Promise<{ path?: string; error?: string }>
          }
        }
      }
    }
    const w = window as unknown as GoBindings
    // The folder PICKER stays a Wails binding — a native OS dialog is exactly
    // what KTSP cannot express. Registering the picked folder is a service call.
    const pick = w.go?.main?.App?.PickExistingUniverseFolder
    if (!pick) return // Wails not wired (dev/mock)

    try {
      const picked = await pick()
      if (picked?.error) {
        console.error('[ribbon] PickExistingUniverseFolder:', picked.error)
        return
      }
      if (!picked?.path) return // user cancelled

      await orchestratorClient().addExistingUniverse({ path: picked.path })

      // Refresh the universe list so the new icon shows up.
      setUniverses(await listUniversesWithStatus())
    } catch (e) {
      console.error('[ribbon] add-existing flow failed:', e)
    }
  }

  return (
    <Menu position="right-start" shadow="md" width={220} offset={6}>
      <Menu.Target>
        <Tooltip label={t('ribbon.addUniverse')} position="right" withArrow>
          <ActionIcon
            variant="subtle"
            color="gray"
            size="md"
            aria-label={t('ribbon.addUniverseAria')}
            data-element="rail:add-universe"
            data-guide="ribbon.add-universe"
          >
            <IconPlus size={16} />
          </ActionIcon>
        </Tooltip>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>{t('universe.menuLabel')}</Menu.Label>
        <Menu.Item
          leftSection={<IconSparkles size={14} />}
          onClick={onCreate}
          data-guide="add-universe.create"
        >
          {t('universe.createNew')}
        </Menu.Item>
        <Menu.Item
          leftSection={<IconFolderPlus size={14} />}
          onClick={() => void onAddExisting()}
        >
          {t('universe.addExisting')}
        </Menu.Item>
        <Menu.Divider />
        <Menu.Item
          leftSection={<IconSettings2 size={14} />}
          onClick={() => setMainPaneRoute({ kind: 'welcome' })}
        >
          {t('universe.manage')}
        </Menu.Item>
      </Menu.Dropdown>
    </Menu>
  )
}

