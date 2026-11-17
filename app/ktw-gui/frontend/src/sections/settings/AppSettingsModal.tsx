// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from 'react'
import {
  Box,
  ColorInput,
  Divider,
  Group,
  Modal,
  NavLink,
  NumberInput,
  Radio,
  ScrollArea,
  Select,
  Stack,
  Switch,
  Text,
  ThemeIcon,
  Tooltip,
} from '@mantine/core'
import {
  IconBell,
  IconFileText,
  IconMessageReport,
  IconPalette,
  IconSettings2,
} from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { useT } from '../../i18n'
import { useMediaStore } from '../../store/mediaSlice'
import { FeedbackSettingsPanel } from '../feedback/FeedbackSettingsPanel'
import type { SettingsSection } from '../../types/settings'
import {
  type FontFamily,
  type MonoFamily,
  type CodeTheme,
  type ThemeMode,
} from '../../types/settings'

// ── Section nav ───────────────────────────────────────────────────────────────

interface NavItem {
  key:      SettingsSection
  labelKey: string
  Icon:     typeof IconSettings2
  disabled: boolean
}

const NAV_ITEMS: NavItem[] = [
  { key: 'general',       labelKey: 'appSettings.nav.general',       Icon: IconSettings2, disabled: false },
  { key: 'appearance',    labelKey: 'appSettings.nav.appearance',    Icon: IconPalette,   disabled: false },
  { key: 'notifications', labelKey: 'appSettings.nav.notifications', Icon: IconBell,      disabled: true  },
  { key: 'feedback',      labelKey: 'appSettings.nav.feedback',      Icon: IconMessageReport, disabled: false },
]

// ── Section panels ─────────────────────────────────────────────────────────────

function GeneralPanel() {
  const t              = useT('settings')
  const settings       = useAppStore((s) => s.appSettings.general)
  const updateSettings = useAppStore((s) => s.updateSettings)
  const patch          = (p: typeof settings) => updateSettings('general', p)
  const cameraBlur     = useMediaStore((s) => s.cameraBlur)
  const setCameraBlur  = useMediaStore((s) => s.setCameraBlur)

  return (
    <Stack gap="lg">
      <div>
        <Text size="xs" fw={700} c="dimmed" tt="uppercase" mb="xs" style={{ letterSpacing: 0.8 }}>
          {t('appSettings.general.camera')}
        </Text>
        <Switch
          label={t('appSettings.general.blurBackground')}
          description={t('appSettings.general.blurBackgroundHint')}
          size="sm"
          checked={cameraBlur}
          onChange={(e) => setCameraBlur(e.currentTarget.checked)}
          labelPosition="left"
          styles={{ body: { justifyContent: 'space-between' } }}
        />
      </div>
      <div>
        <Text size="xs" fw={700} c="dimmed" tt="uppercase" mb="xs" style={{ letterSpacing: 0.8 }}>
          {t('appSettings.general.chatBehavior')}
        </Text>
        <Stack gap="xs">
          <Switch
            label={t('appSettings.general.autoScroll')}
            size="sm"
            checked={settings.autoScroll}
            onChange={(e) => patch({ ...settings, autoScroll: e.currentTarget.checked })}
            labelPosition="left"
            styles={{ body: { justifyContent: 'space-between' } }}
          />
          <Switch
            label={t('appSettings.general.showToolInputs')}
            size="sm"
            checked={settings.showToolInputs}
            onChange={(e) => patch({ ...settings, showToolInputs: e.currentTarget.checked })}
            labelPosition="left"
            styles={{ body: { justifyContent: 'space-between' } }}
          />
          <Switch
            label={t('appSettings.general.compactMessages')}
            size="sm"
            checked={settings.compactMessages}
            onChange={(e) => patch({ ...settings, compactMessages: e.currentTarget.checked })}
            labelPosition="left"
            styles={{ body: { justifyContent: 'space-between' } }}
          />
          <Switch
            label={t('appSettings.general.fullWidth')}
            description={t('appSettings.general.fullWidthHint')}
            size="sm"
            checked={settings.fullWidthMessages}
            onChange={(e) => patch({ ...settings, fullWidthMessages: e.currentTarget.checked })}
            labelPosition="left"
            styles={{ body: { justifyContent: 'space-between' } }}
          />
        </Stack>
      </div>
    </Stack>
  )
}

