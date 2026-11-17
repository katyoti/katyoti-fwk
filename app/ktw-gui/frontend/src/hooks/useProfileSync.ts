// SPDX-License-Identifier: Apache-2.0
import { useEffect } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useProfileStore } from '../store/profileStore'

/**
 * Loads the profile at startup and re-reads it whenever it changes anywhere —
 * this GUI, another one, or `katyoti profile` in a terminal (the Go side
 * forwards the admin universe's profile-changed event as `profile.changed`).
 */
export function useProfileSync() {
  const load = useProfileStore((s) => s.load)
  useEffect(() => {
    void load()
    const off = EventsOn('profile.changed', () => { void load() })
    return () => { off() }
  }, [load])
}
