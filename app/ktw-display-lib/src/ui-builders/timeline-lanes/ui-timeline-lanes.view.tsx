// SPDX-License-Identifier: Apache-2.0
// UITimelineLanesView — lanes on a shared date axis, drawn
// as the delivery mock-up's timeline: a card scrolling sideways (its inner
// width at least 980px); an axis row (a label, a tick every Monday); a lane
// per element — its header cell (the lane type's ui-view variant, framed by
// a 3px rule in the lane's colour) and its track: a dashed line at today, the
// backlog chip at its start, its items as cards placed by date (the item
// type's ui-view variant; one with no date at today, dashed), colliding
// cards stacked in rows, the selected one outlined.

import { HoverCard, Stack } from '@mantine/core'
import { Fragment, type CSSProperties, type KeyboardEvent, type ReactNode } from 'react'

import { MONO } from '../fields/_planning/marks.tsx'
import { color } from '../fields/_planning/palette.ts'
import { UIViewView } from '../view/index.ts'
import type { UITimelineLanes } from './ui-timeline-lanes.builder.ts'
import {
  axisRange, backlogLabel, backlogPct, cardPct, datePct, laneColor, laneItems, mondayTicks, parseDay, rowTop,
  stackRows, todayOf, trackHeight,
} from './ui-timeline-lanes.utils.ts'

type Row = Record<string, unknown>

export interface UITimelineLanesViewProps {
  timeline: UITimelineLanes
  /** The lanes. */
  data: unknown[]
  /** A click (or Enter / Space) on an item card. */
  onItemSelect?: (item: Row) => void
  /** Whether an item card is the selected one. */
  isSelected?: (item: Row) => boolean
  /** The date `today: true` reads (tests fix it). */
  now?: Date
  /** Extra attributes on the root element (a page node's data-* keys); the view's own win. */
  attrs?: Record<string, string>
}

const COLS = '230px 1fr'

const S = {
  frame: { background: color('surface'), border: `1px solid ${color('line')}`, borderRadius: 10, overflowX: 'auto', minWidth: 0 } as CSSProperties,
  inner: { minWidth: 980, padding: '10px 12px 14px' } as CSSProperties,
  axis: { display: 'grid', gridTemplateColumns: COLS, fontFamily: MONO, fontSize: 11, color: color('muted') } as CSSProperties,
  ticks: { position: 'relative', height: 18 } as CSSProperties,
  tick: { position: 'absolute', transform: 'translateX(-50%)', whiteSpace: 'nowrap' } as CSSProperties,
  lane: { display: 'grid', gridTemplateColumns: COLS, borderTop: `1px solid ${color('line')}`, minHeight: 74 } as CSSProperties,
  head: { padding: '8px 10px 8px 0', display: 'flex', flexDirection: 'column', gap: 4, minWidth: 0 } as CSSProperties,
  track: { position: 'relative' } as CSSProperties,
  today: { position: 'absolute', top: 0, bottom: 0, width: 0, borderLeft: `2px dashed ${color('block')}`, opacity: 0.6 } as CSSProperties,
  backlog: {
    position: 'absolute', left: 6, top: 20, display: 'inline-flex', alignItems: 'center', gap: 4,
    fontFamily: MONO, fontSize: 12, lineHeight: 1.5, padding: '1px 8px', borderRadius: 999,
    border: `1px solid ${color('line')}`, color: color('muted'), whiteSpace: 'nowrap', background: color('surface'),
  } as CSSProperties,
  card: {
    position: 'absolute', transform: 'translateX(-50%)', background: color('surface'), borderRadius: 9,
    padding: '5px 9px', minWidth: 126, width: 'max-content', maxWidth: 240, cursor: 'pointer', display: 'flex', flexDirection: 'column', gap: 2,
    fontSize: 12.5, color: 'inherit', textAlign: 'left',
  } as CSSProperties,
}

function onKeys(run: () => void) {
  return (e: KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); run() }
  }
}

export function UITimelineLanesView({ timeline, data, onItemSelect, isSelected, now, attrs }: UITimelineLanesViewProps) {
  const def = timeline.definition
  const lanes = (Array.isArray(data) ? data : []).filter((l): l is Row => !!l && typeof l === 'object')
  const today = todayOf(def.today, now)
  const range = axisRange(lanes, def, today)
  const ticks = mondayTicks(range)
  const todayPct = today !== undefined ? datePct(today, range) : undefined

  return (
    <section {...attrs} data-ktw-timeline-lanes="" aria-label={def.axisLabel || 'Timeline'} style={S.frame}>
      <div style={S.inner}>
        <div data-part="axis" style={S.axis}>
          <span>{def.axisLabel ?? ''}</span>
          <div style={S.ticks}>
            {ticks.map((t) => <span key={t.day} data-tick={t.label} style={{ ...S.tick, left: `${t.pct}%` }}>{t.label}</span>)}
          </div>
        </div>
        {lanes.map((lane, li) => (
          <Lane key={timeline.laneId(lane) || li} timeline={timeline} lane={lane} index={li}
            range={range} today={today} todayPct={todayPct} onItemSelect={onItemSelect} isSelected={isSelected} />
        ))}
      </div>
    </section>
  )
}