function AppearancePanel() {
  const t              = useT('settings')
  const settings       = useAppStore((s) => s.appSettings.appearance)
  const updateSettings = useAppStore((s) => s.updateSettings)
  const patch          = (p: Partial<typeof settings>) => updateSettings('appearance', p)

  return (
    <Stack gap="lg">
      {/* Theme mode */}
      <div>
        <Text size="xs" fw={700} c="dimmed" tt="uppercase" mb="xs" style={{ letterSpacing: 0.8 }}>
          {t('appSettings.appearance.theme')}
        </Text>
        <Radio.Group
          value={settings.themeMode}
          onChange={(v) => patch({ themeMode: v as ThemeMode })}
        >
          <Group gap="xl">
            <Radio value="dark"   label={t('appSettings.appearance.dark')}   size="sm" />
            <Radio value="light"  label={t('appSettings.appearance.light')}  size="sm" />
            <Radio value="system" label={t('appSettings.appearance.system')} size="sm" />
          </Group>
        </Radio.Group>
      </div>

      <Divider />

      {/* Accent color */}
      <div>
        <Text size="xs" fw={700} c="dimmed" tt="uppercase" mb="xs" style={{ letterSpacing: 0.8 }}>
          {t('appSettings.appearance.accentColor')}
        </Text>
        <ColorInput
          size="sm"
          value={settings.accentColor}
          onChange={(v) => patch({ accentColor: v })}
          format="hex"
          swatches={['#228be6','#12b886','#fa5252','#fd7e14','#7950f2','#e64980','#74c0fc']}
          w={220}
        />
      </div>

      <Divider />

      {/* Typography */}
      <div>
        <Text size="xs" fw={700} c="dimmed" tt="uppercase" mb="xs" style={{ letterSpacing: 0.8 }}>
          {t('appSettings.appearance.typography')}
        </Text>
        <Stack gap="sm">
          <NumberInput
            label={t('appSettings.appearance.fontSize')}
            size="sm"
            value={settings.fontSize}
            onChange={(v) => patch({ fontSize: Number(v) })}
            min={10}
            max={20}
            step={1}
            w={160}
          />
          <Select
            label={t('appSettings.appearance.uiFont')}
            size="sm"
            value={settings.fontFamily}
            onChange={(v) => patch({ fontFamily: v as FontFamily })}
            data={['Inter','Roboto','Open Sans','Lato','System UI']}
            w={240}
          />
          <Select
            label={t('appSettings.appearance.monoFont')}
            size="sm"
            value={settings.monoFamily}
            onChange={(v) => patch({ monoFamily: v as MonoFamily })}
            data={['JetBrains Mono','Fira Code','Cascadia Code','Source Code Pro','Consolas']}
            w={240}
          />
        </Stack>
      </div>

      <Divider />

      {/* Code block themes */}
      <div>
        <Text size="xs" fw={700} c="dimmed" tt="uppercase" mb="xs" style={{ letterSpacing: 0.8 }}>
          {t('appSettings.appearance.codeTheme')}
        </Text>
        <Stack gap="sm">
          <Select
            label={t('appSettings.appearance.codeThemeDark')}
            size="sm"
            value={settings.codeThemeDark}
            onChange={(v) => patch({ codeThemeDark: v as CodeTheme })}
            data={[
              { value: 'github-dark', label: 'GitHub Dark' }, // i18n-ignore
              { value: 'dracula',     label: 'Dracula'     },
              { value: 'one-dark',    label: 'One Dark'    }, // i18n-ignore
              { value: 'nord',        label: 'Nord'        },
            ]}
            w={240}
          />
          <Select
            label={t('appSettings.appearance.codeThemeLight')}
            size="sm"
            value={settings.codeThemeLight}
            onChange={(v) => patch({ codeThemeLight: v as CodeTheme })}
            data={[
              { value: 'github-light', label: 'GitHub Light' }, // i18n-ignore
              { value: 'one-dark',     label: 'One Dark'     }, // i18n-ignore
              { value: 'nord',         label: 'Nord'         },
            ]}
            w={240}
          />
        </Stack>
      </div>
    </Stack>
  )
}

