// SPDX-License-Identifier: Apache-2.0
// Pure presentational components shared by every view: the page DSL's
// renderers and the hand-written consoles draw a figure, a titled panel, an
// item, a state and an empty view through these, never their own.

export { KpiTile, kpiTileStyle, softLine, trendTone, sparkPath, type KpiTileProps } from './KpiTile.tsx'
export { SectionCard, type SectionCardProps } from './SectionCard.tsx'
export { ItemCard, toneBorder, PULSE_CSS, type ItemCardProps } from './ItemCard.tsx'
export { StateBadge, stateTone, type StateBadgeProps } from './StateBadge.tsx'
export { EmptyState, type EmptyStateProps } from './EmptyState.tsx'