function Lane({ timeline, lane, index, range, today, todayPct, onItemSelect, isSelected }: {
  timeline: UITimelineLanes; lane: Row; index: number; range: { from: number; to: number }; today?: number; todayPct?: number
  onItemSelect?: (item: Row) => void; isSelected?: (item: Row) => boolean
}) {
  const def = timeline.definition
  const lc = laneColor(lane, index, def.colorField ?? 'color')
  // An item with no date is still drawn: at today (the axis' start without
  // a today line), dashed.
  const undatedDay = today ?? range.from
  const items = laneItems(lane, def).map((it) => {
    const day = parseDay(it[def.itemDate])
    return { it, day: day ?? undatedDay, undated: day === undefined }
  })
  const pcts = items.map((x) => cardPct(x.day, range))
  const backlog = def.backlog ? laneItems(lane, { items: def.backlog }) : []
  // The backlog chip holds the first row's start: a card over it drops a row.
  const chip = backlog.length > 0 ? backlogPct(backlogLabel(backlog.map((e) => timeline.backlogId(e)).filter(Boolean))) : 0
  const rows = stackRows(pcts, undefined, chip)
  const nRows = rows.length ? Math.max(...rows) + 1 : 0

  return (
    <div data-part="lane" data-lane-id={timeline.laneId(lane)} style={{ ...S.lane, ['--lc' as string]: lc }}>
      <div data-part="lane-header" style={{ ...S.head, borderRight: `3px solid ${lc}` }}>
        {timeline.header
          ? <UIViewView view={timeline.header} item={lane} hideTitle enableDetailLevelToggle={false} />
          : <b style={{ fontSize: 15 }}>{timeline.laneTitle(lane)}</b>}
      </div>
      <div data-part="track" style={{ ...S.track, minHeight: trackHeight(nRows) }}>
        {todayPct !== undefined && todayPct >= 0 && todayPct <= 100 && (
          <div data-part="today" title="today" style={{ ...S.today, left: `${todayPct}%` }} />
        )}
        {backlog.length > 0 && <BacklogChip timeline={timeline} entries={backlog} />}
        {items.map(({ it, undated }, k) => {
          const selected = isSelected?.(it) === true
          const select = onItemSelect ? () => onItemSelect(it) : undefined
          return (
            <div key={timeline.itemId(it) || k} role="button" tabIndex={0} aria-pressed={selected}
              data-part="item" data-item-id={timeline.itemId(it)} data-row={rows[k]} data-undated={undated || undefined}
              title={undated ? 'no date' : undefined}
              onClick={select} onKeyDown={select ? onKeys(select) : undefined}
              style={{
                ...S.card, left: `${pcts[k]}%`, top: rowTop(rows[k]), border: `1.5px ${undated ? 'dashed' : 'solid'} ${lc}`,
                boxShadow: selected ? `0 0 0 3px ${color('accent')}` : undefined, zIndex: selected ? 2 : 1,
              }}>
              {timeline.itemView
                ? <UIViewView view={timeline.itemView} item={it} hideTitle enableDetailLevelToggle={false} />
                : <span style={{ fontWeight: 700, fontSize: 13.5 }}>{timeline.itemTitle(it)}</span>}
            </div>
          )
        })}
      </div>
    </div>
  )
}

function BacklogChip({ timeline, entries }: { timeline: UITimelineLanes; entries: Row[] }) {
  const ids = entries.map((e) => timeline.backlogId(e)).filter(Boolean)
  const chip: ReactNode = (
    <span data-part="backlog" title={ids.join(', ')} style={S.backlog}>{backlogLabel(ids)}</span>
  )
  const view = timeline.backlogView
  if (!view) return chip
  return (
    <HoverCard openDelay={150} position="bottom-start" shadow="md" withinPortal>
      <HoverCard.Target>{chip}</HoverCard.Target>
      <HoverCard.Dropdown>
        <Stack gap={6}>
          {entries.map((e, i) => (
            <Fragment key={timeline.backlogId(e) || i}>
              <UIViewView view={view} item={e} hideTitle enableDetailLevelToggle={false} />
            </Fragment>
          ))}
        </Stack>
      </HoverCard.Dropdown>
    </HoverCard>
  )
}
