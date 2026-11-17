// SPDX-License-Identifier: Apache-2.0
/**
 * The guide in ktw-gui (modules/guide/): the player, the guide tab/drawer
 * content, and `startRecipe(uri)` for anything that starts a recipe (the chat
 * card, the first-run choice).
 */

export { startRecipe, useGuidePlayer } from './player'
export type { CompletionHow, MissingControl } from './player'
export { GuidePlayer } from './GuidePlayer'
export { GuideBrowser } from './GuideBrowser'
export { GuideDrawer } from './GuideDrawer'
export { GuideReader } from './GuideReader'
export { GuideAtStart } from './GuideAtStart'
export { GUIDE_TAB_ID, PROJECT_PANEL_TOPICS, topicsForTabKind, useGuideDrawer } from './surface'
export type { GuideTabPayload } from './surface'
export type { RecipeEntry } from './recipes'
