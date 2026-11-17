// SPDX-License-Identifier: Apache-2.0
// Renderers of the choice nodes: an inline select whose choice
// runs its action at once, and an action menu — a small button opening its
// entries, a plain entry running its action, a choice entry listing its
// options, a prompt entry asking a short text first. Each action runs on the
// element it is on (a row), the option chosen or the text typed
// as `${value}`.

import { dataAttrs } from './data-attrs.ts'
import { useEffect, useState } from 'react'
import { Button, Group, Menu, Modal, Select, Stack, TextInput, Tooltip } from '@mantine/core'
import { IconPointFilled } from '@tabler/icons-react'

import { tablerIcon } from '../../../ui-core/components/icons/index.ts'
import { ItemContext, usePageContext, useItem, useRunAction } from '../page-context.tsx'
import {
  choiceEvent, choiceOptions, menuItemAction, pathOf, promptEvent,
  type UIMenuItem, type UIMenuNode, type UISelectNode,
} from '../ui-page.interactive.ts'
import { actionLabel, actionShown } from './interactive.tsx'

const NO_HOST = 'Runs in a panel'

// =================================================================================================
// select
// =================================================================================================

export function SelectRenderer({ node }: { node: UISelectNode }) {
  const item = useItem()
  const { draft } = usePageContext()
  const run = useRunAction()(node.action)
  const [value, setValue] = useState<string | null>(node.value ?? null)
  useEffect(() => { setValue(node.value ?? null) }, [node.value])
  const options = choiceOptions(node, item, draft)
  const sel = (
    <Select {...(run ? dataAttrs(node) : {})} size={node.size ?? 'xs'} label={node.label} placeholder={node.placeholder}
      data={options.map((o) => ({ value: o.value, label: o.label ?? o.value }))}
      value={value} allowDeselect={false} comboboxProps={{ withinPortal: true }} disabled={!run}
      onChange={(v) => {
        if (v === null || v === value || !run) return
        setValue(v)
        const e = choiceEvent(node.action, item, v)
        run(e.item, e.extra)
      }}
      onClick={(e) => e.stopPropagation()}
      data-guide="page.select" data-guide-name={node.id ?? node.action.uri ?? node.action.id} />
  )
  return run ? sel : <Tooltip label={NO_HOST} withArrow><span {...dataAttrs(node)}>{sel}</span></Tooltip>
}

// =================================================================================================
// menu
// =================================================================================================

type RunOf = ReturnType<typeof useRunAction>

export function MenuRenderer({ node }: { node: UIMenuNode }) {
  const item = useItem()
  const { draft } = usePageContext()
  const runOf = useRunAction()
  const [asking, setAsking] = useState<UIMenuItem | undefined>()
  const Icon = node.icon ? tablerIcon(node.icon, IconPointFilled) : undefined
  const entries = (node.items ?? []).filter((e) => actionShown({ when: e.when }, item))
  return (
    <>
      <Menu shadow="md" position="bottom-end" withinPortal>
        <Menu.Target>
          <Button {...dataAttrs(node)} size="compact-xs" variant={node.variant ?? 'subtle'} leftSection={Icon ? <Icon size={12} /> : undefined}
            onClick={(e) => e.stopPropagation()}
            data-guide="page.menu" data-guide-name={node.id}>
            {node.label ?? '⋯'}
          </Button>
        </Menu.Target>
        <Menu.Dropdown onClick={(e) => e.stopPropagation()}>
          {entries.map((e) => <MenuEntry key={e.id} entry={e} item={item} data={draft} runOf={runOf} ask={setAsking} />)}
        </Menu.Dropdown>
      </Menu>
      {asking && <PromptModal entry={asking} item={item} runOf={runOf} close={() => setAsking(undefined)} />}
    </>
  )
}

