// SPDX-License-Identifier: Apache-2.0
// Icon-related types.
//
// IconName is the type used everywhere a Tabler icon name is expected
// (e.g., on UIAction.icon, in schema's icon content-type configs). Kept
// as a wide string for ergonomic authoring; runtime lookup is dynamic
// through the lazily loaded icon set in byName.tsx.
//
// Lives here (not in src/schema/) per the 2026-05-03 strict cleanup of
// schema-contents.ts: type-specific aliases moved out to their owning
// UI/content-type folders.

import type * as TablerIcons from '@tabler/icons-react'

/** Concrete keyof Tabler's exported icon components — used internally by
 *  Icon.tsx for the dynamic lookup. */
export type TablerIconName = keyof typeof TablerIcons

/** Wide string alias documenting "Tabler icon name expected here".
 *  E.g., 'IconPlus', 'IconTrash', 'IconPencil'. */
export type IconName = string
