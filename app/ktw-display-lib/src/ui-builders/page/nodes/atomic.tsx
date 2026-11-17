// SPDX-License-Identifier: Apache-2.0
// Atomic node renderers (13 kinds): heading, text, divider, badge, link,
// image, video, audio, file, timeline, kpi, code-block, markdown.
//
// Each renderer is a thin Mantine wrapper over the corresponding DSL
// node interface. Media nodes (video / audio / file) use native HTML
// elements + Mantine chrome. Timeline maps to Mantine's Timeline
// component. (The `chart` node lives in chart.tsx — see dsl-chart-node.)

import { useEffect, useRef, useState, type ReactNode } from 'react'
import i18next from 'i18next'
import {
  ActionIcon,
  Anchor,
  Badge,
  Box,
  Button,
  Card,
  Code,
  Collapse,
  CopyButton,
  Divider,
  Group,
  Image,
  Spoiler,
  Stack,
  Text,
  ThemeIcon,
  Timeline,
  Title,
  Tooltip,
  UnstyledButton,
} from '@mantine/core'
import { useItem, useRunAction } from '../page-context.tsx'
import { monogramLetters } from './marks.ts'
import { dataAttrs } from './data-attrs.ts'
import { showLessLabel, showMoreLabel } from './window.tsx'
import { tablerIcon } from '../../../ui-core/components/icons/index.ts'
import { KpiTile } from '../../../ui-core/components/other/index.ts'
import {
  IconCheck,
  IconCopy,
  IconDownload,
  IconFile,
} from '@tabler/icons-react'
import ReactMarkdown, { defaultUrlTransform } from 'react-markdown'
import { MONO, usePlanningT } from '../../fields/_planning/marks.tsx'
import { color as tone, statusTone, toneColors, wordLabel } from '../../fields/_planning/palette.ts'
import remarkGfm from 'remark-gfm'

import type {
  UIAudioNode,
  UIBadgeNode,
  UIMonogramNode,
  UICodeBlockNode,
  UIDividerNode,
  UIFileNode,
  UIHeadingNode,
  UIImageNode,
  UIKPINode,
  UILegendNode,
  UILinkNode,
  UIMarkdownNode,
  UIPageNode,
  UITextNode,
  UITimelineNode,
  UIVideoNode,
} from '../ui-page.types.ts'

// =================================================================================================
// heading / text / divider
// =================================================================================================

/** A section's small label: mono, upper case, spaced, muted (the planning
 *  views' « SHIPS IN THIS RELEASE (8) »). */
const LABEL_STYLE = {
  fontFamily: MONO, fontSize: 11, fontWeight: 600, letterSpacing: '0.08em', textTransform: 'uppercase', margin: '6px 0 2px',
} as const

export function HeadingRenderer({ node }: { node: UIHeadingNode }) {
  if (node.variant === 'label') {
    return <div {...dataAttrs(node)} role="heading" aria-level={node.level ?? 4} data-heading="label" style={{ ...LABEL_STYLE, color: node.color ?? tone('muted') }}>{node.text}</div>
  }
  const title = (
    <Title {...(node.icon ? {} : dataAttrs(node))} order={node.level ?? 2} c={node.icon ? undefined : node.color}>
      {node.text}
    </Title>
  )
  if (!node.icon) return title
  const Icon = tablerIcon(node.icon, IconFile)
  return (
    <Group {...dataAttrs(node)} gap="sm" wrap="nowrap">
      <ThemeIcon variant="light" color={node.color ?? 'blue'} radius="md" size={(node.level ?? 2) <= 3 ? 'lg' : 'md'}>
        <Icon size={16} />
      </ThemeIcon>
      {title}
    </Group>
  )
}

/** The words a legend names by default: the Workflows view's. */
const LEGEND_LABELS: Record<string, string> = { waiting: 'awaiting a person', todo: 'to do' }

/** LegendRenderer draws the planning palette's legend: a small square in
 *  each word's tone and its label. */
