// SPDX-License-Identifier: Apache-2.0
/**
 * Markdown — shared GitHub-flavored markdown renderer.
 *
 * Mantine-styled element overrides (code, tables, headings, lists,
 * blockquotes) so rendered markdown matches the app's density and theme
 * tokens; ```mermaid fenced blocks are drawn as diagrams and ```guide-recipe
 * blocks as a card that starts the recipe (modules/guide). Given the file the
 * text was read from (`media`), a relative image or diagram is drawn in place
 * and a relative link opens its target. The one markdown
 * setup of the GUI (the markdown display engine uses it too). Each heading
 * carries the id of its line (`document/toc`); with `toc`, a text of three
 * headings or more reads beside its table of contents. Used by the chat text renderer and any prose surface (node
 * descriptions, item summaries, …). Callers pass already-normalized text.
 */
import type { ReactNode } from 'react'
import { Badge, Box, Divider, NavLink, Text } from '@mantine/core'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Mermaid } from './document/Mermaid'
import { GuideRecipeCard } from '../guide/GuideRecipeCard'
import { useT } from '../i18n'
import { headingsOf, TOC_MIN_HEADINGS, type Heading } from './document/toc'
import { isPlainImage, isRelativeRef, joinRef, stripFrontmatter, wrapSpacedDestinations } from './document/fileLinks'
import { hasDialect, lozengeColour, splitDialect, statusLozenges, type DialectAdmonition, type DialectSegment } from '@ktw/display-lib/utils/markdown-dialect.ts'

/** The file a markdown text was read from: a relative image is drawn in place
 *  from the file's folder (`embed`, through the display engine by its media
 *  type — a diagram, a picture), a relative link opens its target (`open`). */
export interface MarkdownMedia {
  /** The folder the text's file sits in (module-relative). */
  folder: string
  embed: (path: string, title: string) => ReactNode
  open?: (path: string) => void
}

/** True when a paragraph holds an image: drawn as a block, since an embedded
 *  document is not phrasing content (a status lozenge is: it stays inline). */
function holdsImage(node: unknown): boolean {
  const kids = (node as { children?: Array<{ type?: string; tagName?: string; properties?: { src?: unknown } }> } | undefined)?.children ?? []
  return kids.some((c) => c.type === 'element' && c.tagName === 'img' && !lozengeColour(String(c.properties?.src ?? '')))
}

/** The id of the heading a node is: the id of its line in the text. */
function headingId(node: unknown, ids: Map<number, string>, offset: number): string | undefined {
  const line = (node as { position?: { start?: { line?: number } } } | undefined)?.position?.start?.line
  return line ? ids.get(line + offset) : undefined
}

/** The colour of an admonition's kind (the MkDocs Material kinds). */
const ADMONITION_COLOR: Record<string, string> = {
  note: 'blue', info: 'cyan', todo: 'cyan', abstract: 'cyan', summary: 'cyan', tldr: 'cyan',
  tip: 'teal', hint: 'teal', important: 'teal', success: 'green', check: 'green', done: 'green',
  question: 'lime', help: 'lime', faq: 'lime', warning: 'orange', caution: 'orange', attention: 'orange',
  failure: 'red', fail: 'red', missing: 'red', danger: 'red', error: 'red', bug: 'red',
  example: 'violet', quote: 'gray', cite: 'gray', details: 'gray',
}

/** The Mantine colour of a Confluence status lozenge's colour. */
const LOZENGE_COLOR: Record<string, string> = { green: 'green', yellow: 'yellow', red: 'red', blue: 'blue', purple: 'violet', grey: 'gray' }

/** An admonition of the Markdown (Confluence) dialect: a box of its kind's
 *  colour, collapsible (`???`) or not (`!!!`), its body drawn as Markdown. */
