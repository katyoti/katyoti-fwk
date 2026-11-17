// SPDX-License-Identifier: Apache-2.0
import {
  ActionIcon,
  Divider,
  Group,
  ScrollArea,
  Select,
  Slider,
  Stack,
  Switch,
  Tabs,
  Text,
} from '@mantine/core'
import { IconX } from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'

/** Right settings panel (280px, collapsible via store) */
export function SettingsPanel() {
  const toggleSettings = useAppStore((s) => s.toggleSettings)
  const t = useT('settings')

  return (
    <Stack
      gap={0}
      style={{
        height: '100%',
        background: 'var(--mantine-color-body)',
      }}
    >
      {/* Header */}
      <Group
        justify="space-between"
        align="center"
        px="sm"
        py={10}
        style={{ borderBottom: '1px solid var(--mantine-color-default-border)', flexShrink: 0 }}
      >
        <Text size="sm" fw={600}>{t('panel.title')}</Text>
        <ActionIcon variant="subtle" color="gray" size="sm" onClick={toggleSettings}>
          <IconX size={14} />
        </ActionIcon>
      </Group>

      {/* Tabs + scrollable content */}
      <ScrollArea style={{ flex: 1 }}>
        <Tabs defaultValue="general">
          <Tabs.List px={4}>
            <Tabs.Tab value="general" fz={12}>{t('panel.tabs.general')}</Tabs.Tab>
            <Tabs.Tab value="model"   fz={12}>{t('panel.tabs.model')}</Tabs.Tab>
          </Tabs.List>

          {/* ── General ─────────────────────────────────────── */}
          <Tabs.Panel value="general">
            <Stack gap="md" p="sm">
              <div>
                <Text size="xs" fw={600} c="dimmed" tt="uppercase" mb={6} style={{ letterSpacing: 0.8 }}>
                  {t('panel.appearance')}
                </Text>
                <Select
                  label={t('panel.theme')}
                  size="xs"
                  data={[
                    { value: 'Dark', label: t('panel.themeDark') },
                    { value: 'Light', label: t('panel.themeLight') },
                    { value: 'System', label: t('panel.themeSystem') },
                  ]}
                  defaultValue="Dark"
                />
              </div>

              <Divider />

              <div>
                <Text size="xs" fw={600} c="dimmed" tt="uppercase" mb={6} style={{ letterSpacing: 0.8 }}>
                  {t('panel.behavior')}
                </Text>
                <Stack gap="xs">
                  <Switch label={t('panel.autoScroll')} size="xs" defaultChecked labelPosition="left"
                    styles={{ body: { justifyContent: 'space-between' } }} />
                  <Switch label={t('panel.showToolInputs')} size="xs" defaultChecked labelPosition="left"
                    styles={{ body: { justifyContent: 'space-between' } }} />
                  <Switch label={t('panel.compactMode')} size="xs"              labelPosition="left"
                    styles={{ body: { justifyContent: 'space-between' } }} />
                </Stack>
              </div>
            </Stack>
          </Tabs.Panel>

          {/* ── Model ───────────────────────────────────────── */}
          <Tabs.Panel value="model">
            <Stack gap="md" p="sm">
              <Select
                label={t('panel.model')}
                size="xs"
                data={['claude-sonnet-4-6', 'claude-opus-4-6', 'claude-haiku-4-5']}
                defaultValue="claude-sonnet-4-6"
              />

              <div>
                <Group justify="space-between" mb={4}>
                  <Text size="xs" fw={500}>{t('panel.temperature')}</Text>
                  <Text size="xs" c="dimmed">0.7</Text>
                </Group>
                <Slider size="xs" min={0} max={1} step={0.1} defaultValue={0.7} />
              </div>

              <div>
                <Group justify="space-between" mb={4}>
                  <Text size="xs" fw={500}>{t('panel.maxTokens')}</Text>
                  <Text size="xs" c="dimmed">8192</Text>
                </Group>
                <Slider size="xs" min={1024} max={32768} step={1024} defaultValue={8192} />
              </div>

              <Divider />

              <Switch label={t('panel.streaming')} size="xs" defaultChecked labelPosition="left"
                styles={{ body: { justifyContent: 'space-between' } }} />
            </Stack>
          </Tabs.Panel>
        </Tabs>
      </ScrollArea>
    </Stack>
  )
}