export function LegendRenderer({ node }: { node: UILegendNode }) {
  const t = usePlanningT()
  const items = (node.items ?? []).map((e) => typeof e === 'string' ? { word: e } : e).filter((e) => e && e.word)
  return (
    <div {...dataAttrs(node)} data-ktw-legend="" style={{ display: 'flex', flexWrap: 'wrap', gap: '4px 14px', fontSize: 12, color: tone('muted') }}>
      {items.map((e) => {
        const { fg } = toneColors(statusTone(e.word).tone)
        return (
          <span key={e.word} data-word={e.word} style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}>
            <span style={{ width: 9, height: 9, borderRadius: 2, background: fg, display: 'inline-block' }} />
            {e.label ?? t(`legend.${e.word}`, { defaultValue: LEGEND_LABELS[e.word] ?? wordLabel('status', e.word, t) })}
          </span>
        )
      })}
    </div>
  )
}

/** textOverflows says whether a clamped text holds more than its lines show:
 *  its full height beyond the height drawn (a pixel of rounding aside). */
export function textOverflows(el: { scrollHeight: number; clientHeight: number } | null | undefined): boolean {
  return !!el && el.scrollHeight > el.clientHeight + 1
}

/** TextToggle is the « Afficher plus » / « Afficher moins » button under an
 *  expandable text, `text:<id>:toggle` its element id. Its click stops there:
 *  a text in a row that opens on click (a tree's or a list's item) unfolds,
 *  the row does not open. */
export function TextToggle({ id, open, onToggle }: { id?: string; open: boolean; onToggle: () => void }) {
  return (
    <Button size="compact-xs" variant="subtle" px={0} aria-expanded={open}
      onClick={(e: { stopPropagation: () => void }) => { e.stopPropagation(); onToggle() }}
      data-element={id ? `text:${id}:toggle` : undefined}>
      {open ? showLessLabel(i18next.language) : showMoreLabel(i18next.language)}
    </Button>
  )
}

/** measureOverflow is what a measure of a clamped text says: whether it
 *  overflows its lines, or nothing for an element out of the document - a
 *  removed or replaced box measures 0 and says nothing of the text drawn. */
export function measureOverflow(el: { scrollHeight: number; clientHeight: number; isConnected?: boolean } | null | undefined): boolean | undefined {
  if (!el || el.isConnected === false) return undefined
  return textOverflows(el)
}

/** ClampedText draws an expandable text and, once it overflows or is open,
 *  its toggle under it. The text keeps its place in the tree whether the
 *  toggle shows or not: a text moved under a new parent is a new element,
 *  the measured one gone. */
export function ClampedText({ text, id, open, overflows, onToggle }: {
  text: ReactNode; id?: string; open: boolean; overflows: boolean; onToggle: () => void
}) {
  return (
    <Box>
      {text}
      {(overflows || open) && <TextToggle id={id} open={open} onToggle={onToggle} />}
    </Box>
  )
}

/** TextRenderer draws a text: clamped to `lineClamp` lines, in the monospace
 *  font with `mono`, and with `expandable` a toggle under it once it is seen
 *  to overflow its lines - measured on the text drawn now, again when its box
 *  is resized and once the fonts are loaded. */
export function TextRenderer({ node }: { node: UITextNode }) {
  const clamp = typeof node.lineClamp === 'number' && node.lineClamp >= 1 ? Math.floor(node.lineClamp) : undefined
  const expandable = !!node.expandable && clamp !== undefined
  const ref = useRef<HTMLParagraphElement>(null)
  const [open, setOpen] = useState(false)
  const [overflows, setOverflows] = useState(false)
  useEffect(() => {
    const el = ref.current
    if (!expandable || open || !el) return
    const measure = () => {
      const seen = measureOverflow(ref.current)
      if (seen !== undefined) setOverflows(seen)
    }
    measure()
    let live = true
    document.fonts?.ready.then(() => { if (live) measure() })
    if (typeof ResizeObserver === 'undefined') return () => { live = false }
    const seen = new ResizeObserver(measure)
    seen.observe(el)
    return () => { live = false; seen.disconnect() }
  }, [expandable, open, clamp, node.content])
  const text = (
    <Text ref={ref} {...dataAttrs(node)} size={node.size} c={node.color} fw={node.fw} fs={node.fs}
      ff={node.mono ? 'monospace' : undefined} lineClamp={open ? undefined : clamp}>
      {node.content}
    </Text>
  )
  if (!expandable) return text
  return <ClampedText text={text} id={node.id} open={open} overflows={overflows} onToggle={() => setOpen((o) => !o)} />
}