function Admonition({ seg, children }: { seg: DialectAdmonition; children: ReactNode }) {
  const c = ADMONITION_COLOR[seg.kind] ?? 'blue'
  const style = { borderLeft: `4px solid var(--mantine-color-${c}-filled)`, background: `var(--mantine-color-${c}-light)`, borderRadius: 4, margin: '8px 0', padding: '6px 12px' }
  const title = seg.title ? <Text component="span" fw={600} fz={14} c={`var(--mantine-color-${c}-light-color)`}>{seg.title}</Text> : null
  if (!seg.collapsible) {
    return (
      <Box data-guide="markdown.admonition" data-kind={seg.kind} style={style}>
        {title && <Box mb={4}>{title}</Box>}
        {children}
      </Box>
    )
  }
  return (
    <Box component="details" open={seg.open || undefined} data-guide="markdown.admonition" data-kind={seg.kind} data-collapsible="true" style={style}>
      <Box component="summary" data-guide="markdown.admonition.title" style={{ cursor: 'pointer' }}>{title ?? seg.kind}</Box>
      <Box mt={4}>{children}</Box>
    </Box>
  )
}

const HEADING_STYLE = { 1: { fontSize: 18, margin: '12px 0 4px' }, 2: { fontSize: 16, margin: '10px 0 4px' }, 3: { fontSize: 14, margin: '8px 0 4px' }, 4: { fontSize: 13, margin: '6px 0 4px' } } as const

/** The table of contents beside a long text: a click scrolls to the heading. */
/** `top`: where it sticks - below a sticky header of the scrolling container. */
export function TableOfContents({ headings, top = 0 }: { headings: Heading[]; top?: number }) {
  const first = Math.min(...headings.map((h) => h.level))
  const t = useT('components')
  return (
    <Box component="nav" aria-label={t('documentView.toc')} data-guide="document.toc" w={230}
      style={{ flexShrink: 0, position: 'sticky', top, alignSelf: 'flex-start', maxHeight: `calc(100vh - ${top}px)`, overflowY: 'auto' }}>
      {headings.map((h) => (
        <NavLink key={h.id} href={`#${h.id}`} py={2} pl={8 + (h.level - first) * 12}
          label={<Text size="xs" lineClamp={2}>{h.text}</Text>}
          onClick={(e) => { e.preventDefault(); document.getElementById(h.id)?.scrollIntoView({ behavior: 'smooth', block: 'start' }) }} />
      ))}
    </Box>
  )
}

/** `toc`: beside its table of contents when long; `tocTop`: the height of a
 *  sticky header above the text in its scrolling container. */