function NotificationsPanel() {
  const t = useT('settings')
  return (
    <Stack gap="md" align="center" pt="xl">
      <ThemeIcon size={48} radius="xl" variant="light" color="gray">
        <IconBell size={24} />
      </ThemeIcon>
      <Text size="sm" c="dimmed" ta="center">
        {t('appSettings.notifications.unavailable')}
      </Text>
      <Text size="xs" c="dimmed" ta="center">
        {t('appSettings.notifications.future')}
      </Text>
    </Stack>
  )
}

// ── Root modal ─────────────────────────────────────────────────────────────────

export function AppSettingsModal() {
  const t       = useT('settings')
  const open    = useAppStore((s) => s.settingsModalOpen)
  const section = useAppStore((s) => s.settingsModalSection)
  const setSection = (sec: SettingsSection) =>
    useAppStore.getState().openSettingsModal(sec)
  const close   = useAppStore((s) => s.closeSettingsModal)

  // Storage path footer — fetched once when the modal opens. Surfaces where
  // the YAML lives so users can edit it manually or know what to back up.
  const [settingsPath, setSettingsPath] = useState<string>('')
  useEffect(() => {
    if (!open) return
    const win = window as unknown as {
      go?: { main?: { App?: { GetGuiSettingsPath?: () => Promise<string> } } }
    }
    const fn = win.go?.main?.App?.GetGuiSettingsPath
    if (!fn) return
    fn().then(setSettingsPath).catch(() => setSettingsPath(''))
  }, [open])

  return (
    <Modal
      opened={open}
      onClose={close}
      title={<Text fw={600} size="sm">{t('appSettings.title')}</Text>}
      size={680}
      padding={0}
      styles={{
        header: { padding: '12px 16px', borderBottom: '1px solid var(--mantine-color-default-border)' },
        body:   { padding: 0 },
      }}
    >
      <Group align="stretch" gap={0} style={{ minHeight: 420 }}>
        {/* ── Left nav ──────────────────────────────────────── */}
        <Box
          style={{
            width: 180,
            borderRight: '1px solid var(--mantine-color-default-border)',
            padding: '8px 4px',
            flexShrink: 0,
          }}
        >
          {NAV_ITEMS.map(({ key, labelKey, Icon, disabled }) => (
            <NavLink
              key={key}
              label={t(labelKey)}
              active={section === key}
              disabled={disabled}
              leftSection={<Icon size={16} />}
              onClick={() => !disabled && setSection(key)}
              styles={{
                root: { borderRadius: 6, marginBottom: 2 },
              }}
            />
          ))}
        </Box>

        {/* ── Panel content ─────────────────────────────────── */}
        <ScrollArea style={{ flex: 1 }} p="lg">
          <Box p="lg">
            {section === 'general'       && <GeneralPanel />}
            {section === 'appearance'    && <AppearancePanel />}
            {section === 'notifications' && <NotificationsPanel />}
            {section === 'feedback'      && <FeedbackSettingsPanel />}
          </Box>
        </ScrollArea>
      </Group>

      {/* ── Storage path footer ──────────────────────────────────────── */}
      {settingsPath && (
        <Box
          style={{
            padding: '8px 16px',
            borderTop: '1px solid var(--mantine-color-default-border)',
            display: 'flex',
            alignItems: 'center',
            gap: 6,
          }}
        >
          <IconFileText size={12} style={{ opacity: 0.5, flexShrink: 0 }} />
          <Tooltip label={settingsPath} withArrow position="top-start" multiline maw={500}>
            <Text size="xs" c="dimmed" truncate style={{ flex: 1, minWidth: 0 }}>
              {t('appSettings.savedAt', { path: settingsPath })}
            </Text>
          </Tooltip>
        </Box>
      )}
    </Modal>
  )
}
