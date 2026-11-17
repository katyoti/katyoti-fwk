// SPDX-License-Identifier: Apache-2.0
// Renderer of the page DSL's `item-detail` node: one item a
// source brings, opened through its source's fetch - its card
// at the summary size (the item's roles), its link, its data
// drawn by the view of the data-type its connector declares for its kind (the
// object pane resolving that view, as it resolves a declared panel's), its
// markdown text with its images, and the objects it is used by. Generic: the
// item is data, the fetch and the resolve are method URIs. Drawn where the
// host shows the source model (IN-045's switch); elsewhere nothing.
// What it draws is itemDetailParts', tested apart from rendering.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, useState, type ReactNode } from 'react'
import { Alert, Anchor, Group, Loader, Stack, Table, Text, Title } from '@mantine/core'

import { useHostServices, useSourceModel } from '../../../ui-core/host/index.tsx'
import { enclosingDrafts, usePageContext } from '../page-context.tsx'
import {
  ITEM_DETAIL_LABELS, ITEM_DETAIL_RESOLVE, itemDetailParts, resolveActionArgs,
  type UIItemDetailNode, type UIItemOpened,
} from '../ui-page.interactive.ts'
import type { UIPageNode } from '../ui-page.types.ts'
import type { RenderNode } from './layout.tsx'
import { SourceItem } from './source-item.tsx'

export type ItemDetailLabels = typeof ITEM_DETAIL_LABELS

/** Where opening an item stands: the item it answered (laid over the bound
 *  one), the data-type's view once resolved, or why it failed. */
interface Opening {
  at?: string
  opened?: UIItemOpened
  failed?: string
  view?: Record<string, unknown>
  viewFailed?: string
}

/** itemFetchArgs are the args a node's fetch is called with: its `${…}`
 *  read in the page's data, then in the pages around it, as an action's. */
export function itemFetchArgs(node: Pick<UIItemDetailNode, 'fetch'>, draft: unknown, around: unknown[] = []): Record<string, unknown> | undefined {
  if (!node.fetch) return undefined
  const scope = draft && typeof draft === 'object' && !Array.isArray(draft) ? draft as Record<string, unknown> : {}
  return resolveActionArgs(node.fetch.args ?? {}, undefined, scope, around)
}

export function ItemDetailRenderer({ node, render }: { node: UIItemDetailNode; render: RenderNode }) {
  if (!useSourceModel()) return null
  return <ItemDetailLoad node={node} render={render} />
}

/** ItemDetailLoad opens the item through `fetch` when the node names one,
 *  then resolves its data-type's view. */
function ItemDetailLoad({ node, render }: { node: UIItemDetailNode; render: RenderNode }) {
  const { callMethod } = useHostServices()
  const page = usePageContext()
  const args = itemFetchArgs(node, page.draft, enclosingDrafts(page))
  const fetchAt = node.fetch ? JSON.stringify([node.fetch.method, args]) : undefined
  const [st, setSt] = useState<Opening>({})

  // The item: fetched once per (method, args), the producer's `used` kept.
  useEffect(() => {
    if (!node.fetch || !fetchAt) return
    if (!callMethod) { setSt({ at: fetchAt, failed: 'no host to open the item' }); return }
    let live = true
    setSt({ at: fetchAt })
    callMethod(node.fetch.method, args).then(
      (got) => { if (live) setSt({ at: fetchAt, opened: (got ?? {}) as UIItemOpened }) },
      (err: unknown) => { if (live) setSt({ at: fetchAt, failed: err instanceof Error ? err.message : String(err) }) },
    )
    return () => { live = false }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fetchAt, callMethod])

  const fetched = node.fetch ? (st.at === fetchAt ? st.opened : undefined) : undefined
  const opened: UIItemOpened | undefined = node.fetch
    ? (fetched ? { ...fetched, used: fetched.used ?? node.data?.used } : undefined)
    : node.data
  const dataType = opened?.data && Object.keys(opened.data).length > 0 ? opened.data_type : undefined

  // The data-type's view, resolved by the host's resolve-part.
  const [view, setView] = useState<{ type?: string; schema?: Record<string, unknown>; failed?: string }>({})
  useEffect(() => {
    if (!dataType || !callMethod) return
    let live = true
    callMethod(node.resolve || ITEM_DETAIL_RESOLVE, { kind: 'view', schema: dataType }).then(
      (got) => {
        const part = (got as { part?: unknown } | undefined)?.part
        if (live) setView(part && typeof part === 'object' ? { type: dataType, schema: part as Record<string, unknown> } : { type: dataType, failed: 'no view' })
      },
      (err: unknown) => { if (live) setView({ type: dataType, failed: err instanceof Error ? err.message : String(err) }) },
    )
    return () => { live = false }
  }, [dataType, callMethod, node.resolve])

  const labels = { ...ITEM_DETAIL_LABELS, ...(node.labels ?? {}) } as ItemDetailLabels
  if (node.fetch && st.at === fetchAt && st.failed) {
    return <Alert {...dataAttrs(node)} color="red" title={labels.failed} data-guide="item-detail" data-state="failed">{st.failed}</Alert>
  }
  if (!opened) {
    return (
      <Group {...dataAttrs(node)} gap="xs" data-guide="item-detail" data-state="loading">
        <Loader size="xs" /><Text size="sm" c="dimmed">{labels.loading}</Text>
      </Group>
    )
  }
  return <ItemDetailView node={node} opened={opened} labels={labels} render={render}
    view={view.type === dataType ? view.schema : undefined} viewFailed={view.type === dataType ? view.failed : undefined} />
}

