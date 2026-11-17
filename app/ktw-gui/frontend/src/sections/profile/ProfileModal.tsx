// SPDX-License-Identifier: Apache-2.0
import { Group, Modal, Text } from '@mantine/core'
import { IconUser } from '@tabler/icons-react'
import { useProfileStore } from '../../store/profileStore'
import { useT } from '../../i18n'
import { ProfileForm } from './ProfileForm'

/**
 * The Profile view — who the person using KaTyoti says they are.
 *
 * Opened from the ribbon's profile menu: a way back to the profile, never the
 * only way - the first step of « Prêt à démarrer » asks it in place.
 * The form is the same one (`ProfileForm`), seeded each time the
 * view opens (Mantine unmounts a closed modal's content).
 */
export function ProfileModal() {
  const opened = useProfileStore((s) => s.modalOpen)
  const close = useProfileStore((s) => s.closeModal)
  const t = useT('profile')

  return (
    <Modal opened={opened} onClose={close} title={<Group gap="xs"><IconUser size={16} /><Text fw={600}>{t('title')}</Text></Group>} size="md" centered>
      <ProfileForm onSaved={close} onCancel={close} />
    </Modal>
  )
}
