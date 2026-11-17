// SPDX-License-Identifier: Apache-2.0
import { useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Avatar,
  Badge,
  Box,
  Button,
  Checkbox,
  ColorSwatch,
  Divider,
  Group,
  Modal,
  ScrollArea,
  SegmentedControl,
  Select,
  Stack,
  Switch,
  Text,
  Textarea,
  TextInput,
  Title,
  UnstyledButton,
} from '@mantine/core'
import {
  IconAlertCircle,
  IconFolderOpen,
  IconSearch,
  IconWorld,
} from '@tabler/icons-react'
import { useAppStore } from '../../store'
import { orchestratorClient } from '../../ktsp'
import { SolutionInstallForm } from './SolutionInstallForm'
import { listUniversesWithStatus } from '../../ktsp/universes'
import type { SidecarBinary, Universe } from '../../types/universe'
import { useT } from '../../i18n'
import { loadRecipes } from '../../guide/recipes'
import { chosenFirstGuide, firstGuideChoices, type KnownGuide } from './firstGuideChoices'

// Mantine color palette used for universe avatars. Mirrors the orchestrator
// ADR's accepted color set; the swatch grid below uses the -6 variant.
const COLOR_OPTIONS = [
  'violet', 'blue', 'teal', 'lime', 'yellow',
  'orange', 'red', 'pink', 'grape', 'indigo',
  'cyan', 'green',
] as const

type Color = typeof COLOR_OPTIONS[number]

const SLUG_RE = /^[a-z0-9-]{1,64}$/

/**
 * UniverseCreationModal — the `+` button wizard (Create New flow).
 *
 * Post-cutover (path-based model, 2026-05-15 onwards):
 *  - Identity lives in the workspace folder's `.katyoti/config/Univers.yaml`;
 *    the universe slug is derived from the display name (never user-editable
 *    in this form — it shows up read-only in the preview so the user knows
 *    the internal id but can't get the model into an inconsistent state).
 *  - The workspace folder is a PRIMARY field (was hidden in Advanced before).
 *    Default: `~/.config/katyoti/universes/<slug>/`. Browse to override.
 *  - Sidecar picker has a search filter + sticky toolbar (count + bulk-toggle)
 *    so the list stays usable as the inventory grows.
 *
 * On success: closes modal, refreshes universe list, activates the new
 * universe (so the user lands directly in their new context).
 */