/** ItemDetailView draws an opened item: its card, its link, its data by the
 *  data-type's `view` when resolved (field by field until then or without
 *  one), its text, where it is used. */
export function ItemDetailView({ node, opened, labels = ITEM_DETAIL_LABELS, render, view, viewFailed }: {
  node: Pick<UIItemDetailNode, 'id' | 'kinds'> & Partial<UIItemDetailNode>
  opened: UIItemOpened
  labels?: ItemDetailLabels
  render: RenderNode
  view?: Record<string, unknown>
  viewFailed?: string
}): ReactNode {
  const { navigate } = useHostServices()
  const p = itemDetailParts(opened)
  return (
    <Stack {...dataAttrs(node as UIPageNode)} gap="md" data-guide="item-detail" data-state="ready" data-kind={p.item?.kind}>
      {p.item && <SourceItem item={p.item} size="summary" kinds={node.kinds} guide="item-detail" />}
      {p.href && <Anchor href={p.href} target="_blank" rel="noreferrer" size="sm" data-guide="item-detail.link">{labels.link}</Anchor>}
      {p.uri && <Text size="xs" c="dimmed" data-guide="item-detail.uri">{p.uri}</Text>}
      {p.data && (
        <Stack gap={4} data-guide="item-detail.data" data-type={p.dataType}>
          <Title order={6}>{labels.data}</Title>
          {view
            ? render({ kind: 'view', schema: view, data: p.data } as unknown as UIPageNode, 'data')
            : (
              <Table withRowBorders={false} verticalSpacing={2} data-guide="item-detail.fields" title={viewFailed}>
                <Table.Tbody>
                  {p.dataRows.map((r) => (
                    <Table.Tr key={r.field}>
                      <Table.Td><Text size="xs" c="dimmed">{r.field}</Text></Table.Td>
                      <Table.Td><Text size="sm">{r.value}</Text></Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            )}
        </Stack>
      )}
      {p.text && (
        <Stack gap={4} data-guide="item-detail.text">
          {p.data && <Title order={6}>{labels.text}</Title>}
          {render({ kind: 'markdown', content: p.text } as UIPageNode, 'text')}
        </Stack>
      )}
      {p.empty && <Text size="sm" c="dimmed" fs="italic" data-guide="item-detail.empty">{labels.empty}</Text>}
      {p.used && (
        <Stack gap={2} data-guide="item-detail.used">
          <Title order={6}>{labels.used}</Title>
          {p.used.length === 0
            ? <Text size="sm" c="dimmed">{labels.unused}</Text>
            : p.used.map((u) => (
              <Anchor key={u.id} component="button" type="button" size="sm" ta="left" data-guide="item-detail.use" data-guide-name={u.id}
                onClick={() => navigate?.({ object: u.id })}>{u.name || u.id}</Anchor>
            ))}
        </Stack>
      )}
    </Stack>
  )
}