export function Markdown({ text, media, toc, tocTop }: { text: string; media?: MarkdownMedia; toc?: boolean; tocTop?: number }) {
  const source = media ? wrapSpacedDestinations(stripFrontmatter(text)) : text
  const headings = headingsOf(source)
  const ids = new Map(headings.map((h) => [h.line, h.id]))
  const showToc = !!toc && headings.length >= TOC_MIN_HEADINGS
  // The Markdown (Confluence) dialect - admonitions, [[TOC]], comments - is
  // read first; a text without it is drawn as one block.
  const draw = (segments: DialectSegment[], offset: number): ReactNode[] => segments.map((seg) => {
    const key = `${seg.type}-${seg.line + offset}`
    if (seg.type === 'text') return markdownBlock(seg.text, seg.line - 1 + offset, key)
    if (seg.type === 'admonition') {
      return <Admonition key={key} seg={seg}>{draw(splitDialect(seg.body), seg.line + offset)}</Admonition>
    }
    if (showToc || headings.length === 0) return null
    return (
      <Box key={key} component="nav" data-guide="markdown.toc" my={8}>
        {headings.map((h) => (
          <NavLink key={h.id} href={`#${h.id}`} py={1} pl={8 + (h.level - Math.min(...headings.map((x) => x.level))) * 12}
            label={<Text size="sm">{h.text}</Text>}
            onClick={(e) => { e.preventDefault(); document.getElementById(h.id)?.scrollIntoView({ behavior: 'smooth', block: 'start' }) }} />
        ))}
      </Box>
    )
  })
  // One run of plain Markdown; `offset`: the lines above it in the text, so
  // each heading gets the id of its line.
  function markdownBlock(md: string, offset: number, key?: string) {
  const heading = (level: 1 | 2 | 3 | 4) => ({ node, children }: { node?: unknown; children?: ReactNode }) => {
    const Tag = `h${level}` as const
    return <Tag id={headingId(node, ids, offset)} style={{ ...HEADING_STYLE[level], fontWeight: 600, scrollMarginTop: 8 }}>{children}</Tag>
  }
  return (
      <ReactMarkdown
        key={key}
        remarkPlugins={[remarkGfm]}
        components={{
          img({ src, alt }) {
            const status = lozengeColour(typeof src === 'string' ? src : undefined)
            if (status) {
              return (
                <Badge component="span" color={LOZENGE_COLOR[status]} variant="light" radius="sm" size="sm"
                  data-guide="markdown.status" data-status={status} style={{ verticalAlign: 'middle' }}>
                  {alt}
                </Badge>
              )
            }
            const target = media && isRelativeRef(src) ? joinRef(media.folder, String(src)) : undefined
            if (!media || !target) return <img src={src} alt={alt} style={{ maxWidth: '100%' }} />
            return (
              <Box component="span" display="block" my={8}
                style={{ height: isPlainImage(target) ? 320 : 420, border: '1px solid var(--mantine-color-default-border)', borderRadius: 6, overflow: 'hidden' }}>
                {media.embed(target, alt || target)}
              </Box>
            )
          },
          a({ href, children }) {
            const target = media?.open && isRelativeRef(href) ? joinRef(media.folder, String(href)) : undefined
            if (!target || !media?.open) return <a href={href}>{children}</a>
            const open = media.open
            return <a href={href} onClick={(e) => { e.preventDefault(); open(target) }}>{children}</a>
          },
          code({ children, className }) {
            const isBlock = !!className?.startsWith('language-')
            if (className === 'language-mermaid') {
              return <Mermaid code={String(children ?? '')} />
            }
            if (className === 'language-guide-recipe') {
              return <GuideRecipeCard block={String(children ?? '')} />
            }
            if (isBlock) {
              return (
                <Box
                  component="pre"
                  style={{
                    background: 'var(--mantine-color-default-hover)',
                    borderRadius: 6,
                    padding: '8px 12px',
                    overflowX: 'auto',
                    fontSize: 12,
                    margin: '8px 0',
                    fontFamily: 'monospace',
                  }}
                >
                  <code>{children}</code>
                </Box>
              )
            }
            return (
              <Box
                component="code"
                style={{
                  background: 'var(--mantine-color-default-hover)',
                  borderRadius: 3,
                  padding: '1px 5px',
                  fontSize: '0.88em',
                  fontFamily: 'monospace',
                }}
              >
                {children}
              </Box>
            )
          },
          table: ({ children }) => (
            <Box component="table" style={{ borderCollapse: 'collapse', width: '100%', margin: '8px 0', fontSize: 13 }}>
              {children}
            </Box>
          ),
          th: ({ children }) => (
            <th style={{ borderBottom: '1px solid var(--mantine-color-default-border)', padding: '4px 10px', textAlign: 'left' }}>
              {children}
            </th>
          ),
          td: ({ children }) => (
            <td style={{ borderBottom: '1px solid var(--mantine-color-default-border)', padding: '4px 10px' }}>
              {children}
            </td>
          ),
          p:  ({ node, children }) => (holdsImage(node)
            ? <div style={{ margin: '4px 0' }}>{children}</div>
            : <p style={{ margin: '4px 0' }}>{children}</p>),
          h1: heading(1),
          h2: heading(2),
          h3: heading(3),
          h4: heading(4),
          ul: ({ children }) => <ul style={{ paddingLeft: 20, margin: '4px 0' }}>{children}</ul>,
          ol: ({ children }) => <ol style={{ paddingLeft: 20, margin: '4px 0' }}>{children}</ol>,
          li: ({ children }) => <li style={{ marginBottom: 2 }}>{children}</li>,
          blockquote: ({ children }) => (
            <Box component="blockquote" style={{ borderLeft: '3px solid var(--mantine-color-blue-7)', paddingLeft: 12, margin: '8px 0', color: 'var(--mantine-color-dimmed)' }}>
              {children}
            </Box>
          ),
          hr: () => <Divider my={8} />,
        }}
      >
        {statusLozenges(md)}
      </ReactMarkdown>
  )
  }
  const body = (
    <Box fz={14} style={{ lineHeight: 1.7, flex: 1, minWidth: 0 }}>
      {hasDialect(source) ? draw(splitDialect(source), 0) : markdownBlock(source, 0)}
    </Box>
  )
  if (!showToc) return body
  return (
    <Box style={{ display: 'flex', gap: 24, alignItems: 'flex-start' }}>
      <TableOfContents headings={headings} top={tocTop} />
      {body}
    </Box>
  )
}
