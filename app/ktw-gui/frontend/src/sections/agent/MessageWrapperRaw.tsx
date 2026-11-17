// SPDX-License-Identifier: Apache-2.0
/**
 * MessageWrapperRaw — naked message display
 * ─────────────────────────────────────────────────────────────────────────
 * No box, no border, no title bar. Just renders children directly.
 * Used for plain text messages (markdown).
 */
import type { ReactNode } from 'react'

interface Props {
  children: ReactNode
}

export function MessageWrapperRaw({ children }: Props) {
  return <>{children}</>
}
