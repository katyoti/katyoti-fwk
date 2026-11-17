// SPDX-License-Identifier: Apache-2.0
import { useState, type ReactNode } from 'react'
import {
  Avatar,
  Box,
  Button,
  Container,
  Group,
  Loader,
  Paper,
  SimpleGrid,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import {
  IconAlertCircle,
  IconAlertTriangle,
  IconArrowRight,
  IconPlus,
  IconShieldLock,
} from '@tabler/icons-react'
import { useAppStore } from '../store'
import type { Universe } from '../types/universe'
import { KatyotiWelcome } from '../components/katyoti-welcome/KatyotiWelcome'
import { activateOnKey } from '../guide/anchors'
import { FirstRunChoice } from '../guide/FirstRunChoice'
import { useT } from '../i18n'
import { errorTextWith } from '../i18n/errors'
import { useUniverseWarnings } from '../guide/universeWarnings'
import { isAdminUniverse } from '../sections/shell/workUniverses'

/**
 * Full-bleed welcome shell shown on every no-universe-selected screen.
 *
 * The animated KaTyoti scene fills the whole background (it letterboxes itself
 * to a 1774×887 aspect over its own black backdrop). The foreground `children`
 * (the "Pick a universe" cards / onboarding CTA) are anchored at the bottom
 * over a bottom-to-top black gradient scrim that keeps them readable.
 *
 * Pass no `children` to show the animation alone — this is what happens once a
 * universe is selected: the cards drop away and only the scene remains while
 * the universe core spins up.
 */
function WelcomeShell({ children }: { children?: ReactNode }) {
  return (
    <Box style={{ position: 'relative', height: '100%', overflow: 'hidden', backgroundColor: '#000' }}>
      {/* Animated scene — full-bleed background layer. */}
      <Box style={{ position: 'absolute', inset: 0, zIndex: 0 }}>
        <KatyotiWelcome />
      </Box>

      {/* Foreground content, anchored at the bottom over a darkening scrim.
          Omitted entirely while activating, leaving only the animation. */}
      {children && (
        <Box
          style={{
            position: 'absolute',
            insetInline: 0,
            bottom: 0,
            zIndex: 1,
            maxHeight: '100%',
            overflow: 'auto',
            backgroundImage: 'linear-gradient(to bottom, rgba(0,0,0,0) 0%, rgba(0,0,0,0.92) 55%)',
          }}
        >
          {children}
        </Box>
      )}
    </Box>
  )
}

/**
 * Welcome / empty-state main pane shown when no universe is displayed.
 *
 * Renders a card grid of registered universes (admin first, then user-created
 * by name). Cards use SINGLE-CLICK select (cards are large affordances that
 * reduce accidental-switch risk; the IconRibbon's tighter gestures use
 * double-click).
 *
 * If the registry is empty (first launch, fresh install), shows a hero
 * onboarding card with a primary CTA to create the first universe.
 */
export function UniversePicker() {
  const t                = useT('universes')
  const universes        = useAppStore((s) => s.universes)
  const universesLoaded  = useAppStore((s) => s.universesLoaded)
  const universesError   = useAppStore((s) => s.universesError)
  const activateUniverse = useAppStore((s) => s.activateUniverse)
  const openCreate       = useAppStore((s) => s.openCreationModal)
  const setMainPaneRoute = useAppStore((s) => s.setMainPaneRoute)

  // Name of the universe currently being activated. While set, the picker
  // shows only the animation — the selection cards are removed.
  const [activating, setActivating] = useState<string | null>(null)

  const handleSelect = async (name: string) => {
    setActivating(name)
    await activateUniverse(name)
    // On success the route flips to `universe` and this component unmounts.
    // If activation failed (route stays `welcome`), restore the cards so the
    // user can retry.
    setActivating(null)
  }

  // Sort: admin first, then alphabetical by displayName.
  const sorted = [...universes].sort((a, b) => {
    if (a.protected && !b.protected) return -1
    if (!a.protected && b.protected) return  1
    return (a.displayName || a.name).localeCompare(b.displayName || b.name)
  })
  // The admin universe is always listed: first run means no universe of the
  // person's own (the guide's first-run offer keys off this).
  const userUniverses = sorted.filter((u) => !u.protected).length

  // Loading skeleton.
  if (!universesLoaded) {
    return (
      <Box style={{ height: '100%', backgroundColor: '#000', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Stack align="center" gap="xs">
          <Loader size="md" />
          <Text size="sm" c="dimmed">{t('picker.loading')}</Text>
        </Stack>
      </Box>
    )
  }

  // Error state.
  if (universesError) {
    return (
      <Container size="md" py="xl" style={{ height: '100%', backgroundColor: '#000' }}>
        <Paper withBorder p="xl" radius="md">
          <Group gap="md" wrap="nowrap" align="flex-start">
            <IconAlertCircle size={32} color="var(--mantine-color-red-6)" style={{ flexShrink: 0 }} />
            <div>
              <Title order={4}>{t('picker.loadFailed')}</Title>
              <Text size="sm" c="dimmed" mt={4}>{universesError}</Text>
              <Text size="xs" c="dimmed" mt="md">
                {t('picker.checkReadableBefore')} <code>{'~/.config/katyoti/universes.yaml'}</code> {t('picker.checkReadableAfter')}
              </Text>
            </div>
          </Group>
        </Paper>
      </Container>
    )
  }

  // A universe is being activated — drop the cards, keep only the animation
  // while its core spins up.
  if (activating) {
    return <WelcomeShell />
  }

  // Empty registry — onboarding hero (anchored at the bottom over the banner).
  if (sorted.length === 0) {
    return (
      <WelcomeShell>
        <Container size="md" py="xl">
          <Paper withBorder p="xl" radius="lg" style={{ width: '100%', textAlign: 'center', backgroundColor: 'rgba(0,0,0,0.55)' }}>
            <Stack align="center" gap="md">
              <Title order={3}>{t('picker.welcomeTitle')}</Title>
              <Text size="sm" c="dimmed" maw={420}>
                {t('picker.welcomeBody')}
              </Text>
              <Button
                size="md"
                leftSection={<IconPlus size={16} />}
                onClick={openCreate}
                variant="filled"
                color="violet"
                mt="xs"
                data-guide="picker.create"
                data-guide-label={t('picker.createFirst')}
              >
                {t('picker.createFirst')}
              </Button>
            </Stack>
          </Paper>
          <Box mt="md"><FirstRunChoice userUniverses={userUniverses} /></Box>
        </Container>
      </WelcomeShell>
    )
  }

  // Card grid — pick a universe (anchored at the bottom over the banner).
  return (
    <WelcomeShell>
      <Container size="lg" py="xl">
        <Stack gap="lg">
          <div>
            <Title order={3}>{t('picker.title')}</Title>
            <Text size="sm" c="dimmed" mt={4}>
              {t('picker.subtitle')}
            </Text>
          </div>

          <FirstRunChoice userUniverses={userUniverses} />

          <SimpleGrid cols={{ base: 1, sm: 2, md: 3 }} spacing="md">
            {sorted.map((u) => (
              <UniverseCard
                key={u.name}
                universe={u}
                // The admin universe holds no work: its card
                // opens its settings, never the universe as a place to work.
                onSelect={() => { if (isAdminUniverse(u)) setMainPaneRoute({ kind: 'edit', name: u.name }); else void handleSelect(u.name) }}
              />
            ))}
            <CreateUniverseCard onCreate={openCreate} />
          </SimpleGrid>
        </Stack>
      </Container>
    </WelcomeShell>
  )
}

// ── Universe card ─────────────────────────────────────────────────────────

interface UniverseCardProps {
  universe: Universe
  onSelect: () => void
}

function UniverseCard({ universe, onSelect }: UniverseCardProps) {
  const tg = useT('guide')
  // The universe checks that warn about the universe itself (its work not
  // backed up off this machine, …); the admin universe holds no work.
  const warnings = useUniverseWarnings(universe.name, isAdminUniverse(universe))
  return (
    <Paper
      withBorder
      p="md"
      radius="md"
      onClick={onSelect}
      role="button"
      tabIndex={0}
      aria-label={universe.displayName || universe.name}
      onKeyDown={activateOnKey(onSelect)}
      data-guide="picker.universe"
      data-guide-name={universe.name}
      style={{
        cursor: 'pointer',
        transition: 'border-color 120ms ease, transform 120ms ease',
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.borderColor = `var(--mantine-color-${universe.color}-6)`
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.borderColor = ''
      }}
    >
      <Stack gap="sm">
        <Group gap="sm" wrap="nowrap" align="flex-start">
          <Avatar size={42} radius="md" color={universe.color} variant="light" style={{ fontSize: 14, fontWeight: 700 }}>
            {universe.icon}
          </Avatar>
          <div style={{ flex: 1, minWidth: 0 }}>
            <Group gap={6} wrap="nowrap" align="center">
              <Text fw={600} size="sm" truncate>
                {universe.displayName || universe.name}
              </Text>
              {universe.protected && (
                <IconShieldLock size={12} color="var(--mantine-color-dimmed)" />
              )}
            </Group>
            <Text size="xs" c="dimmed" truncate>{universe.name}</Text>
          </div>
        </Group>

        {universe.description && (
          <Text size="sm" c="dimmed" lineClamp={2}>
            {universe.description}
          </Text>
        )}

        {warnings.map((w, i) => (
          <Group key={i} gap={6} wrap="nowrap" align="flex-start" data-guide="picker.universe.warning" data-guide-name={universe.name}>
            <IconAlertTriangle size={14} color="var(--mantine-color-yellow-6)" style={{ flexShrink: 0, marginTop: 2 }} />
            <Text size="xs" c="yellow.6">{errorTextWith(tg, w.detail, w.detailMsg)}</Text>
          </Group>
        ))}

        <Group justify="space-between" align="center" mt="xs">
          <Text size="xs" c="dimmed" truncate maw={180}>
            {universe.homePath}
          </Text>
          <IconArrowRight size={14} style={{ opacity: 0.5 }} />
        </Group>
      </Stack>
    </Paper>
  )
}

// ── Create-universe card (visually distinct affordance) ─────────────────

function CreateUniverseCard({ onCreate }: { onCreate: () => void }) {
  const t = useT('universes')
  return (
    <Paper
      withBorder
      p="md"
      radius="md"
      onClick={onCreate}
      role="button"
      tabIndex={0}
      aria-label={t('picker.createAria')}
      onKeyDown={activateOnKey(onCreate)}
      data-guide="picker.create"
      style={{
        cursor: 'pointer',
        borderStyle: 'dashed',
        background: 'transparent',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: 140,
        transition: 'border-color 120ms ease, background 120ms ease',
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.borderColor = 'var(--mantine-color-violet-6)'
        e.currentTarget.style.background = 'var(--mantine-color-violet-light)'
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.borderColor = ''
        e.currentTarget.style.background = 'transparent'
      }}
    >
      <Stack align="center" gap="xs">
        <Box
          style={{
            width: 42,
            height: 42,
            borderRadius: 8,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            background: 'var(--mantine-color-violet-light)',
          }}
        >
          <IconPlus size={20} color="var(--mantine-color-violet-6)" />
        </Box>
        <Text fw={600} size="sm">{t('create.title')}</Text>
        <Text size="xs" c="dimmed" ta="center">
          {t('picker.createCardHint')}
        </Text>
      </Stack>
    </Paper>
  )
}
