// SPDX-License-Identifier: Apache-2.0
/**
 * TerminalsBody — content rendered inside the right-pane "Term" tab.
 *
 * Layout: narrow session-list column (left) + active TerminalView (right).
 * The Start menu (top-left) lets the user spawn a log-tail or an embedded
 * PTY-backed shell. Clicking a list entry switches the right-pane view.
 *
 * Companion to MainPaneTabs (Stage 1.5 of gui-mainpane-restructure):
 * gives the user back the Term entry point that lived in the legacy
 * ProjectPanel WORK/CODE/TERM tab strip.
 */

import { useEffect, useMemo, useRef, useState } from 'react'
import {
  ActionIcon,
  Box,
  Center,
  Divider,
  Group,
  Menu,
  NavLink,
  Stack,
  Text,
} from '@mantine/core'
import {
  IconBrandDebian,
  IconBrandWindows,
  IconFileText,
  IconPlus,
  IconTerminal2,
} from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useTabsStore } from '../../store/tabsSlice'
import { useTerminalSessionsStore } from '../../store/terminalSessionsSlice'
import { TerminalView } from './TerminalView'
import { GetInstalledTerminals } from '../../../wailsjs/go/main/App'
import { useT } from '../../i18n'

interface TerminalInfo {
  id:    string
  label: string
  icon:  string
  path:  string
}

function terminalIcon(icon: string) {
  switch (icon) {
    case 'windows': return <IconBrandWindows size={14} />
    case 'linux':   return <IconBrandDebian size={14} />
    default:        return <IconTerminal2 size={14} />
  }
}

export function TerminalsBody() {
  const t = useT('screens')
  const universes             = useAppStore((s) => s.universes)
  const displayedUniverseName = useAppStore((s) => s.displayedUniverseName)
  const displayed             = universes.find((u) => u.name === displayedUniverseName)
  const cwd                   = displayed?.homePath ?? ''

  const sessions   = useTerminalSessionsStore((s) => s.sessions)
  const activeId   = useTerminalSessionsStore((s) => s.activeId)
  const setActive  = useTerminalSessionsStore((s) => s.setActive)
  const startLog   = useTerminalSessionsStore((s) => s.startLog)
  const startShell = useTerminalSessionsStore((s) => s.startShell)

  const closeTab = useTabsStore((s) => s.closeTab)

  const [installed, setInstalled] = useState<TerminalInfo[]>([])
  const [installedError, setInstalledError] = useState<string | null>(null)

  // Auto-close the Term tab after the user closes the last running
  // terminal. We track the previous count so we only fire on the
  // >0 → 0 transition (NOT on initial mount when count is 0).
  const prevCountRef = useRef(sessions.size)
  useEffect(() => {
    const prev = prevCountRef.current
    const now = sessions.size
    if (prev > 0 && now === 0 && displayedUniverseName) {
      closeTab(displayedUniverseName, 'term')
    }
    prevCountRef.current = now
  }, [sessions.size, displayedUniverseName, closeTab])

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

  const onStartLog = async (source: 'ktw-gui-log') => {
    const id = await startLog(source)
    if (id) setActive(id)
  }

  const onStartShell = async (shellId: string, label: string) => {
    const id = await startShell(shellId, label, cwd)
    if (id) setActive(id)
  }

  return (
    <Box style={{ display: 'flex', height: '100%', minHeight: 0 }}>
      {/* Sessions column */}
      <Stack
        gap={0}
        style={{
          width: 240,
          flexShrink: 0,
          borderRight: '1px solid var(--mantine-color-default-border)',
          overflow: 'hidden',
        }}
      >
        <Group justify="space-between" px="sm" py={6} style={{ flexShrink: 0 }}>
          <Text size="xs" c="dimmed">{t('terminals.running', { count: sessionList.length })}</Text>
          <Menu shadow="md" width={220} position="bottom-end">
            <Menu.Target>
              <ActionIcon size="sm" variant="default" aria-label={t('terminals.start')}>
                <IconPlus size={14} />
              </ActionIcon>
            </Menu.Target>
            <Menu.Dropdown>
              <Menu.Label>{t('terminals.logs')}</Menu.Label>
              <Menu.Item
                leftSection={<IconFileText size={14} />}
                onClick={() => void onStartLog('ktw-gui-log')}
              >
                {t('terminals.guiLogs')}
              </Menu.Item>
              {installed.length > 0 && (
                <>
                  <Menu.Divider />
                  <Menu.Label>{t('terminals.shells')}</Menu.Label>
                  {installed.map((t) => (
                    <Menu.Item
                      key={t.id}
                      leftSection={terminalIcon(t.icon)}
                      onClick={() => void onStartShell(t.id, t.label)}
                    >
                      {t.label}
                    </Menu.Item>
                  ))}
                </>
              )}
              {installedError && (
                <Menu.Item disabled c="red">
                  {t('terminals.error', { error: installedError })}
                </Menu.Item>
              )}
            </Menu.Dropdown>
          </Menu>
        </Group>
        <Divider />
        <Box style={{ flex: 1, minHeight: 0, overflow: 'auto' }}>
          {sessionList.length === 0 && (
            <Text size="xs" c="dimmed" px="sm" py="xs">
              {t('terminals.noneBefore')} <IconPlus size={10} style={{ verticalAlign: 'middle' }} /> {t('terminals.noneAfter')}
            </Text>
          )}
          {sessionList.map((s) => (
            <NavLink
              key={s.id}
              active={s.id === activeId}
              label={<Text size="xs">{s.label}</Text>}
              description={`${s.kind === 'log' ? t('terminals.kindLog') : t('terminals.kindShell')} · ${s.status}`}
              leftSection={s.kind === 'log' ? <IconFileText size={14} /> : <IconTerminal2 size={14} />}
              onClick={() => setActive(s.id)}
            />
          ))}
        </Box>
      </Stack>

      {/* Active terminal view */}
      <Box style={{ flex: 1, minWidth: 0, minHeight: 0 }}>
        {activeId ? (
          <TerminalView id={activeId} />
        ) : (
          <Center style={{ height: '100%' }}>
            <Stack align="center" gap="xs">
              <IconTerminal2 size={36} stroke={1.2} color="var(--mantine-color-dimmed)" />
              <Text size="sm" c="dimmed">{t('terminals.noneSelected')}</Text>
              <Text size="xs" c="dimmed">{t('terminals.clickPlus')}</Text>
            </Stack>
          </Center>
        )}
      </Box>
    </Box>
  )
}
