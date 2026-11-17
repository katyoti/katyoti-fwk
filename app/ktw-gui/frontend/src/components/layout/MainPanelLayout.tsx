// SPDX-License-Identifier: Apache-2.0
import type { ReactNode } from 'react'

/**
 * MainPanelLayout — the canonical 3-zone layout for any full-pane view.
 *
 * Title and footer are fixed; the content zone scrolls independently.
 * Fills 100% of the parent (which itself fills the right pane), so the
 * scrollbar lives inside the content zone instead of the page.
 *
 * Usage:
 *
 *   <MainPanelLayout
 *     title={<Title order={3}>Edit universe</Title>}
 *     footer={<Group justify="flex-end"><Button>Save</Button></Group>}
 *   >
 *     {scrollable form / content}
 *   </MainPanelLayout>
 */
interface MainPanelLayoutProps {
  title:    ReactNode
  children: ReactNode
  footer?:  ReactNode
}

export function MainPanelLayout({ title, children, footer }: MainPanelLayoutProps) {
  return (
    <div
      style={{
        display:       'flex',
        flexDirection: 'column',
        height:        '100%',
        minHeight:     0,
      }}
    >
      <div
        style={{
          flex:          '0 0 auto',
          padding:       '16px 24px',
          borderBottom:  '1px solid var(--mantine-color-default-border)',
        }}
      >
        {title}
      </div>

      <div
        style={{
          flex:          '1 1 auto',
          minHeight:     0,
          display:       'flex',
          flexDirection: 'column',
          padding:       '16px 24px',
        }}
      >
        {children}
      </div>

      {footer && (
        <div
          style={{
            flex:        '0 0 auto',
            padding:     '12px 24px',
            borderTop:   '1px solid var(--mantine-color-default-border)',
          }}
        >
          {footer}
        </div>
      )}
    </div>
  )
}
