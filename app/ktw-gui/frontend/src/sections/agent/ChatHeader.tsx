// SPDX-License-Identifier: Apache-2.0
import { ActionIcon, Badge, CopyButton, Group, Menu, Text, Tooltip } from '@mantine/core'
import { IconAdjustments, IconCheck, IconCopy, IconDots, IconSettings, IconTarget } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'

/** Top bar of the chat area — task name, scope chip, session id, settings */
export function ChatHeader({ sessionId, scopedNodeName }: { sessionId?: string; scopedNodeName?: string }) {
  const activeTask        = useAppStore((s) => s.activeTask)
  const settingsOpen      = useAppStore((s) => s.settingsOpen)
  const toggleSettings    = useAppStore((s) => s.toggleSettings)
  const openSettingsModal = useAppStore((s) => s.openSettingsModal)
  const t = useT('chat')

  return (
    <Group
      align="center"
      px="md"
      style={{
        height: 44,
        flexShrink: 0,
        borderBottom: '1px solid var(--mantine-color-default-border)',
      }}
    >
      {/* Title: task name + session id fill the available space */}
      <Group gap="sm" align="center" style={{ flex: 1, minWidth: 0 }}>
        <Text fw={600} size="sm" c={activeTask ? undefined : 'dimmed'} truncate="end">
          {activeTask ?? t('header.noActiveSession')}
        </Text>
        {scopedNodeName && (
          <Tooltip label={t('header.scopedTooltip')} withArrow>
            <Badge
              size="sm"
              variant="light"
              color="violet"
              leftSection={<IconTarget size={11} />}
              style={{ flexShrink: 0, textTransform: 'none' }}
            >
              {t('header.scopedTo', { name: scopedNodeName })}
            </Badge>
          </Tooltip>
        )}
        {sessionId && (
          <CopyButton value={sessionId}>
            {({ copied, copy }) => (
              <Tooltip label={copied ? t('header.copied') : t('header.copySessionId')} withArrow>
                <Group
                  gap={4}
                  align="center"
                  onClick={copy}
                  style={{ cursor: 'pointer' }}
                >
                  <Text size="xs" c="dimmed" ff="monospace">
                    {sessionId}
                  </Text>
                  {copied
                    ? <IconCheck size={12} color="var(--mantine-color-green-6)" />
                    : <IconCopy size={12} color="var(--mantine-color-dimmed)" />}
                </Group>
              </Tooltip>
            )}
          </CopyButton>
        )}
      </Group>

      {/* Right: settings + more. (Stop moved onto the chat tab-strip line,
          next to the debug button — see ChatPanel.) */}
      <Group gap={4}>
        <ActionIcon
          variant={settingsOpen ? 'light' : 'subtle'}
          color={settingsOpen ? 'blue' : 'gray'}
          size="sm"
          onClick={toggleSettings}
          title={t('header.sessionSettings')}
        >
          <IconSettings size={14} />
        </ActionIcon>

        {/* More menu */}
        <Menu position="bottom-end" shadow="md" width={180}>
          <Menu.Target>
            <ActionIcon variant="subtle" color="gray" size="sm">
              <IconDots size={14} />
            </ActionIcon>
          </Menu.Target>
          <Menu.Dropdown>
            <Menu.Item
              leftSection={<IconAdjustments size={14} />}
              onClick={() => openSettingsModal('appearance')}
            >
              {t('header.appSettings')}
            </Menu.Item>
          </Menu.Dropdown>
        </Menu>
      </Group>
    </Group>
  )
}