function MenuEntry({ entry, item, data, runOf, ask }: {
  entry: UIMenuItem; item: unknown; data: unknown; runOf: RunOf; ask: (e: UIMenuItem) => void
}) {
  const action = menuItemAction(entry)
  const run = runOf(action)
  const Icon = entry.icon ? tablerIcon(entry.icon, IconPointFilled) : undefined
  const color = entry.dangerous ? 'red' : undefined
  if (entry.choice) {
    // A choice entry: its label heads the section of its options.
    const options = choiceOptions(entry.choice, item, data)
    return (
      <>
        <Menu.Label>{actionLabel(entry.label, item)}</Menu.Label>
        {options.map((o) => (
          <Menu.Item key={o.value} pl="lg" color={color} disabled={!run}
            onClick={() => { if (action && run) { const e = choiceEvent(action, item, o.value); run(e.item, e.extra) } }}
            data-guide="page.menu.choice" data-guide-name={`${entry.id}:${o.value}`}>
            {o.label ?? o.value}
          </Menu.Item>
        ))}
      </>
    )
  }
  return (
    <Menu.Item color={color} leftSection={Icon ? <Icon size={14} /> : undefined} disabled={!run}
      onClick={() => { if (entry.prompt) ask(entry); else run?.(item) }}
      data-guide="page.menu.item" data-guide-name={entry.id}>
      {actionLabel(entry.label, item)}{entry.prompt ? '…' : ''}
    </Menu.Item>
  )
}

// =================================================================================================
// row menu — its `button` entries in the row, the others under ⋯
// =================================================================================================

/** splitMenu parts a row's menu: the entries flagged `button` drawn in the
 *  row, the rest under ⋯. */
export function splitMenu(entries: UIMenuItem[] | undefined): { buttons: UIMenuItem[]; rest: UIMenuItem[] } {
  const all = entries ?? []
  return { buttons: all.filter((e) => e.button === true), rest: all.filter((e) => e.button !== true) }
}

/** RowMenu draws a row's menu on the element in ItemContext:
 *  each `button` entry as its own small button — a choice « <label> ▾ »
 *  opening its options, a prompt asking its text, a plain one running — and
 *  the remaining entries under ⋯ (none when none remain). */
export function RowMenu({ id, entries }: { id?: string; entries?: UIMenuItem[] }) {
  const item = useItem()
  const { draft } = usePageContext()
  const runOf = useRunAction()
  const [asking, setAsking] = useState<UIMenuItem | undefined>()
  const { buttons, rest } = splitMenu(entries)
  const shown = buttons.filter((e) => actionShown({ when: e.when }, item))
  return (
    <>
      {shown.map((entry) => {
        const action = menuItemAction(entry)
        const run = runOf(action)
        if (entry.choice) {
          const options = choiceOptions(entry.choice, item, draft)
          return (
            <Menu key={entry.id} shadow="md" position="bottom-end" withinPortal>
              <Menu.Target>
                <Button size="compact-xs" variant="default" disabled={!run} onClick={(e) => e.stopPropagation()}
                  data-guide="page.menu.button" data-guide-name={entry.id}>
                  {actionLabel(entry.label, item)} ▾
                </Button>
              </Menu.Target>
              <Menu.Dropdown onClick={(e) => e.stopPropagation()}>
                {options.map((o) => (
                  <Menu.Item key={o.value} disabled={!run}
                    onClick={() => { if (action && run) { const e = choiceEvent(action, item, o.value); run(e.item, e.extra) } }}
                    data-guide="page.menu.choice" data-guide-name={`${entry.id}:${o.value}`}>
                    {o.label ?? o.value}
                  </Menu.Item>
                ))}
              </Menu.Dropdown>
            </Menu>
          )
        }
        return (
          <Button key={entry.id} size="compact-xs" variant="default" color={entry.dangerous ? 'red' : undefined} disabled={!run}
            onClick={(e) => { e.stopPropagation(); if (entry.prompt) setAsking(entry); else run?.(item) }}
            data-guide="page.menu.button" data-guide-name={entry.id}>
            {actionLabel(entry.label, item)}{entry.prompt ? '…' : ''}
          </Button>
        )
      })}
      {rest.length > 0 && <MenuRenderer node={{ kind: 'menu', id, items: rest }} />}
      {asking && <PromptModal entry={asking} item={item} runOf={runOf} close={() => setAsking(undefined)} />}
    </>
  )
}

// =================================================================================================
// action bar — a row of entries drawn inline (a view's `slots.actions`)
// =================================================================================================

/** An entry of an action bar: a menu entry (plain, choice or prompt), and for
 *  a choice the element's field holding its current value (`value`). */
export type UIActionEntry = UIMenuItem & { value?: string }

