// SPDX-License-Identifier: Apache-2.0
// UITimelineLanesDef — the DSL `ui-timeline-lanes` extension shape:
// a lane per element of the data, on a shared date axis.
//
// Declared once on the LANE type (the data is a list of lanes):
//
//   extensions:
//     - type: ui-timeline-lanes
//       name: default
//       laneHeader: lane-header   # a ui-view variant of the lane type
//       items: releases           # the lane's field holding its items
//       itemDate: date            # the items' date field (YYYY-MM-DD)
//       itemView: card            # a ui-view variant of the ITEM type
//       backlog: backlog          # a lane field drawn as a chip at its start
//       select: release           # the selection key a click on an item sets
//       today: true               # a dashed line at today (or a fixed date)

import type { SchemaTypeExtension } from '../../schema/index.ts'

export type UITimelineLanesDef = SchemaTypeExtension<{
  type: 'ui-timeline-lanes'

  /** Variant name. Defaults to 'default'. */
  name?: string

  /** A ui-view variant of the lane type drawn in the lane's header cell
   *  (its actions run with the lane as the item). Without it the header is
   *  the lane's title. */
  laneHeader?: string

  /** The lane's field holding its items (an array of objects). */
  items: string

  /** The items' date field (YYYY-MM-DD, or any date `Date` reads). */
  itemDate: string

  /** A ui-view variant of the ITEM type (the `items` field's element type)
   *  drawing each card. Without it a card is the item's title. */
  itemView?: string

  /** A lane field (an array) drawn as one chip at the lane's start:
   *  « Backlog <n>: <first ids>… », every id in its title. */
  backlog?: string

  /** A ui-view variant of the backlog's element type, drawing each entry in
   *  the chip's hover card. Without it the chip lists ids. */
  backlogView?: string

  /** The selection key a click on an item sets to its id; the item whose id
   *  is that key's value is outlined. */
  select?: string

  /** A dashed line at today: `true` for the current date, a date string for
   *  a fixed one, absent or false for none. */
  today?: boolean | string

  /** The axis' first day. Default: the earliest item date (or today, when
   *  earlier) minus 7 days. */
  from?: string

  /** The axis' last day. Default: the latest item date (or today, when
   *  later) plus 7 days. */
  to?: string

  /** The lane field holding its colour (a palette token or a CSS colour).
   *  Default 'color'; a lane without one takes a palette colour by its
   *  index. */
  colorField?: string

  /** The axis row's left cell (« Stream · release mode »). Default: none. */
  axisLabel?: string
}>

/** UITimelineLanesRef — a variant name, or a definition inline. */
export type UITimelineLanesRef = string | UITimelineLanesDef

/** The extension types a composition's fields name a variant of — what a
 *  host keeps on the schema it resolves for it (Go mirror:
 *  render.ExtensionReferences). */
export const TIMELINE_LANES_REFERENCES: Record<string, 'ui-view'> = {
  laneHeader: 'ui-view',
  itemView: 'ui-view',
  backlogView: 'ui-view',
}
