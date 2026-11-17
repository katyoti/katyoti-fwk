// SPDX-License-Identifier: Apache-2.0
/**
 * XtermView — embedded interactive shell rendered with xterm.js.
 *
 * Owns the bidirectional bridge between the iframe-side xterm Terminal
 * and the Go-side PTY:
 *   - Subscribes to "shell.<id>.data" → terminal.write(data)
 *   - terminal.onData → WriteShellPTY(id, data)
 *   - ResizeObserver on container → fit() → ResizeShellPTY(id, cols, rows)
 *   - "shell.<id>.exit" → marks session as ended
 *
 * Each XtermView creates its own Terminal instance so multiple shells
 * coexist without scrollback bleed. xterm CSS is imported once.
 */

import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { Box } from '@mantine/core'
import { EventsOff, EventsOn } from '../../../wailsjs/runtime/runtime'
import { ResizeShellPTY, WriteShellPTY } from '../../../wailsjs/go/main/App'
import { useTerminalSessionsStore } from '../../store/terminalSessionsSlice'

interface Props {
  id: string
}

export function XtermView({ id }: Props) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const setEnded = useTerminalSessionsStore((s) => s.setEnded)

  useEffect(() => {
    const el = containerRef.current
    if (!el) return

    // Build the terminal. fontFamily matches the rest of the GUI's
    // monospace styling; theme is dark to match Mantine's default.
    const term = new Terminal({
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
      fontSize: 13,
      lineHeight: 1.2,
      cursorBlink: true,
      scrollback: 5000,
      convertEol: true,
      theme: {
        background: '#0e0f12',
        foreground: '#d0d3d6',
        cursor: '#a0a4ab',
        selectionBackground: 'rgba(120, 140, 200, 0.35)',
      },
    })
    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(el)
    termRef.current = term
    fitRef.current = fit

    try {
      fit.fit()
    } catch {
      /* ignore — fit needs the element to have non-zero dimensions */
    }

    // Forward keystrokes → Go PTY.
    const onDataDisposable = term.onData((data) => {
      void WriteShellPTY(id, data).catch((err: unknown) => {
        // eslint-disable-next-line no-console
        console.error('[shell] write failed:', err)
      })
    })

    // Resize the PTY whenever xterm's geometry changes.
    const onResizeDisposable = term.onResize(({ cols, rows }) => {
      void ResizeShellPTY(id, cols, rows).catch(() => {})
    })

    // Subscribe to PTY output.
    EventsOn(`shell.${id}.data`, (evt: { data?: string } | undefined) => {
      if (evt?.data) term.write(evt.data)
    })
    EventsOn(`shell.${id}.exit`, () => {
      term.write('\r\n\x1b[33m[shell exited]\x1b[0m\r\n')
      setEnded(id)
    })

    // Container-resize observer → fit().
    const ro = new ResizeObserver(() => {
      try {
        fit.fit()
      } catch {
        /* ignore */
      }
    })
    ro.observe(el)

    // Push initial size so the PTY matches the visible widget on mount.
    queueMicrotask(() => {
      try {
        fit.fit()
        const { cols, rows } = term
        if (cols > 0 && rows > 0) {
          void ResizeShellPTY(id, cols, rows).catch(() => {})
        }
      } catch {
        /* ignore */
      }
    })

    // Focus the terminal so typing works without an extra click.
    term.focus()

    return () => {
      ro.disconnect()
      onDataDisposable.dispose()
      onResizeDisposable.dispose()
      EventsOff(`shell.${id}.data`)
      EventsOff(`shell.${id}.exit`)
      term.dispose()
      termRef.current = null
      fitRef.current = null
    }
  }, [id, setEnded])

  return (
    <Box
      ref={containerRef}
      style={{
        flex: 1,
        minHeight: 0,
        background: '#0e0f12',
        // xterm wants padding-free contact with the container; padding
        // would offset the cursor calculations. Inner spacing is handled
        // via xterm's own layer.
      }}
    />
  )
}
