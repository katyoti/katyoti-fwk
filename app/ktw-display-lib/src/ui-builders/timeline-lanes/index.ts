// SPDX-License-Identifier: Apache-2.0
// Public surface of the timeline-lanes package: the
// `ui-timeline-lanes` extension, its builder, its view and pure helpers.

export type { UITimelineLanesDef, UITimelineLanesRef } from './ui-timeline-lanes.schema.ts'
export { TIMELINE_LANES_REFERENCES } from './ui-timeline-lanes.schema.ts'

export {
  buildUITimelineLanes,
  buildUITimelineLanesVariants,
  toUITimelineLanes,
  toUITimelineLanesDef,
  type BuildUITimelineLanesParams,
  type UITimelineLanes,
} from './ui-timeline-lanes.builder.ts'

export { UITimelineLanesView, type UITimelineLanesViewProps } from './ui-timeline-lanes.view.tsx'

export {
  axisRange, backlogLabel, cardPct, datePct, dayLabel, elementSchema, laneColor, laneItems, mondayTicks,
  backlogPct, parseDay, rowTop, stackRows, todayOf, trackHeight,
  CARD_MAX_PCT, CARD_MIN_PCT, MIN_GAP_PCT, LANE_TOKENS,
  type AxisRange, type Tick,
} from './ui-timeline-lanes.utils.ts'
