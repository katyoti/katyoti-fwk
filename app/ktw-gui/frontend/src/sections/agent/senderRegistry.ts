// SPDX-License-Identifier: Apache-2.0
/**
 * Sender Registry
 * ─────────────────────────────────────────────────────────────────────────
 * Well-known participants. Indexed by the sender key stored in messages.
 * Any unknown key falls back to a generic entry so new participants work
 * automatically without requiring a code change.
 */
import type { ComponentType } from 'react'
import {
  IconBolt,
  IconRobot,
  IconUser,
} from '@tabler/icons-react'
import { tFor } from '../../i18n'
import type { TFn } from '../../i18n'

export interface SenderDef {
  key:   string
  name:  string
  color: string             // Mantine color name
  Icon:  ComponentType<{ size?: number | string }>
  // avatar?: string        // future: image URL for custom participants
}

const SENDER_REGISTRY: Record<string, SenderDef> = {
  user: {
    key:   'user',
    name:  'User',
    color: 'orange',
    Icon:  IconUser,
  },
  assistant: {
    key:   'assistant',
    // Neutral label — the ACTIVE model (which varies per session /
    // provider) is shown authoritatively in the StatusBar from the
    // backend `stats` event. A hardcoded model name here was wrong
    // whenever the session ran a different model (e.g. gpt-5.4-mini).
    name:  'Assistant',
    color: 'blue',
    Icon:  IconRobot,
  },
  system: {
    key:   'system',
    name:  'System',
    color: 'gray',
    Icon:  IconBolt,
  },
}

/**
 * Look up a sender by key.
 * Unknown keys return a generic gray entry using the key as display name.
 * A well-known sender's name is the GUI's word, in the `chat` namespace (`t`).
 */
export function getSender(key: string, t: TFn = tFor('chat')): SenderDef {
  const known = SENDER_REGISTRY[key]
  if (known) return { ...known, name: t(`sender.${key}`, { defaultValue: known.name }) }
  return {
    key,
    name:  key,
    color: 'gray',
    Icon:  IconUser,
  }
}
