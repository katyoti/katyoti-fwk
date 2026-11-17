// SPDX-License-Identifier: Apache-2.0
/**
 * VerticalTabBody — shared shell for tabs that have a left vertical
 * "sub-tab" strip + a right content panel.
 *
 * Used by both the Global tab (chat / tasks / quick-actions / ...)
 * and the Node tab (chat-on-demand / tasks / one entry per allowed
 * item kind). Pattern decided 2026-06-06 — keeps the two surfaces
 * structurally symmetric so users learn one navigation idiom and
 * apply it everywhere.
 *
 * The strip is purely presentational: parents supply entries, the
 * current active id, and an onChange handler. Content rendering
 * happens inside the parent — the shell only handles layout +
 * selection state + the visual treatment of strip rows.
 */

import { Badge, Box, Group, Stack, Text } from '@mantine/core'
import type { ReactNode } from 'react'
import { activateOnKey } from '../../guide/anchors'
import { useT } from '../../i18n'

export interface StripEntry {
  /** Stable id for selection state. */
  id: string
  /** Display label shown in the strip. */
  label: string
  /** Optional icon component (Tabler icon or any FC). */
  icon?: ReactNode
  /** Optional badge text shown in dim color next to the label
   *  (e.g. item count: "12"). */
  badge?: string | number
  /** The badge's tone (a Mantine colour): drawn as a badge in it, else dim
   *  text. */
  badgeColor?: string
  /** Optional Mantine color name for the icon's accent. */
  accent?: string
  /** Optional alert tag shown in red after the label (e.g. "error"). */
  alert?: string
  /** Optional tooltip — an object panel's `description`. */
  description?: string
  /** When true, the row gets a "soft" placeholder treatment (e.g. for
   *  empty / coming-soon sections). */
  muted?: boolean
}

interface VerticalTabBodyProps {
  /** Header rendered above the strip + content (full-width). Typically
   *  the node identity card on a Node tab, or a global title on the
   *  Global tab. Pass `null` to skip. */
  header: ReactNode
  /** Strip entries (left column). */
  entries: StripEntry[]
  /** Currently selected entry id. Empty string falls through to the
   *  first entry. */
  activeId: string
  /** Selection callback. */
  onChange: (id: string) => void
  /** Content for the currently selected entry. Parent dispatches on
   *  activeId (the body shell knows nothing about content shapes). */
  children: ReactNode
  /** Optional element pinned at the BOTTOM of the left strip (below the
   *  entries), e.g. a "Start chat" button. Kept out of the scrolling entry
   *  list so it stays visible. */
  footer?: ReactNode
}

/** Fixed strip width — matches the left tree pane visually. */
const STRIP_WIDTH = 180

export function VerticalTabBody({
  header, entries, activeId, onChange, children, footer,
}: VerticalTabBodyProps) {
  const effectiveActive = activeId || entries[0]?.id || ''
  const t = useT('workspace')

  return (
    // Plain flex divs (not Mantine Stack/Group) for the structural shell:
    // Mantine Group defaults to `flex-wrap: wrap`, which collapses the
    // `align-items: stretch` cross-size so the content panel never gets a
    // definite height — and the inner ScrollArea then grows to content
    // instead of scrolling. Explicit flexbox keeps the height chain intact.
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
      {header && (
        <Box style={{ flexShrink: 0 }}>{header}</Box>
      )}
      <div style={{ display: 'flex', flexDirection: 'row', alignItems: 'stretch', flex: 1, minHeight: 0 }}>
        {/* Left vertical strip — a flex column so the entry list scrolls
            while the optional footer (e.g. Start chat) stays pinned. */}
        <Box
          style={{
            width: STRIP_WIDTH,
            flexShrink: 0,
            borderRight: '1px solid var(--mantine-color-default-border)',
            display: 'flex',
            flexDirection: 'column',
            minHeight: 0,
          }}
        >
          <Stack gap={2} p={6} role="tablist" aria-orientation="vertical" style={{ flex: 1, minHeight: 0, overflowY: 'auto' }}>
            {entries.length === 0 && (
              <Text size="xs" c="dimmed" ta="center" py="md">
                {t('strip.noSections')}
              </Text>
            )}
            {entries.map((e) => {
              const isActive = e.id === effectiveActive
              return (
                <Box
                  key={e.id}
                  title={e.description}
                  onClick={() => !e.muted && onChange(e.id)}
                  role="tab"
                  aria-selected={isActive}
                  aria-disabled={e.muted || undefined}
                  aria-label={e.label}
                  tabIndex={e.muted ? -1 : 0}
                  onKeyDown={activateOnKey(() => { if (!e.muted) onChange(e.id) })}
                  data-guide={'objtab.' + e.id}
                  data-guide-label={e.label}
                  style={{
                    padding: '6px 10px',
                    borderRadius: 4,
                    cursor: e.muted ? 'default' : 'pointer',
                    backgroundColor: isActive
                      ? 'var(--mantine-color-default-hover)'
                      : 'transparent',
                    opacity: e.muted ? 0.5 : 1,
                  }}
                >
                  <Group gap="xs" wrap="nowrap" align="center">
                    {e.icon && (
                      <Box
                        style={{
                          color: e.accent
                            ? `var(--mantine-color-${e.accent}-6)`
                            : 'var(--mantine-color-dimmed)',
                          display: 'flex',
                          alignItems: 'center',
                        }}
                      >
                        {e.icon}
                      </Box>
                    )}
                    <Text
                      size="sm"
                      fw={isActive ? 600 : 400}
                      style={{ flex: 1, minWidth: 0 }}
                      truncate
                    >
                      {e.label}
                    </Text>
                    {e.badge !== undefined && e.badge !== '' && (e.badgeColor ? (
                      <Badge size="xs" color={e.badgeColor} variant="filled" style={{ textTransform: 'none', flexShrink: 0 }} data-guide="objtab.badge">
                        {e.badge}
                      </Badge>
                    ) : (
                      <Text size="xs" c="dimmed">
                        {e.badge}
                      </Text>
                    ))}
                    {e.alert && (
                      <Badge size="xs" color="red" variant="filled" style={{ textTransform: 'none', flexShrink: 0 }}>
                        {e.alert}
                      </Badge>
                    )}
                  </Group>
                </Box>
              )
            })}
          </Stack>
          {footer && (
            <Box style={{ flexShrink: 0, borderTop: '1px solid var(--mantine-color-default-border)', padding: 6 }}>
              {footer}
            </Box>
          )}
        </Box>

        {/* Right content panel — a bounded flex column so children that
            fill height (e.g. SearchableCardGrid's scrollable grid) get a
            definite height to resolve against. Overflow is hidden here;
            the active content owns its own scroll region. */}
        <Box
          style={{
            flex: 1,
            minWidth: 0,
            minHeight: 0,
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
          }}
        >
          {children}
        </Box>
      </div>
    </div>
  )
}
