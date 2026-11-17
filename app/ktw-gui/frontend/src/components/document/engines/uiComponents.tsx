// SPDX-License-Identifier: Apache-2.0
// ui-components engine — application/vnd.ktw.ui+json. The content is a
// component object naming its own type: form, view, table, tree-table, wizard
// and page render in the ktw-display iframe (through UIComponent); extension
// mounts a named GUI view with the host's object context. A content with no
// type is a page tree (a `.page.json` file). Its schema is inline here: a
// schema named by id, or a ui block, was resolved in Go (DocumentOpen) first.
import { Box, Text } from '@mantine/core'
import { UIComponent, type UIComponentDef } from '../../compositions'
import { MEDIA_TYPE_JSON, MEDIA_TYPE_UI } from '../../../generated/media-types'
import type { DisplayEngine, EngineProps } from '../engines'
import { extensionView } from '../extensionViews'
import { useT } from '../../../i18n'

type Component = Record<string, unknown> & { kind?: string }

function componentOf(content: unknown): Component | null {
  let c = content
  if (typeof c === 'string') {
    try { c = JSON.parse(c) } catch { return null }
  }
  if (!c || typeof c !== 'object' || Array.isArray(c)) return null
  const obj = c as Component
  return typeof obj.kind === 'string' && obj.kind ? obj : { kind: 'page', dslTree: obj }
}

function UIComponentsView({ doc, host }: EngineProps) {
  const t = useT('components')
  const c = componentOf(doc.content)
  if (!c) {
    return <Box p="md"><Text size="xs" c="red">{t('uiComponents.noComponent')}</Text></Box>
  }
  if (c.kind === 'extension') {
    const view = extensionView(c.extension as string | undefined)
    if (!view) {
      return <Box p="md"><Text size="xs" c="red">{t('uiComponents.unknownExtension', { name: String(c.extension) })}</Text></Box>
    }
    const ctx = host.context ?? { universeName: host.universe ?? '', objectId: '', objectName: '' }
    return <>{view({ ...ctx, input: c.data })}</>
  }
  // A cross-turn patch lands as a new document: push the live value into the
  // already-mounted iframe instead of re-encoding its URL.
  const pushValue = c.kind === 'page' ? c.dslTree : c.data
  return (
    <UIComponent
      def={c as unknown as UIComponentDef}
      universe={host.universe}
      pushValue={pushValue}
      fillHeight={host.fillHeight ?? true}
      onSubmit={host.onSubmit}
      onAction={host.onAction}
      streamingComplete={host.streamingComplete}
    />
  )
}

export const uiComponentsEngine: DisplayEngine = {
  id: 'ui-components',
  label: 'Component',
  accepts: [MEDIA_TYPE_UI],
  capabilities: {
    view: true, stream: true,
    export: [{ mediaType: MEDIA_TYPE_JSON, label: 'JSON (the component)' }],
  },
  // The component object itself — schema, variant, data — as a file.
  exportDocument: async (doc) => ({
    mediaType: MEDIA_TYPE_JSON,
    title: (doc.title ?? 'component').replace(/[^\w.-]+/g, '-') + '.json',
    content: typeof doc.content === 'string' ? doc.content : JSON.stringify(doc.content, null, 2),
  }),
  builtin: true,
  component: UIComponentsView,
}
