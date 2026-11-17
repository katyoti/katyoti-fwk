// SPDX-License-Identifier: Apache-2.0
import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Autocomplete,
  Avatar,
  Badge,
  Button,
  ColorSwatch,
  Divider,
  Group,
  Loader,
  Paper,
  Stack,
  Tabs,
  Text,
  Textarea,
  TextInput,
  Title,
  UnstyledButton,
} from '@mantine/core'
import {
  IconAlertCircle,
  IconBox,
  IconDeviceFloppy,
  IconBuildingStore,
  IconShieldLock,
} from '@tabler/icons-react'
import { MainPanelLayout } from '../components/layout/MainPanelLayout'
import { useAppStore } from '../store'
import { orchestratorClient } from '../ktsp'
import { listUniversesWithStatus } from '../ktsp/universes'
import { UniverseProjectsPanel } from './UniverseProjectsPanel'
import type { Universe, UniverseInfo } from '../types/universe'
import { useMarketplaceLauncher } from '../hooks/useMarketplaceLauncher'
import { useT } from '../i18n'
import { installedSolutionChoices, offersSolutionChoice } from './universeSolution'

const COLOR_OPTIONS = [
  'violet', 'blue', 'teal', 'lime', 'yellow',
  'orange', 'red', 'pink', 'grape', 'indigo',
  'cyan', 'green', 'gray',
] as const

type Color = typeof COLOR_OPTIONS[number]

/**
 * Edit-universe view shown in the main pane (router state: edit/<name>).
 *
 * Editable: display_name, description, icon, color.
 * Read-only: name (slug — renaming would orphan the data dir), home_path
 * (would require a folder move), the solution once set (moving it is the
 * solution's update), and the sidecar membership — shown here,
 * changed in ONE place: Marketplace › Sidecars (install, wire, unwire), with
 * the universe open.
 *
 * A universe with no solution is offered the action giving it an installed
 * one (orchestrator set-universe-solution).
 *
 * Submits via App.UniverseUpdate; on success refreshes the universe list
 * and stays in the edit view (so the user can confirm the change took
 * effect via the live preview at top).
 */
interface UniverseEditPaneProps {
  name: string
}