/** ActionBar draws entries inline on the element `item`: a
 *  choice entry as a compact select showing the element's `value` field, its
 *  choice running the action with `${value}`; a prompt entry as a button
 *  asking its text first; a plain one as a button — filled when `primary`.
 *  The entries flagged `menu` go under a ⋯ after the others. `confirm` is
 *  asked by the host, as for every action. */
export function ActionBar({ entries, item, justify }: { entries?: UIActionEntry[]; item: unknown; justify?: 'flex-start' | 'flex-end' }) {
  const { draft } = usePageContext()
  const runOf = useRunAction()
  const [asking, setAsking] = useState<UIMenuItem | undefined>()
  const all = (entries ?? []).filter((e) => actionShown({ when: e.when }, item))
  if (all.length === 0) return null
  const shown = all.filter((e) => e.menu !== true)
  const menued = all.filter((e) => e.menu === true)
  return (
    <ItemContext.Provider value={item}>
      <Group gap={6} wrap="wrap" justify={justify} data-ktw-actions="">
        {shown.map((entry) => {
          const action = menuItemAction(entry)
          const run = runOf(action)
          if (entry.choice) {
            const options = choiceOptions(entry.choice, item, draft)
            const current = entry.value ? pathOf(item, entry.value) : undefined
            const sel = (
              <Select key={entry.id} size="xs" w={130} aria-label={entry.label} placeholder={entry.label}
                data={options.map((o) => ({ value: o.value, label: o.label ?? o.value }))}
                value={current === undefined || current === null || current === '' ? null : String(current)}
                allowDeselect={false} comboboxProps={{ withinPortal: true }} disabled={!run}
                onChange={(v) => {
                  if (v === null || v === String(current ?? '') || !action || !run) return
                  const e = choiceEvent(action, item, v)
                  run(e.item, e.extra)
                }}
                onClick={(e) => e.stopPropagation()}
                data-guide="page.action.choice" data-guide-name={entry.id} />
            )
            return run ? sel : <Tooltip key={entry.id} label={NO_HOST} withArrow><span>{sel}</span></Tooltip>
          }
          const btn = (
            <Button key={entry.id} size="compact-xs" variant={entry.primary ? 'filled' : 'default'}
              color={entry.dangerous ? 'red' : undefined} disabled={!run}
              onClick={(e) => { e.stopPropagation(); if (entry.prompt) setAsking(entry); else run?.(item) }}
              data-guide="page.action" data-guide-name={entry.id} data-guide-label={entry.label}
              data-primary={entry.primary || undefined}>
              {actionLabel(entry.label, item)}{entry.prompt ? '…' : ''}
            </Button>
          )
          return run ? btn : <Tooltip key={entry.id} label={NO_HOST} withArrow><span>{btn}</span></Tooltip>
        })}
        {menued.length > 0 && <MenuRenderer node={{ kind: 'menu', id: 'actions', items: menued, variant: 'default' }} />}
      </Group>
      {asking && <PromptModal entry={asking} item={item} runOf={runOf} close={() => setAsking(undefined)} />}
    </ItemContext.Provider>
  )
}

/** PromptModal asks a prompt entry's text; OK runs its action with `${value}`
 *  the text, an empty text runs nothing. */
function PromptModal({ entry, item, runOf, close }: { entry: UIMenuItem; item: unknown; runOf: RunOf; close: () => void }) {
  const [text, setText] = useState('')
  const action = menuItemAction(entry)
  const run = runOf(action)
  const submit = () => {
    const e = action ? promptEvent(action, item, text) : undefined
    if (!e || !run) return
    run(e.item, e.extra)
    close()
  }
  return (
    <Modal opened onClose={close} title={actionLabel(entry.label, item)} size="sm" centered>
      <Stack gap="sm" onClick={(e) => e.stopPropagation()}>
        <TextInput label={entry.prompt} value={text} autoFocus data-autofocus
          onChange={(e) => setText(e.currentTarget.value)}
          onKeyDown={(e) => { if (e.key === 'Enter') submit() }}
          data-guide="page.menu.prompt" data-guide-name={entry.id} />
        <Group justify="flex-end" gap="xs">
          <Button size="xs" variant="default" onClick={close}>Cancel</Button>
          <Button size="xs" color={entry.dangerous ? 'red' : undefined} disabled={!text.trim() || !run} onClick={submit}>OK</Button>
        </Group>
      </Stack>
    </Modal>
  )
}