export function DividerRenderer({ node }: { node: UIDividerNode }) {
  return (
    <Divider
      {...dataAttrs(node)}
      orientation={node.orientation ?? 'horizontal'}
      label={node.label}
      labelPosition={node.label ? 'center' : undefined}
      color={node.color}
      variant={node.variant}
    />
  )
}

// =================================================================================================
// badge / link / image
// =================================================================================================

export function BadgeRenderer({ node }: { node: UIBadgeNode }) {
  const Icon = node.icon ? tablerIcon(node.icon, IconFile) : undefined
  const size = node.size ?? 'md'
  // A badge with an action is a button: it runs it on the element it is on.
  const run = useRunAction()(node.action)
  const item = useItem()
  // A badge bound to a value that is empty draws nothing: never an empty
  // chip, nor its prefix alone (« écrit sans » with nothing after it).
  if (node.dataField && !node.text) return null
  const textStyle = node.mono ? { fontFamily: 'var(--mantine-font-family-monospace)', textTransform: 'none' as const }
    : node.keepCase ? { textTransform: 'none' as const, maxWidth: 260 } : undefined
  const badge = (
    <Badge {...(run ? {} : dataAttrs(node))} color={node.colors?.[node.text] ?? node.color} variant={node.variant ?? 'light'} size={size}
      leftSection={Icon ? <Icon size={size === 'lg' || size === 'xl' ? 14 : 11} /> : undefined}
      // The badge's own cursor is the arrow; one that runs an action points.
      style={run ? { ...textStyle, cursor: 'pointer' } : textStyle}>
      {node.prefix ? (
        <>
          <Text span size="10px" c="dimmed" ff="monospace" tt="uppercase" mr={6} data-guide="badge.prefix">{node.prefix}</Text>
          <b>{node.text}</b>
        </>
      ) : node.text}
    </Badge>
  )
  const clickable = run ? (
    <UnstyledButton {...dataAttrs(node)} onClick={() => run(item)} aria-label={node.action?.label ?? node.text} style={{ display: 'inline-flex' }}>
      {badge}
    </UnstyledButton>
  ) : badge
  if (!node.tooltip) return clickable
  const multiline = node.tooltipMultiline
  return (
    <Tooltip label={node.tooltip} withArrow multiline={multiline} w={multiline ? (node.tooltipWidth ?? 320) : undefined}
      style={multiline ? { whiteSpace: 'pre-wrap' } : undefined}>
      {clickable}
    </Tooltip>
  )
}

export function MonogramRenderer({ node }: { node: UIMonogramNode }) {
  const size = node.size ?? 24
  const mark = (
    <Box {...dataAttrs(node)} data-guide="monogram" data-guide-name={node.text} aria-label={node.tooltip ?? node.text}
      bg={node.color ?? 'gray.6'} c="white" ff="monospace" fw={600}
      style={{ width: size, height: size, borderRadius: 6, display: 'inline-grid', placeItems: 'center', fontSize: Math.round(size * 0.46), flex: 'none' }}>
      {monogramLetters(node.text)}
    </Box>
  )
  return node.tooltip ? <Tooltip label={node.tooltip} withArrow>{mark}</Tooltip> : mark
}

export function LinkRenderer({ node }: { node: UILinkNode }) {
  const run = useRunAction()(node.action)
  const item = useItem()
  if (node.action) {
    // A link that runs an action (open an object, a panel, a chat…): no
    // href to follow; with no host to run it, it is plain text.
    if (!run) return <Text {...dataAttrs(node)} span c="dimmed" title="Runs in a panel">{node.text}</Text>
    return (
      <Anchor {...dataAttrs(node)} component="button" type="button" size={node.size} fw={node.fw} onClick={() => run(item)}
        style={{ textAlign: 'left' }}>
        {node.text}
      </Anchor>
    )
  }
  const href = node.href ?? ''
  // Auto-detect external when not explicitly set: any href starting with
  // http(s):// (or //) is treated as external.
  const isExternal =
    node.external ?? /^(https?:)?\/\//.test(href)
  return (
    <Anchor
      {...dataAttrs(node)}
      href={href}
      size={node.size}
      fw={node.fw}
      target={isExternal ? '_blank' : undefined}
      rel={isExternal ? 'noopener noreferrer' : undefined}
    >
      {node.text}
    </Anchor>
  )
}