export function UniverseCreationModal() {
  const t                   = useT('universes')
  const open                = useAppStore((s) => s.creationModalOpen)
  const closeCreationModal  = useAppStore((s) => s.closeCreationModal)
  const setUniverses        = useAppStore((s) => s.setUniverses)
  const universes           = useAppStore((s) => s.universes)
  const activateUniverse    = useAppStore((s) => s.activateUniverse)

  // Form state. Note: there is no user-editable slug — it's always derived
  // from the display name. Backend still receives it for path composition.
  const [display,     setDisplay]     = useState('')
  const [description, setDescription] = useState('')
  const [icon,        setIcon]        = useState('')
  const [color,       setColor]       = useState<Color>('violet')
  const [homePath,    setHomePath]    = useState('')
  const [homePathTouched, setHomePathTouched] = useState(false)
  const [sidecars,    setSidecars]    = useState<string[]>([])
  // The guide the universe starts with: one of its sidecars'.
  const [firstGuide,  setFirstGuide]  = useState<string | null>(null)
  const [knownGuides, setKnownGuides] = useState<KnownGuide[]>([])
  // Its project: a universe holds no object of its own — they live in the
  // project it attaches. On by default; an empty folder = ~/Katyoti/[<installation>/]<name>.
  const [withProject, setWithProject] = useState(true)
  const [projectDir,  setProjectDir]  = useState('')

  // Inventory.
  const [inventory,    setInventory]    = useState<SidecarBinary[]>([])
  const [invLoading,   setInvLoading]   = useState(false)

  // Submit state.
  // Two ways to reach a universe, one entry point. Blank builds it field by
  // field; a solution reproduces a working one and only asks for its keys.
  const [mode, setMode] = useState<'blank' | 'solution'>('blank')

  // The guides the GUI can read now, from every universe it reaches: the new
  // universe does not run yet, so its sidecars' guides are read where they run.
  useEffect(() => {
    if (!open) return
    let live = true
    void loadRecipes(universes.map((u) => u.name)).then(({ entries }) => {
      if (live) setKnownGuides(entries.map((e) => ({ uri: e.uri, provider: e.provider, name: e.recipe.name ?? e.id })))
    })
    return () => { live = false }
  }, [open, universes])
  const guideChoices = useMemo(() => firstGuideChoices(knownGuides, sidecars), [knownGuides, sidecars])
  const [submitting, setSubmitting] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  // Reset on open + scan inventory each time.
  useEffect(() => {
    if (!open) return
    setDisplay('')
    setDescription('')
    setIcon('')
    setColor('violet')
    setHomePath('')
    setHomePathTouched(false)
    setSidecars([])
    setWithProject(true)
    setProjectDir('')
    setSubmitError(null)

    setInvLoading(true)
    orchestratorClient()
      .scanInventory({})
      .then((res) => setInventory(res.sidecars ?? []))
      .catch(() => setInventory([]))
      .finally(() => setInvLoading(false))
  }, [open])

  // Auto-derive slug from display name. Never user-editable: the slug is
  // an implementation detail of the path-based universe model, surfaced
  // read-only in the preview so users see what folder will be created.
  const derivedSlug = useMemo(() => slugify(display), [display])
  const effectiveName = derivedSlug

  // Auto-derive home path from slug (until user picks one manually).
  const derivedHomePath = useMemo(() => {
    if (!effectiveName) return ''
    return defaultHomePath(effectiveName)
  }, [effectiveName])
  const effectiveHomePath = homePathTouched ? homePath : derivedHomePath

  // Auto-derive icon from display name (until user edits it manually).
  const derivedIcon = useMemo(() => firstChar(display) || firstChar(effectiveName), [display, effectiveName])
  const effectiveIcon = icon || derivedIcon

  // Validation — display-name driven. Slug validity falls out of the
  // slugify rules; we only need to surface conflicts (reserved + dup).
  const displayError = !display.trim()
    ? t('create.errors.required')
    : !effectiveName
      ? t('create.errors.noAlnum')
      : effectiveName === 'admin'
        ? t('create.errors.reserved', { name: 'admin' })
        : universes.some((u) => u.name === effectiveName)
          ? t('create.errors.exists', { id: effectiveName })
          : null

  const formValid = !displayError

  const onSubmit = async () => {
    if (!formValid || submitting) return
    setSubmitting(true)
    setSubmitError(null)

    const win = window as unknown as {
      go?: { main?: { App?: {
        UniverseCreate?: (input: unknown) => Promise<{
          universe?: Universe
          error?: string
        }>
        UniverseList?: () => Promise<{ universes?: Universe[]; error?: string }>
      } } }
    }
    try {
      await orchestratorClient().createUniverse({
        name:        effectiveName,
        displayName: display.trim(),
        description: description.trim(),
        icon:        effectiveIcon,
        color,
        // Only forward an explicit user choice. The `~/...` derived hint is
        // for display only — sending it would make Go's filepath.Clean
        // keep the literal `~` and create a bogus `<cwd>/~/.config/...`
        // folder. Empty string lets the Go side compute the real default
        // under `<GlobalHomeDir>/universes/<name>/`.
        homePath:    homePathTouched ? homePath : '',
        withProject,
        projectDir:  withProject ? projectDir.trim() : '',
        sidecars:    sidecars.map((s) => {
          const inv = inventory.find((i) => i.name === s)
          return { name: s, binaryPath: inv?.binaryPath ?? '' }
        }),
        firstGuide:  chosenFirstGuide(firstGuide, guideChoices),
      })
      // ACTIVATE, don't merely display. A universe's CWM workspace root is
      // created by BOOTSTRAP, which runs when its core starts — and the core
      // only starts on activation. `displayUniverse` paints the header and
      // leaves no core running, so the brand-new universe opened onto an empty
      // tree with no workspace root and no inbox, and nothing said why.
      // `useUniverses` already learned this the hard way and says so in the
      // same words; the creation path just never got the same treatment.
      setUniverses(await listUniversesWithStatus())
      await activateUniverse(effectiveName)
      closeCreationModal()
    } catch (err) {
      setSubmitError(String(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Modal
      opened={open}
      onClose={() => !submitting && closeCreationModal()}
      title={
        <Group gap="xs">
          <IconWorld size={18} />
          <Text fw={600} size="sm">{t('create.title')}</Text>
        </Group>
      }
      size={620}
      padding={0}
      styles={{
        header: { padding: '12px 16px', borderBottom: '1px solid var(--mantine-color-default-border)' },
        body:   { padding: 0 },
      }}
    >
      <ScrollArea.Autosize mah="70vh">
        <Box p="lg">
          <SegmentedControl
            fullWidth
            size="xs"
            mb="md"
            value={mode}
            onChange={(v: string) => setMode(v as 'blank' | 'solution')}
            data={[
              { label: t('create.modeBlank'), value: 'blank' },
              { label: t('create.modeSolution'), value: 'solution' },
            ]}
          />

          {mode === 'solution' ? (
            <SolutionInstallForm
              onCreated={(name) => {
                void (async () => {
                  setUniverses(await listUniversesWithStatus())
                  // Same reason as the manual path above: activation is what
                  // starts the core, and the core is what bootstraps the
                  // workspace root.
                  await activateUniverse(name)
                  closeCreationModal()
                })()
              }}
              onCancel={() => closeCreationModal()}
            />
          ) : (
          <Stack gap="md">
            {/* Live preview */}
            <PreviewCard
              icon={effectiveIcon}
              color={color}
              displayName={display.trim() || effectiveName || t('create.newUniverse')}
              slug={effectiveName || '—'}
              description={description}
            />

            <Divider label={t('form.identity')} labelPosition="left" />

            <TextInput
              label={t('form.displayName')}
              description={t('create.displayNameHint')}
              placeholder={t('create.displayNamePlaceholder')}
              required
              value={display}
              onChange={(e) => setDisplay(e.currentTarget.value)}
              error={displayError && display.length > 0 ? displayError : null}
              data-autofocus
            />

            <Textarea
              label={t('form.description')}
              description={t('form.descriptionHint')}
              placeholder={t('create.descriptionPlaceholder')}
              minRows={2}
              maxRows={4}
              autosize
              value={description}
              onChange={(e) => setDescription(e.currentTarget.value)}
            />

            <Divider label={t('create.workspaceFolder')} labelPosition="left" />

            <Stack gap={6}>
              <Group gap={6} align="flex-end" wrap="nowrap">
                <TextInput
                  label={t('create.folderOnDisk')}
                  description={t('create.folderHint', { path: derivedHomePath || '~/.config/katyoti/universes/<id>/' })}
                  placeholder={derivedHomePath || '~/.config/katyoti/universes/...'}
                  value={effectiveHomePath}
                  onChange={(e) => { setHomePath(e.currentTarget.value); setHomePathTouched(true) }}
                  style={{ flex: 1 }}
                />
                <Button
                  variant="default"
                  size="sm"
                  leftSection={<IconFolderOpen size={14} />}
                  onClick={async () => {
                    const w = window as unknown as {
                      go?: { main?: { App?: {
                        PickWorkspaceFolder?: () => Promise<{ path?: string; error?: string }>
                      } } }
                    }
                    const pick = w.go?.main?.App?.PickWorkspaceFolder
                    if (!pick) return
                    try {
                      const res = await pick()
                      if (res?.error) { setSubmitError(res.error); return }
                      if (res?.path) {
                        setHomePath(res.path)
                        setHomePathTouched(true)
                      }
                    } catch (e) {
                      setSubmitError(String(e))
                    }
                  }}
                >
                  {t('create.browse')}
                </Button>
              </Group>
              {homePathTouched && (
                <UnstyledButton
                  onClick={() => { setHomePath(''); setHomePathTouched(false) }}
                  style={{ alignSelf: 'flex-start' }}
                >
                  <Text size="xs" c="dimmed">{t('create.resetDefault')}</Text>
                </UnstyledButton>
              )}
            </Stack>

            <Stack gap={6}>
              <Switch
                label={t('create.withProject')}
                description={t('create.withProjectHint')}
                checked={withProject}
                onChange={(e) => setWithProject(e.currentTarget.checked)}
              />
              {withProject && (
                <TextInput
                  label={t('create.projectDir')}
                  placeholder={t('create.projectDirDefault', { name: effectiveName || '…' })}
                  value={projectDir}
                  onChange={(e) => setProjectDir(e.currentTarget.value)}
                />
              )}
            </Stack>

            <Divider label={t('form.appearance')} labelPosition="left" />

            <Group align="flex-end" gap="md">
              <TextInput
                label={t('form.icon')}
                description={t('form.iconHint')}
                placeholder={derivedIcon || 'A'}
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

            <Divider label={t('form.sidecars')} labelPosition="left" />

            <SidecarPicker
              loading={invLoading}
              inventory={inventory}
              selected={sidecars}
              onChange={setSidecars}
            />

            <Select
              label={t('create.firstGuide')}
              description={t('create.firstGuideHint')}
              placeholder={t('create.firstGuideNone')}
              data={guideChoices}
              value={chosenFirstGuide(firstGuide, guideChoices) ?? null}
              onChange={setFirstGuide}
              disabled={guideChoices.length === 0}
              clearable
              searchable
            />

            {submitError && (
              <Alert color="red" icon={<IconAlertCircle size={16} />} title={t('create.errors.createFailed')}>
                <Text size="xs">{submitError}</Text>
              </Alert>
            )}
          </Stack>
          )}
        </Box>
      </ScrollArea.Autosize>

      {/* Footer */}
      <Box
        style={{
          borderTop: '1px solid var(--mantine-color-default-border)',
          padding: '10px 16px',
          display: 'flex',
          justifyContent: 'flex-end',
          gap: 8,
        }}
      >
        <Button variant="default" onClick={closeCreationModal} disabled={submitting}>
          {t('common.cancel')}
        </Button>
        <Button
          color={color}
          onClick={onSubmit}
          loading={submitting}
          disabled={!formValid}
          data-guide="create-universe.submit"
        >
          {t('create.submit')}
        </Button>
      </Box>
    </Modal>
  )
}

// ── Live preview card ─────────────────────────────────────────────────────

function PreviewCard({
  icon, color, displayName, slug, description,
}: {
  icon:        string
  color:       Color
  displayName: string
  slug:        string
  description: string
}) {
  const t = useT('universes')
  return (
    <Box
      p="md"
      style={{
        borderRadius: 8,
        background: `var(--mantine-color-${color}-light)`,
        display: 'flex',
        gap: 12,
        alignItems: 'center',
        border: `1px solid var(--mantine-color-${color}-light-hover, var(--mantine-color-${color}-3))`,
      }}
    >
      <Avatar
        size={48}
        radius="md"
        color={color}
        variant="filled"
        style={{ fontSize: 16, fontWeight: 700, flexShrink: 0 }}
      >
        {icon || '?'}
      </Avatar>
      <div style={{ flex: 1, minWidth: 0 }}>
        <Title order={5} style={{ margin: 0 }}>{displayName}</Title>
        <Group gap={6} mt={2} wrap="nowrap">
          <Text size="xs" c="dimmed">{t('create.idLabel')}</Text>
          <Badge size="xs" variant="default" radius="sm">{slug}</Badge>
        </Group>
        {description && (
          <Text size="xs" c="dimmed" mt={4} lineClamp={2}>{description}</Text>
        )}
      </div>
    </Box>
  )
}

// ── Sidecar picker ─────────────────────────────────────────────────────────

function SidecarPicker({
  loading, inventory, selected, onChange,
}: {
  loading:   boolean
  inventory: SidecarBinary[]
  selected:  string[]
  onChange:  (next: string[]) => void
}) {
  const t = useT('universes')
  const [search, setSearch] = useState('')

  if (loading) {
    return <Text size="xs" c="dimmed">{t('sidecars.scanning')}</Text>
  }
  if (inventory.length === 0) {
    return (
      <Text size="xs" c="dimmed">
        {t('sidecars.noneOnPath')}
        <code>{' ktw-univers scan-inventory'}</code> {t('sidecars.noneOnPathAfter')} {/* i18n-ignore */}
      </Text>
    )
  }
  // Sort alphabetically + admin sidecar (ktw-univers) first if present.
  const sorted = [...inventory].sort((a, b) => {
    if (a.name === 'ktw-univers' && b.name !== 'ktw-univers') return -1
    if (b.name === 'ktw-univers' && a.name !== 'ktw-univers') return  1
    return a.name.localeCompare(b.name)
  })

  // Apply search filter — case-insensitive substring match on the
  // binary name. Selection state is preserved across filtering (a sidecar
  // can stay selected even when filtered out — the visible toggle and
  // counter reflect that distinction).
  const filterTerm = search.trim().toLowerCase()
  const visible = filterTerm
    ? sorted.filter((s) => s.name.toLowerCase().includes(filterTerm))
    : sorted

  const allVisibleSelected = visible.length > 0 && visible.every((s) => selected.includes(s.name))
  const noneSelected = selected.length === 0

  // Toggle-all operates on the CURRENTLY VISIBLE set only — this is
  // expected UX for filtered lists (avoids the surprise of "Select all"
  // mass-selecting binaries the user can't see). When no filter is
  // active, behaves like global toggle.
  const toggleVisible = () => {
    if (allVisibleSelected) {
      onChange(selected.filter((n) => !visible.some((v) => v.name === n)))
    } else {
      const next = new Set(selected)
      visible.forEach((v) => next.add(v.name))
      onChange([...next])
    }
  }

  return (
    <Stack gap="xs">
      {/* Sticky toolbar — search + counters + bulk toggle. */}
      <Group gap="xs" wrap="nowrap">
        <TextInput
          placeholder={t('sidecars.search')}
          leftSection={<IconSearch size={14} />}
          size="xs"
          value={search}
          onChange={(e) => setSearch(e.currentTarget.value)}
          style={{ flex: 1 }}
        />
        <UnstyledButton onClick={toggleVisible} disabled={visible.length === 0}>
          <Text size="xs" c={visible.length === 0 ? 'dimmed' : 'blue'}>
            {allVisibleSelected ? t('sidecars.deselectVisible') : t('sidecars.selectVisible')}
          </Text>
        </UnstyledButton>
      </Group>
      <Group justify="space-between" gap={4} px={2}>
        <Text size="xs" c="dimmed">
          {filterTerm
            ? t('sidecars.matchCount', { count: visible.length, total: sorted.length })
            : t('sidecars.installedCount', { count: sorted.length })}
          {' · '}
          <Text component="span" size="xs" fw={500}>{t('sidecars.selectedCount', { count: selected.length })}</Text>
        </Text>
        {selected.length > 0 && filterTerm && (
          <UnstyledButton onClick={() => onChange([])}>
            <Text size="xs" c="dimmed">{t('sidecars.clearAll')}</Text>
          </UnstyledButton>
        )}
      </Group>

      {/* Scrollable list. */}
      <Box
        style={{
          maxHeight: 200,
          overflowY: 'auto',
          border: '1px solid var(--mantine-color-default-border)',
          borderRadius: 6,
          padding: 8,
          background: 'var(--mantine-color-dark-8)',
        }}
      >
        {visible.length === 0 ? (
          <Text size="xs" c="dimmed" ta="center" py="md">
            {t('sidecars.noMatch', { term: JSON.stringify(filterTerm) })}
          </Text>
        ) : (
          <Stack gap={4}>
            {visible.map((s) => {
              const isSelected = selected.includes(s.name)
              return (
                <UnstyledButton
                  key={s.name}
                  onClick={() => {
                    if (isSelected) onChange(selected.filter((n) => n !== s.name))
                    else onChange([...selected, s.name])
                  }}
                  style={{
                    padding: '6px 8px',
                    borderRadius: 4,
                    background: isSelected ? 'var(--mantine-color-blue-light)' : 'transparent',
                    transition: 'background 80ms ease',
                  }}
                  styles={{ root: { '&:hover': { background: 'var(--mantine-color-dark-6)' } } }}
                >
                  <Group gap="xs" wrap="nowrap" align="flex-start">
                    <Checkbox
                      checked={isSelected}
                      readOnly
                      tabIndex={-1}
                      style={{ marginTop: 2 }}
                    />
                    <Box style={{ flex: 1, minWidth: 0 }}>
                      <Group gap={6} wrap="nowrap" mb={1}>
                        <Text size="sm" fw={500}>{s.name}</Text>
                        {s.version && (
                          <Badge size="xs" variant="default" radius="sm">{s.version}</Badge>
                        )}
                        {s.source && (
                          <Badge size="xs" variant="outline" radius="sm" color="gray">{s.source}</Badge>
                        )}
                      </Group>
                      <Text size="xs" c="dimmed" truncate>{s.binaryPath}</Text>
                    </Box>
                  </Group>
                </UnstyledButton>
              )
            })}
          </Stack>
        )}
      </Box>

      {noneSelected && (
        <Text size="xs" c="dimmed" style={{ fontStyle: 'italic' }}>
          {t('sidecars.noneSelectedHint')}
        </Text>
      )}
    </Stack>
  )
}

// ── Helpers ────────────────────────────────────────────────────────────────

function slugify(s: string): string {
  return s
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(0, 64)
}

function firstChar(s: string): string {
  const trimmed = s.trim()
  if (!trimmed) return ''
  return trimmed[0]?.toUpperCase() ?? ''
}

function defaultHomePath(slug: string): string {
  // Best-effort default — mirrors what the Go side computes when HomePath
  // is empty: <GlobalHomeDir>/universes/<slug>/ where GlobalHomeDir is
  // ~/.config/katyoti/ (the global katyoti config root). Shown for
  // transparency so the user can override it via the input field or the
  // "Browse..." picker.
  return `~/.config/katyoti/universes/${slug}/`
}
