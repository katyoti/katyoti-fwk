// SPDX-License-Identifier: Apache-2.0
// Mermaid — one diagram, drawn by the mermaid library loaded on first use (its
// own chunk). Used by the markdown setup for ```mermaid fenced blocks and by the
// text/vnd.mermaid engine. It draws a source once it has stopped changing (a
// streaming document), and a diagram that does not parse shows its source and
// the error instead of breaking the page.
import { useEffect, useId, useState } from 'react'
import { Box, Code, Loader, Text, useComputedColorScheme } from '@mantine/core'
import { useDebouncedValue } from '@mantine/hooks'
import { useT } from '../../i18n'

type MermaidAPI = typeof import('mermaid').default
let loading: Promise<MermaidAPI> | null = null
let initialisedTheme = ''

function loadMermaid(theme: 'dark' | 'default'): Promise<MermaidAPI> {
  loading ??= import('mermaid').then((m) => m.default)
  return loading.then((mermaid) => {
    if (initialisedTheme !== theme) {
      mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', theme })
      initialisedTheme = theme
    }
    return mermaid
  })
}

let exportCount = 0

/** The SVG of a mermaid diagram, rendered off-screen (light theme, for a file). */
export async function renderMermaidSvg(code: string): Promise<string> {
  const mermaid = await loadMermaid('default')
  const { svg } = await mermaid.render(`mmd-export-${++exportCount}`, code.trim())
  return svg
}

export function Mermaid({ code }: { code: string }) {
  const t = useT('components')
  const scheme = useComputedColorScheme('dark')
  const id = `mmd-${useId().replace(/[^a-zA-Z0-9]/g, '')}`
  const [state, setState] = useState<{ svg?: string; error?: string }>({})
  // A diagram still being written (a streaming document) does not parse:
  // draw only a source that has stopped changing for a moment, so a
  // half-written block never flashes an error.
  const [stable] = useDebouncedValue(code, 500)
  const settled = stable === code

  useEffect(() => {
    if (!settled) return
    let cancelled = false
    const source = stable.trim()
    if (!source) { setState({}); return }
    loadMermaid(scheme === 'dark' ? 'dark' : 'default')
      .then((mermaid) => mermaid.render(id, source))
      .then(({ svg }) => { if (!cancelled) setState({ svg }) })
      .catch((e: unknown) => {
        // mermaid leaves its error graphic in the body on a failed render.
        document.getElementById(`d${id}`)?.remove()
        if (!cancelled) setState({ error: e instanceof Error ? e.message : String(e) })
      })
    return () => { cancelled = true }
  }, [stable, settled, id, scheme])

  if (!settled && !state.svg) {
    return <Text size="xs" c="dimmed" my={8}>{t('mermaid.pending')}</Text>
  }
  if (state.error && settled) {
    return (
      <Box my={8}>
        <Text size="xs" c="red">{t('mermaid.error', { error: state.error })}</Text>
        <Code block style={{ fontSize: 12 }}>{code}</Code>
      </Box>
    )
  }
  if (!state.svg) return <Loader size="xs" my={8} />
  return <Box my={8} style={{ overflowX: 'auto', opacity: settled ? 1 : 0.6 }} dangerouslySetInnerHTML={{ __html: state.svg }} />
}
