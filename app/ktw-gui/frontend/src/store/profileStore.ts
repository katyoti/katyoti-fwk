// SPDX-License-Identifier: Apache-2.0
/**
 * Profile store — who the person using this GUI says they are.
 *
 * The profile is held by the admin universe's hub (`ktw-univers:profile/*`);
 * this store is a read-through copy the ribbon, the Profile modal and the
 * person-id consumers (RequestsView) share. Until the user sets a profile the
 * service answers with the John Doe placeholder, and so does this store before
 * its first load — the screen never shows a made-up real person.
 */

import { create } from 'zustand'
import { profileClient } from '../ktsp'
import type { GetOutput, UpdateInput } from '../ktsp/profile'

export type Profile = GetOutput

/** Mirrors the service's placeholder (profile/api.Placeholder). */
export const PLACEHOLDER_PROFILE: Profile = {
  id: 'john-doe',
  firstName: 'John',
  lastName: 'Doe',
  email: 'john.doe@example.com',
  source: 'placeholder',
}

export const isPlaceholder = (p: Profile) => p.source === 'placeholder'

export function displayName(p: Profile): string {
  return [p.firstName, p.lastName].filter(Boolean).join(' ') || p.id
}

export function initials(p: Profile): string {
  const f = p.firstName?.trim()?.[0] ?? ''
  const l = p.lastName?.trim()?.[0] ?? ''
  return (f + l || p.id.slice(0, 2)).toUpperCase()
}

interface ProfileState {
  profile: Profile
  modalOpen: boolean
  /** Re-read the profile from the service; keeps the current one on failure. */
  load: () => Promise<void>
  /** Save through the service; throws the service's message on refusal. */
  save: (input: UpdateInput) => Promise<Profile>
  openModal: () => void
  closeModal: () => void
}

export const useProfileStore = create<ProfileState>((set) => ({
  profile: PLACEHOLDER_PROFILE,
  modalOpen: false,
  load: async () => {
    try {
      set({ profile: await profileClient().get() })
    } catch (err) {
      // A hub hiccup keeps what is on screen rather than blanking the menu.
      // eslint-disable-next-line no-console
      console.warn('[profile] get failed:', err)
    }
  },
  save: async (input) => {
    const profile = await profileClient().update(input)
    set({ profile })
    return profile
  },
  openModal: () => set({ modalOpen: true }),
  closeModal: () => set({ modalOpen: false }),
}))

/** The person actor id for what this GUI's user does. */
export const usePersonId = () => useProfileStore((s) => s.profile.id)
