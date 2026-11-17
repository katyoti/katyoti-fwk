// SPDX-License-Identifier: Apache-2.0
/**
 * treeSettings — what a row of the projects tree draws beside the object's
 * icon and name. The row never shows the object's module (its
 * code mostly repeats the name and cut long names); the type badge shows
 * unless the universe hides the tree's badges (`badge:tree`) or the person
 * turned « Show kinds » off in the tree's settings menu (GuiSettings.yaml,
 * `tree.hideTypes`).
 */
import type { TreeSettings } from '../../types/settings'

/** The element ids of the tree's settings menu: its icon and its items. */
export const TREE_SETTINGS = 'tree.settings'
export const TREE_SETTING_SHOW_TYPES = 'tree.settings.show-types'

/** The badges a tree row draws. */
export interface TreeRowBadges {
  type: boolean
}

/** The badges of a row: the type badge when neither the universe nor the
 *  person's setting hides it. */
export function treeRowBadges(settings: TreeSettings | undefined, universeHidesBadges: boolean): TreeRowBadges {
  return { type: !universeHidesBadges && !settings?.hideTypes }
}
