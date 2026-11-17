// SPDX-License-Identifier: Apache-2.0
import type { SuggestOutput, UpdateInput } from '../../ktsp/profile'
import { isPlaceholder, type Profile } from '../../store/profileStore'

/**
 * The profile being filled - in the Profile view (the profile menu's modal) or
 * in place, on the first step of « Prêt à démarrer »: one draft,
 * one rule for when it can be saved, whichever place shows the form.
 */

export const EMPTY_DRAFT: UpdateInput = { id: '', firstName: '', lastName: '', email: '', language: '' }

/** A draft from what this computer knows about the person (`profile/suggest`). */
export function draftFromSuggestion(s: SuggestOutput): UpdateInput {
  return {
    id: s.id ?? '',
    firstName: s.firstName ?? '',
    lastName: s.lastName ?? '',
    email: s.emails?.[0]?.email ?? '',
    language: '',
  }
}

/** The draft a form opens on: the profile as it is, empty for the placeholder. */
export function draftOf(profile: Profile): UpdateInput {
  return isPlaceholder(profile) ? EMPTY_DRAFT : {
    id: profile.id, firstName: profile.firstName ?? '', lastName: profile.lastName ?? '', email: profile.email,
    language: profile.language ?? '',
  }
}

/** A draft can be saved once it says who the person is: a handle, a name and
 *  an e-mail - what makes the profile step green. */
export function draftSavable(d: UpdateInput): boolean {
  return !!d.id.trim() && !!d.email.trim() && !!(d.firstName?.trim() || d.lastName?.trim())
}

/** The service's refusal, without its call prefix. */
export function saveError(err: unknown): string {
  return err instanceof Error ? err.message.replace(/^[^:]*profile\/update: /, '') : String(err)
}