export function ImageRenderer({ node }: { node: UIImageNode }) {
  // When the image lives inside a stretched container (a 2-col grid where
  // a sibling card has more content), a fixed `height` leaves whitespace
  // below the image and looks broken. Treat the LLM-provided height as a
  // MINIMUM (`minHeight`) and let the image fill any extra vertical space
  // its parent gives it. Width-100% + minWidth:0 lets it shrink in narrow
  // columns instead of overflowing the card.
  const fit = node.fit ?? 'cover'
  const radius =
    typeof node.radius === 'number'
      ? node.radius
      : node.radius
        ? `var(--mantine-radius-${node.radius})`
        : undefined
  const minH = typeof node.height === 'number' ? node.height : undefined
  const w = typeof node.width === 'number' ? node.width : '100%'

  const img = (
    <img
      src={node.src}
      alt={node.alt ?? ''}
      style={{
        display: 'block',
        width: w,
        height: '100%',
        minHeight: minH,
        objectFit: fit as 'cover' | 'contain' | 'fill' | 'none' | 'scale-down',
        borderRadius: radius,
      }}
    />
  )

  return (
    <Stack {...dataAttrs(node)} gap={4} style={{ flex: 1, minHeight: 0 }}>
      <div style={{ flex: 1, minHeight: 0, minWidth: 0, display: 'flex' }}>
        {img}
      </div>
      {node.caption && (
        <Text size="xs" c="dimmed" ta="center">{node.caption}</Text>
      )}
    </Stack>
  )
}

// =================================================================================================
// kpi — headline metric card
// =================================================================================================

export function KPIRenderer({ node, render }: { node: UIKPINode; render?: (n: UIPageNode, key?: string | number) => ReactNode }) {
  const run = useRunAction()(node.action)
  const item = useItem()
  // Its children sit in the tile, under the caption (a button of their own:
  // a tile with children is not itself clickable).
  const footer = render && node.children?.length ? node.children.map((c, i) => render(c, c.id ?? i)) : undefined
  if (footer) return <KPICard node={node} attrs={dataAttrs(node)} footer={footer} />
  const card = <KPICard node={node} attrs={run ? undefined : dataAttrs(node)} />
  if (!run) return card
  return (
    <UnstyledButton {...dataAttrs(node)} onClick={() => run(item)} aria-label={node.action?.label ?? node.label} {...kpiPressed(node)} style={{ display: 'block', width: '100%' }}>
      {card}
    </UnstyledButton>
  )
}

/** A clickable tile's state for assistive tech: the chosen one (a filter's,
 *  `selected`) is the current one. */
export function kpiPressed(node: Pick<UIKPINode, 'selected'>): { 'aria-current'?: 'true' } {
  return node.selected ? { 'aria-current': 'true' } : {}
}

function KPICard({ node, attrs, footer }: { node: UIKPINode; attrs?: Record<string, string>; footer?: ReactNode }) {
  return (
    <KpiTile label={node.label} value={node.value} description={node.description} color={node.color}
      icon={node.icon} trend={node.trend} trendColor={node.trendColor} selected={node.selected} attrs={attrs} footer={footer} variant={node.variant} />
  )
}

// =================================================================================================
// code-block — syntax-highlighted code with copy button
// =================================================================================================

