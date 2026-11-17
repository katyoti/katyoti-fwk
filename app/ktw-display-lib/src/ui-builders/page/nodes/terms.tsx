// SPDX-License-Identifier: Apache-2.0
// Renderer of the page DSL's `terms` node: the terms a search
// was run with, each an editable chip - « × » removes it, the box beside adds
// one on Enter - and « Chercher encore » running the node's action with
// `${terms}` the terms shown, the one being typed included. Where it stands is
// termsReduce's: a run waits until the producer sends other
// terms, or its timeout; terms the producer sends replace the chips, an edit
// not run that they replace is said.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, useReducer, useState, type ReactNode } from 'react'
import { ActionIcon, Badge, Button, Group, Stack, Text, TextInput, Tooltip } from '@mantine/core'
import { IconX } from '@tabler/icons-react'

import { pageOwn, useItem, useRunAction } from '../page-context.tsx'
import { addTerm, removeTerm, termsEvent, termsInit, termsReduce, termsToRun, type UITermsNode } from '../ui-page.interactive.ts'

/** The words the node shows, the node's own when it gives them. */
export const TERMS_LABELS = {
  title: 'Recherche faite avec',
  again: 'Chercher encore',
  add: 'Ajouter un terme… (Entrée)',
  remove: 'Retirer',
  none: 'Aucun terme : ajoutez-en un pour chercher.',
  replaced: 'Les termes ont été mis à jour par la recherche : vos modifications non lancées ont été remplacées.',
}

const NO_HOST = 'Runs in a panel'

/** How long a run waits for the producer's terms at most: an answer with the
 *  same terms ends it only so. */
export const TERMS_BUSY_MS = 15_000

export function TermsRenderer({ node }: { node: UITermsNode }): ReactNode {
  const item = useItem()
  const run = useRunAction()(node.action)
  const labels = { ...TERMS_LABELS, ...(node.labels ?? {}) }
  const [state, move] = useReducer(termsReduce, node.terms, termsInit)
  const [typed, setTyped] = useState('')

  // The producer's terms: by content, never by the node's identity (a page
  // redraws its nodes on every render).
  const sent = JSON.stringify(node.terms ?? [])
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { move({ type: 'bound', terms: node.terms }) }, [sent])
  useEffect(() => {
    if (!state.busy) return
    const timer = setTimeout(() => move({ type: 'timeout' }), TERMS_BUSY_MS)
    return () => clearTimeout(timer)
  }, [state.busy])

  const { terms, busy } = state
  const toRun = termsToRun(terms, typed)
  const off = !run || node.disabled || busy || toRun.length === 0
  const again = (
    <Button size="compact-xs" variant="light" disabled={off} loading={busy} data-guide="terms.again"
      onClick={() => {
        if (off || !run) return
        move({ type: 'run', terms: toRun, wait: !pageOwn(node.action) })
        setTyped('')
        const e = termsEvent(node.action, item, toRun)
        run(e.item, e.extra)
      }}>
      {labels.again}
    </Button>
  )
  return (
    <Stack {...dataAttrs(node)} gap={6} data-element={node.id ? `terms:${node.id}` : undefined} data-guide="terms">
      <Text size="sm" fw={600}>{node.title ?? labels.title}</Text>
      <Group gap={6} wrap="wrap" align="center">
        {terms.length === 0 && <Text size="xs" c="dimmed">{labels.none}</Text>}
        {terms.map((t) => (
          <Badge key={t.toLowerCase()} size="md" variant="light" color="gray" tt="none" data-guide="terms.term" data-guide-name={t}
            rightSection={
              <ActionIcon size={14} variant="transparent" color="gray" aria-label={`${labels.remove} ${t}`} disabled={node.disabled || busy}
                onClick={() => move({ type: 'edit', terms: removeTerm(terms, t) })}>
                <IconX size={10} />
              </ActionIcon>
            }>
            {t}
          </Badge>
        ))}
        <TextInput size="xs" w={200} placeholder={labels.add} value={typed} aria-label={labels.add} data-guide="terms.add"
          disabled={node.disabled || busy}
          onChange={(e) => setTyped(e.currentTarget.value)}
          onKeyDown={(e) => {
            if (e.key !== 'Enter') return
            e.preventDefault()
            move({ type: 'edit', terms: addTerm(terms, typed) })
            setTyped('')
          }} />
        {run ? again : <Tooltip label={NO_HOST} withArrow><span>{again}</span></Tooltip>}
      </Group>
      {state.replaced && <Text size="xs" c="orange" data-guide="terms.replaced">{labels.replaced}</Text>}
      {node.note && <Text size="xs" c="dimmed">{node.note}</Text>}
    </Stack>
  )
}