export function UniverseEditPane({ name }: UniverseEditPaneProps) {
  const t                = useT('universes')
  const setMainPaneRoute = useAppStore((s) => s.setMainPaneRoute)
  const displayedName    = useAppStore((s) => s.displayedUniverseName)
  const universes        = useAppStore((s) => s.universes)
  const setUniverses     = useAppStore((s) => s.setUniverses)
  const summary          = universes.find((u) => u.name === name)

  const [info, setInfo]       = useState<UniverseInfo | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  // Editable form state.
  const [display, setDisplay]         = useState('')
  const [description, setDescription] = useState('')
  const [icon, setIcon]               = useState('')
  const [color, setColor]             = useState<Color>('violet')
  const { launchMarketplace } = useMarketplaceLauncher()

  // Submit state.
  const [saving, setSaving]   = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState<number | null>(null)

  // The solution action (shown only while the universe has none).
  const [solution, setSolution]           = useState('')
  const [givingSolution, setGiving]       = useState(false)
  const [solutionError, setSolutionError] = useState<string | null>(null)
  const solutionChoices = useMemo(() => installedSolutionChoices(universes), [universes])

  // Load universe info on mount / name change.
  useEffect(() => {
    setLoading(true)
    setLoadError(null)
    setInfo(null)
    setSavedAt(null)
    setSolution('')
    setSolutionError(null)
    const orch = orchestratorClient()

    orch.getUniverse({ name })
      .then((info) => {
        setInfo(info)
        setDisplay(info.summary.displayName)
        setDescription(info.summary.description ?? '')
        setIcon(info.summary.icon)
        setColor((info.summary.color as Color) || 'violet')
      })
      .catch((err) => setLoadError(String(err)))
      .finally(() => setLoading(false))
  }, [name])

  const goBack = () => {
    setMainPaneRoute(displayedName ? { kind: 'universe' } : { kind: 'welcome' })
  }

  const dirty = useMemo(() => {
    if (!info) return false
    if (display !== info.summary.displayName) return true
    if (description !== (info.summary.description ?? '')) return true
    if (icon !== info.summary.icon) return true
    if (color !== info.summary.color) return true
    return false
  }, [info, display, description, icon, color])

  const formValid = display.trim().length > 0

  const onSave = async () => {
    if (!info || !formValid || saving) return
    setSaving(true)
    setSaveError(null)
    const orch = orchestratorClient()
    try {
      await orch.updateUniverse({
        name,
        displayName: display.trim(),
        description: description.trim(),
        icon:        icon.trim() || (display.trim()[0]?.toUpperCase() ?? '?'),
        color,
        // The membership as it is: every member is kept verbatim (an absent
        // list would replace it with nothing).
        sidecars: info.sidecars.map((s) => ({ name: s.name, binaryPath: '' })),
      })
      // Refresh sidebar list.
      setUniverses(await listUniversesWithStatus())
      // Re-fetch the full universe info so the dirty-state baseline mirrors
      // disk (summary AND sidecars). Without this, info.sidecars stays at the
      // pre-save value and the Save button keeps reading "dirty".
      setInfo(await orch.getUniverse({ name }))
      setSavedAt(Date.now())
    } catch (err) {
      setSaveError(String(err))
    } finally {
      setSaving(false)
    }
  }

  const onGiveSolution = async () => {
    const want = solution.trim()
    if (!info || !want || givingSolution) return
    setGiving(true)
    setSolutionError(null)
    try {
      setInfo(await orchestratorClient().setUniverseSolution({ name, solution: want }))
      setSolution('')
      setUniverses(await listUniversesWithStatus())
    } catch (err) {
      setSolutionError(String(err))
    } finally {
      setGiving(false)
    }
  }

  const titleZone = (
    <Stack gap="md">
      <Group gap="sm" align="center">
        <Title order={3}>{t('edit.title')}</Title>
        {summary?.protected && (
          <Badge size="sm" color="gray" leftSection={<IconShieldLock size={11} />}>
            {t('common.protected')}
          </Badge>
        )}
        <div style={{ flex: 1 }} />
        {savedAt && !dirty && (
          <Text size="xs" c="teal">{t('edit.saved')}</Text>
        )}
      </Group>
      {info && (
        <Paper
          p="md"
          radius="md"
          style={{
            background: `var(--mantine-color-${color}-light)`,
            border: `1px solid var(--mantine-color-${color}-3)`,
          }}
        >
          <Group gap="md" wrap="nowrap" align="center">
            <Avatar size={56} radius="md" color={color} variant="filled" style={{ fontSize: 18, fontWeight: 700 }}>
              {icon || '?'}
            </Avatar>
            <div style={{ flex: 1, minWidth: 0 }}>
              <Title order={4}>{display.trim() || name}</Title>
              <Text size="xs" c="dimmed">{name} · {info.summary.homePath}</Text>
              {description && (
                <Text size="sm" c="dimmed" mt={4} lineClamp={2}>{description}</Text>
              )}
            </div>
          </Group>
        </Paper>
      )}
    </Stack>
  )

  const footerZone = info ? (
    <Group justify="flex-end" gap="sm">
      <Button variant="default" onClick={goBack} disabled={saving}>
        {dirty ? t('edit.discard') : t('edit.close')}
      </Button>
      <Button
        color={color}
        leftSection={<IconDeviceFloppy size={16} />}
        onClick={onSave}
        loading={saving}
        disabled={!dirty || !formValid}
      >
        {t('edit.save')}
      </Button>
    </Group>
  ) : null

  return (
    <MainPanelLayout title={titleZone} footer={footerZone}>
      {loading && (
        <Group gap="xs">
          <Loader size="sm" />
          <Text size="sm" c="dimmed">{t('edit.loading', { name })}</Text>
        </Group>
      )}

      {loadError && (
        <Alert color="red" icon={<IconAlertCircle size={16} />} title={t('edit.errors.loadFailed')}>
          <Text size="xs">{loadError}</Text>
        </Alert>
      )}

      {info && (
        <>
          <Tabs
            defaultValue="global"
            keepMounted
            style={{
              flex:          1,
              minHeight:     0,
              display:       'flex',
              flexDirection: 'column',
            }}
          >
            <Tabs.List>
              <Tabs.Tab value="global">{t('edit.tabs.global')}</Tabs.Tab>
              <Tabs.Tab value="sidecars">{t('form.sidecars')}</Tabs.Tab>
              <Tabs.Tab value="projects" data-guide="universe-edit.projects">{t('edit.tabs.projects')}</Tabs.Tab>
            </Tabs.List>

            <Tabs.Panel
              value="global"
              pt="md"
              style={{
                flex:      1,
                minHeight: 0,
                overflowY: 'auto',
              }}
            >
                  <Stack gap="md">
                    <Divider label={t('form.identity')} labelPosition="left" />

                    <TextInput
                      label={t('form.displayName')}
                      required
                      value={display}
                      onChange={(e) => setDisplay(e.currentTarget.value)}
                      error={display.trim().length === 0 ? t('create.errors.required') : null}
                    />

                    <Textarea
                      label={t('form.description')}
                      description={t('form.descriptionHint')}
                      minRows={2}
                      maxRows={4}
                      autosize
                      value={description}
                      onChange={(e) => setDescription(e.currentTarget.value)}
                    />

                    <Group gap="md" align="flex-start">
                      <TextInput
                        label={t('edit.slug')}
                        description={t('edit.slugHint')}
                        value={info.summary.name}
                        disabled
                        style={{ flex: 1 }}
                      />
                      <TextInput
                        label={t('edit.home')}
                        description={t('edit.homeHint')}
                        value={info.summary.homePath}
                        disabled
                        style={{ flex: 2 }}
                      />
                    </Group>

                    <Divider label={t('edit.solution.section')} labelPosition="left" />

                    {offersSolutionChoice(info.summary) ? (
                      <Stack gap="xs" data-guide="universe-edit.solution">
                        <Text size="sm" c="dimmed">{t('edit.solution.none')}</Text>
                        <Group align="flex-end" gap="sm">
                          <Autocomplete
                            label={t('edit.solution.pick')}
                            description={t('edit.solution.pickHint')}
                            data={solutionChoices}
                            value={solution}
                            onChange={setSolution}
                            style={{ flex: 1 }}
                            data-guide="universe-edit.solution-input"
                          />
                          <Button
                            leftSection={<IconBox size={16} />}
                            onClick={onGiveSolution}
                            loading={givingSolution}
                            disabled={!solution.trim()}
                            data-guide="universe-edit.give-solution"
                          >
                            {t('edit.solution.give')}
                          </Button>
                        </Group>
                        {solutionError && (
                          <Alert color="red" icon={<IconAlertCircle size={16} />} title={t('edit.solution.failed')}>
                            <Text size="xs">{solutionError}</Text>
                          </Alert>
                        )}
                      </Stack>
                    ) : (
                      <TextInput
                        label={t('edit.solution.label')}
                        description={t('edit.solution.hint')}
                        value={info.summary.solution ?? ''}
                        disabled
                        data-guide="universe-edit.solution"
                      />
                    )}

                    <Divider label={t('form.appearance')} labelPosition="left" />

                    <Group align="flex-end" gap="md">
                      <TextInput
                        label={t('form.icon')}
                        description={t('form.iconHint')}
                        value={icon}
                        onChange={(e) => setIcon(e.currentTarget.value.slice(0, 2))}
                        w={90}
                      />
                      <div style={{ flex: 1 }}>
                        <Text size="sm" fw={500} mb={4}>{t('form.color')}</Text>
                        <Group gap="xs">
                          {COLOR_OPTIONS.map((c) => (
                            <UnstyledButton
                              key={c}
                              onClick={() => setColor(c)}
                              title={c}
                              style={{
                                borderRadius: 6,
                                padding: 2,
                                outline: c === color ? `2px solid var(--mantine-color-${c}-6)` : '2px solid transparent',
                                outlineOffset: 1,
                                transition: 'outline-color 120ms ease',
                              }}
                            >
                              <ColorSwatch color={`var(--mantine-color-${c}-6)`} size={22} />
                            </UnstyledButton>
                          ))}
                        </Group>
                      </div>
                    </Group>
                  </Stack>
                </Tabs.Panel>

                <Tabs.Panel
                  value="sidecars"
                  pt="md"
                  style={{
                    flex:          1,
                    minHeight:     0,
                    display:       'flex',
                    flexDirection: 'column',
                  }}
                >
                  <Stack gap="md" style={{ flex: 1, minHeight: 0 }}>
                    <Group justify="space-between" align="center" gap="sm">
                      <Text size="sm" c="dimmed">{t('edit.sidecarsHint')}</Text>
                      <Button
                        variant="light"
                        size="xs"
                        leftSection={<IconBuildingStore size={14} />}
                        disabled={name !== displayedName}
                        title={name !== displayedName ? t('edit.manageNeedsOpen') : undefined}
                        onClick={() => launchMarketplace('sidecars')}
                      >
                        {t('edit.manageInMarketplace')}
                      </Button>
                    </Group>
                    <Stack gap={4}>
                      {(info?.sidecars ?? []).length === 0 && <Text size="xs" c="dimmed">{t('edit.noMembers')}</Text>}
                      {(info?.sidecars ?? []).map((sc) => (
                        <Group key={sc.name} gap={6} wrap="nowrap">
                          <Text size="sm" fw={500}>{sc.name}</Text>
                          <Text size="xs" c="dimmed" truncate>{sc.binary}</Text>
                        </Group>
                      ))}
                    </Stack>
                  </Stack>
                </Tabs.Panel>

                {/* Projects attach to a UNIVERSE, so this belongs beside its
                    sidecars rather than in a global tab. */}
                <Tabs.Panel
                  value="projects"
                  pt="md"
                  style={{ flex: 1, minHeight: 0, overflowY: 'auto' }}
                >
                  <UniverseProjectsPanel universe={name} />
                </Tabs.Panel>
              </Tabs>

          {saveError && (
            <Alert
              color="red"
              icon={<IconAlertCircle size={16} />}
              title={t('edit.errors.saveFailed')}
              mt="md"
            >
              <Text size="xs">{saveError}</Text>
            </Alert>
          )}
        </>
      )}
    </MainPanelLayout>
  )
}