export function CodeBlockRenderer({ node }: { node: UICodeBlockNode }) {
  const showCopy = node.withCopyButton !== false
  return (
    <Box {...dataAttrs(node)} style={{ position: 'relative' }}>
      <Code block style={{ fontSize: 12, padding: 12, paddingTop: showCopy ? 32 : 12 }}>
        {node.content}
      </Code>
      {showCopy && (
        <Box style={{ position: 'absolute', top: 4, right: 4 }}>
          <CopyButton value={node.content} timeout={1500}>
            {({ copied, copy }) => (
              <Tooltip label={copied ? 'Copied' : 'Copy'} withArrow>
                <button
                  type="button"
                  onClick={copy}
                  style={{
                    background: 'transparent',
                    border: 'none',
                    cursor: 'pointer',
                    padding: 4,
                    display: 'flex',
                  }}
                  aria-label="copy"
                >
                  {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
                </button>
              </Tooltip>
            )}
          </CopyButton>
        </Box>
      )}
      {node.language && (
        <Box
          style={{
            position: 'absolute',
            top: 4,
            left: 8,
            fontSize: 10,
            color: 'var(--mantine-color-dimmed)',
            textTransform: 'uppercase',
            letterSpacing: 0.5,
          }}
        >
          {node.language}
        </Box>
      )}
    </Box>
  )
}

// =================================================================================================
// markdown — GFM rendered inline
// =================================================================================================

/** markdownUrl keeps an image inlined as a data:image URI - a fetched
 *  page's pictures, drawn without a file - which the
 *  default transform empties; every other address goes through it. */
export function markdownUrl(url: string, key: string): string {
  return key === 'src' && /^data:image\//i.test(url) ? url : defaultUrlTransform(url)
}

export function MarkdownRenderer({ node }: { node: UIMarkdownNode }) {
  const body = (
    <Box {...(node.maxHeight && node.maxHeight > 0 ? {} : dataAttrs(node))} className="ui-page-markdown">
      <ReactMarkdown remarkPlugins={[remarkGfm]} urlTransform={markdownUrl}>{node.content}</ReactMarkdown>
    </Box>
  )
  // Collapsible: up to maxHeight, then a toggle opens the rest.
  if (!node.maxHeight || node.maxHeight <= 0) return body
  return (
    <Spoiler {...dataAttrs(node)} maxHeight={node.maxHeight} showLabel="Show more" hideLabel="Show less">
      {body}
    </Spoiler>
  )
}

// =================================================================================================
// video — native <video> element wrapped in Mantine chrome
// =================================================================================================

function mimeFromSrc(src: string): string | undefined {
  const ext = src.split('?')[0]?.split('.').pop()?.toLowerCase()
  switch (ext) {
    case 'mp4':  return 'video/mp4'
    case 'webm': return 'video/webm'
    case 'ogg':
    case 'ogv':  return 'video/ogg'
    case 'mov':  return 'video/quicktime'
    case 'mp3':  return 'audio/mpeg'
    case 'wav':  return 'audio/wav'
    case 'm4a':
    case 'aac':  return 'audio/aac'
    case 'flac': return 'audio/flac'
    default:     return undefined
  }
}

export function VideoRenderer({ node }: { node: UIVideoNode }) {
  const mime = node.mime ?? mimeFromSrc(node.src)
  // autoplay implies muted unless the caller explicitly opted out — browsers
  // block autoplay-with-sound by default and silently fail to start.
  const muted = node.muted ?? (node.autoplay ? true : false)
  const radius =
    typeof node.radius === 'number'
      ? node.radius
      : node.radius
        ? `var(--mantine-radius-${node.radius})`
        : undefined

  // Don't set both src= AND <source> — the source child is the
  // canonical mechanism when a MIME type is known (lets the browser
  // refuse early on unsupported codecs instead of silent black-screen).
  // Bug surfaced 2026-05-18: WebView2 silently failed on BigBuckBunny
  // because src+source duplicated the URL and conflicted with the
  // preload heuristic.
  const player = (
    <video
      poster={node.poster}
      controls={node.controls ?? true}
      autoPlay={node.autoplay ?? false}
      loop={node.loop ?? false}
      muted={muted}
      preload="metadata"
      playsInline
      style={{
        display: 'block',
        width: node.width ?? '100%',
        height: node.height ?? '100%',
        minHeight: typeof node.height === 'number' ? node.height : undefined,
        objectFit: 'contain',
        borderRadius: radius,
        background: '#000',
      }}
      onError={(e) => {
        // Surface load errors in devtools — black screens are otherwise
        // indistinguishable from "video still loading".
        const v = e.currentTarget as HTMLVideoElement
        // eslint-disable-next-line no-console
        console.warn('[video] error code:', v.error?.code, 'src:', node.src)
      }}
    >
      <source src={node.src} type={mime ?? 'video/mp4'} />
      <Text size="xs" c="dimmed">
        Your browser doesn&apos;t support inline video. <Anchor href={node.src}>Download instead.</Anchor>
      </Text>
    </video>
  )

  const wrapped = node.caption ? (
    <Stack {...dataAttrs(node)} gap={4} style={{ flex: 1, minHeight: 0 }}>
      <div style={{ flex: 1, minHeight: 0, display: 'flex' }}>{player}</div>
      <Text size="xs" c="dimmed" ta="center">{node.caption}</Text>
    </Stack>
  ) : (
    <div {...dataAttrs(node)} style={{ flex: 1, minHeight: 0, display: 'flex' }}>{player}</div>
  )

  return wrapped
}

// =================================================================================================
// audio — native <audio> element with optional label
// =================================================================================================

export function AudioRenderer({ node }: { node: UIAudioNode }) {
  const mime = node.mime ?? mimeFromSrc(node.src)
  const muted = node.muted ?? (node.autoplay ? true : false)
  const player = (
    <audio
      {...(node.label ? {} : dataAttrs(node))}
      src={node.src}
      controls={node.controls ?? true}
      autoPlay={node.autoplay ?? false}
      loop={node.loop ?? false}
      muted={muted}
      style={{ display: 'block', width: '100%' }}
    >
      {mime ? <source src={node.src} type={mime} /> : null}
      <Text size="xs" c="dimmed">
        Your browser doesn&apos;t support inline audio. <Anchor href={node.src}>Download instead.</Anchor>
      </Text>
    </audio>
  )
  if (!node.label) return player
  return (
    <Stack {...dataAttrs(node)} gap={4}>
      <Text size="xs" c="dimmed">{node.label}</Text>
      {player}
    </Stack>
  )
}

// =================================================================================================
// file — downloadable file card
// =================================================================================================

function formatBytes(n: number | undefined): string | undefined {
  if (n === undefined || n < 0 || !Number.isFinite(n)) return undefined
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(1)} MB`
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

export function FileRenderer({ node }: { node: UIFileNode }) {
  const sizeLabel = formatBytes(node.sizeBytes)
  const downloadName = node.downloadAs ?? node.name
  return (
    <Card {...dataAttrs(node)} withBorder padding="sm" radius="sm">
      <Group gap="sm" wrap="nowrap" align="flex-start">
        <ActionIcon variant="light" size="lg" radius="sm" color="gray" aria-hidden>
          <IconFile size={18} />
        </ActionIcon>
        <Stack gap={2} style={{ flex: 1, minWidth: 0 }}>
          <Text size="sm" fw={500} truncate>{node.name}</Text>
          <Group gap={6} wrap="nowrap">
            {sizeLabel && (
              <Badge size="xs" variant="default" radius="sm">{sizeLabel}</Badge>
            )}
            {node.mime && (
              <Badge size="xs" variant="default" radius="sm" color="gray">{node.mime}</Badge>
            )}
          </Group>
          {node.description && (
            <Text size="xs" c="dimmed" mt={2}>{node.description}</Text>
          )}
        </Stack>
        <Button
          component="a"
          href={node.src}
          download={downloadName}
          size="xs"
          variant="light"
          leftSection={<IconDownload size={12} />}
        >
          Download
        </Button>
      </Group>
    </Card>
  )
}

// =================================================================================================
// timeline — vertical event timeline (Mantine Timeline)
// =================================================================================================

/** TimelineDetails folds an item's longer text under a Details… switch. */
function TimelineDetails({ text }: { text: string }) {
  const [open, setOpen] = useState(false)
  return (
    <Box mt={2}>
      <Anchor component="button" type="button" size="xs" onClick={() => setOpen((o) => !o)} aria-expanded={open}>
        {open ? 'Hide details' : 'Details…'}
      </Anchor>
      <Collapse in={open}>
        <Box className="ui-page-markdown" mt={4} p="xs" style={{ borderLeft: '2px solid var(--mantine-color-default-border)' }}>
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{text}</ReactMarkdown>
        </Box>
      </Collapse>
    </Box>
  )
}

export function TimelineRenderer({ node }: { node: UITimelineNode }) {
  // `items` can be absent on the first (partial / streaming) render — guard it
  // so the whole widget doesn't crash before the array arrives.
  const items = node.items ?? []
  const active = node.activeIndex ?? items.length
  return (
    <Timeline
      {...dataAttrs(node)}
      active={active - 1}
      bulletSize={20}
      lineWidth={2}
      color={node.color ?? 'blue'}
    >
      {items.map((item, i) => (
        <Timeline.Item
          key={i}
          title={
            <Group gap={6} wrap="nowrap" align="baseline">
              <Text fw={600} size="sm">{item.title}</Text>
              <Text size="xs" c="dimmed">{item.date}</Text>
            </Group>
          }
          color={item.color}
        >
          {item.body && (
            <Box className="ui-page-markdown" mt={2}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>{item.body}</ReactMarkdown>
            </Box>
          )}
          {item.details && <TimelineDetails text={item.details} />}
        </Timeline.Item>
      ))}
    </Timeline>
  )
}
