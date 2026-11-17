// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from 'react'
import { Alert, Autocomplete, Avatar, Badge, Button, Group, Select, Stack, Text, TextInput } from '@mantine/core'
import { IconAlertCircle, IconDeviceDesktop } from '@tabler/icons-react'
import { profileClient } from '../../ktsp'
import type { SuggestOutput, UpdateInput } from '../../ktsp/profile'
import { displayName, initials, isPlaceholder, useProfileStore, type Profile } from '../../store/profileStore'
import { useT } from '../../i18n'
import { draftFromSuggestion, draftOf, draftSavable, saveError } from './profileDraft'

type EmailCandidate = NonNullable<SuggestOutput['emails']>[number]

/** The profile languages (profile/api.SupportedLanguages); unset follows the OS.
 *  Each language is named in itself, whatever the GUI language. */
const AUTO_LANGUAGE = 'auto'
const LANGUAGES = [
  { value: 'en', label: 'English' },
  { value: 'fr', label: 'Français' },
]

interface Props {
  /** After a save the service accepted. */
  onSaved: () => void
  /** Shown as Cancel when given. */
  onCancel?: () => void
  /** The `data-element` of the Save button, for a place that names it. */
  saveElement?: string
  /** The prefix of its fields' `data-element` (`<prefix>-handle`, `-first-name`,
   *  `-last-name`, `-email`), for a place that names them. */
  fieldElement?: string
}

/**
 * The profile form - who the person using KaTyoti says they are - in the
 * Profile view (the profile menu's modal) and in place on « Prêt à démarrer ».
 * While no profile is set (the John Doe placeholder) it opens
 * pre-filled from what this computer knows about the user (`profile/suggest`);
 * nothing is saved until Save. Validation is the service's: its message is
 * shown as is, so the rule lives in one place.
 */
export function ProfileForm({ onSaved, onCancel, saveElement, fieldElement }: Props) {
  const field = (name: string) => (fieldElement ? `${fieldElement}-${name}` : undefined)
  const profile = useProfileStore((s) => s.profile)
  const save = useProfileStore((s) => s.save)
  const t = useT('profile')

  const [draft, setDraft] = useState<UpdateInput>(() => draftOf(profile))
  const [suggestion, setSuggestion] = useState<SuggestOutput | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let live = true
    profileClient().suggest()
      .then((s) => {
        if (!live) return
        setSuggestion(s)
        if (isPlaceholder(profile)) setDraft((d) => ({ ...draftFromSuggestion(s), language: d.language }))
      })
      .catch((err) => {
        // Suggestions are a convenience: the form still works without them.
        // eslint-disable-next-line no-console
        console.warn('[profile] suggest failed:', err)
      })
    return () => { live = false }
    // Seeded once, when the form appears - not when a save updates the profile.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const emails: EmailCandidate[] = suggestion?.emails ?? []
  const sourceOf = new Map(emails.map((e) => [e.email, e.source]))
  const preview: Profile = { ...draft, source: profile.source }
  const set = (k: keyof UpdateInput) => (v: string) => setDraft((d) => ({ ...d, [k]: v }))

  const onSave = async () => {
    setSaving(true)
    setError(null)
    try {
      await save(draft)
      onSaved()
    } catch (err) {
      setError(saveError(err))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Stack gap="md">
      <Group gap="md" wrap="nowrap">
        <Avatar size={52} radius="xl" color="violet" variant="light" style={{ fontWeight: 700 }}>
          {draft.id || draft.firstName || draft.lastName ? initials(preview) : '?'}
        </Avatar>
        <Stack gap={2} style={{ minWidth: 0 }}>
          <Text fw={600} truncate>{draft.id ? displayName(preview) : t('whoAreYou')}</Text>
          <Text size="xs" c="dimmed" truncate>{draft.email || t('noEmail')}</Text>
          {isPlaceholder(profile)
            ? <Badge size="xs" color="orange" variant="light">{t('notSet')}</Badge>
            : <Badge size="xs" color="gray" variant="light">{t('local')}</Badge>}
        </Stack>
      </Group>

      <TextInput
        label={t('handle')}
        description={t('handleHelp')}
        placeholder={t('handlePlaceholder')}
        value={draft.id}
        onChange={(e) => set('id')(e.currentTarget.value)}
        name="profile-handle"
        data-element={field('handle')}
        data-autofocus
      />
      <Group grow>
        <TextInput label={t('firstName')} value={draft.firstName} onChange={(e) => set('firstName')(e.currentTarget.value)} name="profile-first-name" data-element={field('first-name')} />
        <TextInput label={t('lastName')} value={draft.lastName} onChange={(e) => set('lastName')(e.currentTarget.value)} name="profile-last-name" data-element={field('last-name')} />
      </Group>
      <Autocomplete
        label={t('email')}
        description={emails.length > 0 ? t('emailHelp') : undefined}
        placeholder={t('emailPlaceholder')}
        value={draft.email}
        onChange={set('email')}
        data={emails.map((e) => e.email)}
        name="profile-email"
        data-element={field('email')}
        renderOption={({ option }) => (
          <Group justify="space-between" wrap="nowrap" w="100%">
            <Text size="sm" truncate>{option.value}</Text>
            <Badge size="xs" variant="light" color="gray">{sourceOf.get(option.value)}</Badge>
          </Group>
        )}
      />

      <Select
        label={t('language')}
        description={t('languageHelp')}
        data={[{ value: AUTO_LANGUAGE, label: t('languageAuto') }, ...LANGUAGES]}
        value={draft.language || AUTO_LANGUAGE}
        onChange={(v) => set('language')(!v || v === AUTO_LANGUAGE ? '' : v)}
        allowDeselect={false}
      />

      {error && (
        <Alert color="red" variant="light" icon={<IconAlertCircle size={16} />} py="xs">
          {error}
        </Alert>
      )}

      <Group justify="space-between">
        <Button
          variant="subtle"
          size="xs"
          leftSection={<IconDeviceDesktop size={14} />}
          disabled={!suggestion}
          onClick={() => suggestion && setDraft((d) => ({ ...draftFromSuggestion(suggestion), language: d.language }))}
        >
          {t('fill')}
        </Button>
        <Group gap="xs">
          {onCancel && <Button variant="default" onClick={onCancel}>{t('cancel')}</Button>}
          <Button onClick={onSave} loading={saving} disabled={!draftSavable(draft)} data-element={saveElement}>{t('save')}</Button>
        </Group>
      </Group>
    </Stack>
  )
}
