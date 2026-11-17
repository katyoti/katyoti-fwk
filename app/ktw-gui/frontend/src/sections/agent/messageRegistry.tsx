// SPDX-License-Identifier: Apache-2.0
/**
 * Message Routing Registry
 * ─────────────────────────────────────────────────────────────────────────
 * Maps each message kind to its display configuration and content renderer.
 *
 * Columns:
 *   color           — Mantine color for accent border, icon, badge
 *   icon            — icon displayed in the collapsed title bar
 *   summary         — one-line text shown when collapsed
 *   defaultOpen     — initial expand state
 *   boxed           — true → MessageWrapperBox (Paper + collapse),
 *                     false → MessageWrapperRaw (naked content)
 *   ContentRenderer — inner content component
 */
import React from 'react'
import type { ComponentType } from 'react'
import {
  IconAlertTriangle,
  IconCheck,
  IconCircleX,
  IconBook,
  IconFolderSearch,
  IconGitCommit,
  IconInfoCircle,
  IconLoader,
  IconPackage,
  IconPencil,
  IconTool,
  IconTerminal2,
} from '@tabler/icons-react'
import type { MsgItem, TextMsg, ToolMsg, SysMsg, PromptMsg } from '../../types/messages'
import { TextMessageContent }   from './messages/TextMessage'
import { ToolMessageContent }   from './messages/ToolMessage'
import { SystemMessageContent } from './messages/SystemMessage'
import { AnswerViewContent }    from './messages/AnswerViewMessage'
import { AskFormContent }       from './messages/AskFormMessage'
import { tFor } from '../../i18n'
import type { TFn } from '../../i18n'

// ── Shared tool helpers (exported for use in ToolMessageContent) ──────────

export type ToolMeta = {
  verb:  string
  Icon:  ComponentType<{ size?: number | string }>
  color: string
}

export function getToolMeta(name: string, t: TFn = tFor('chat')): ToolMeta {
  if (/read_file|read/.test(name))        return { verb: t('tools.read'),       Icon: IconBook,         color: 'orange' }
  if (/write_file|write/.test(name))      return { verb: t('tools.write'),      Icon: IconPencil,       color: 'blue'   }
  if (/glob|find_files|found/.test(name)) return { verb: t('tools.foundFiles'), Icon: IconFolderSearch, color: 'cyan'   }
  if (/bash|run|execute|cmd/.test(name))  return { verb: t('tools.ran'),        Icon: IconTerminal2,    color: 'green'  }
  if (/go_build|build/.test(name))        return { verb: t('tools.built'),      Icon: IconPackage,      color: 'teal'   }
  if (/git/.test(name))                   return { verb: t('tools.git'),        Icon: IconGitCommit,    color: 'violet' }
  return { verb: name, Icon: IconTool, color: 'gray' }
}

export function toolPrimaryArg(input: string): string {
  try {
    const obj = JSON.parse(input)
    for (const v of Object.values(obj)) {
      if (typeof v === 'string') return v
    }
  } catch { /* not JSON */ }
  return input.slice(0, 80)
}

// ── Route shape ───────────────────────────────────────────────────────────

export interface MessageRoute {
  /** Mantine color name */
  color: string
  /** Icon node shown in the box title bar */
  icon: React.ReactNode
  /** Single line of text shown when collapsed */
  summary: string
  /** Whether a boxed message starts expanded */
  defaultOpen: boolean
  /** true → MessageWrapperBox, false → MessageWrapperRaw */
  boxed: boolean
  /** Inner content component */
  ContentRenderer: ComponentType<{ msg: MsgItem }>
}

// ── Per-type route builders ───────────────────────────────────────────────

function textRoute(msg: TextMsg): MessageRoute {
  const isUser = msg.sender === 'user'
  return {
    color:       isUser ? 'orange' : 'blue',
    icon:        isUser ? <IconCheck size={12} /> : <IconLoader size={12} />,
    summary:     msg.text.trim().slice(0, 90) + (msg.text.length > 90 ? '…' : ''),
    defaultOpen: false,
    boxed:       false,
    ContentRenderer: TextMessageContent as ComponentType<{ msg: MsgItem }>,
  }
}

function toolRoute(msg: ToolMsg, t: TFn): MessageRoute {
  // Prefer server-provided display data (from terminal protocol).
  const display = msg.display
  const meta = getToolMeta(msg.name, t)
  const label = display?.label || meta.verb
  const title = display?.title || toolPrimaryArg(msg.input)
  const baseColor = display?.color ? mantineColorFromHex(display.color) : meta.color

  const colorMap: Record<ToolMsg['status'], string> = {
    running: 'yellow',
    done:    baseColor,
    error:   'red',
  }
  const iconMap: Record<ToolMsg['status'], React.ReactNode> = {
    running: <IconLoader  size={12} />, // i18n-ignore
    done:    <IconCheck   size={12} />, // i18n-ignore
    error:   <IconCircleX size={12} />,
  }

  return {
    color:       colorMap[msg.status],
    icon:        iconMap[msg.status],
    summary:     title ? `${label} · ${title}` : label,
    defaultOpen: false,
    boxed:       true,
    ContentRenderer: ToolMessageContent as ComponentType<{ msg: MsgItem }>,
  }
}

/** Map a hex color to the closest Mantine color name (best effort). */
function mantineColorFromHex(hex: string): string {
  const map: Record<string, string> = {
    '#FFCB6B': 'orange', '#C3E88D': 'green', '#82AAFF': 'blue',
    '#89DDFF': 'cyan',   '#C792EA': 'violet', '#F07178': 'red',
  }
  return map[hex.toUpperCase()] || 'gray'
}

function systemRoute(msg: SysMsg): MessageRoute {
  const colorMap: Record<string, string> = {
    info:    'gray',
    warn:    'yellow',
    error:   'red',
    success: 'green',
  }
  const iconMap: Record<string, React.ReactNode> = {
    info:    <IconInfoCircle    size={12} />, // i18n-ignore
    warn:    <IconAlertTriangle size={12} />, // i18n-ignore
    error:   <IconCircleX       size={12} />, // i18n-ignore
    success: <IconCheck          size={12} />,
  }

  return {
    color:       colorMap[msg.level] || 'gray',
    icon:        iconMap[msg.level] || <IconInfoCircle size={12} />,
    summary:     msg.text,
    defaultOpen: true,
    boxed:       true,
    ContentRenderer: SystemMessageContent as ComponentType<{ msg: MsgItem }>,
  }
}

// ── Public API ────────────────────────────────────────────────────────────

/** Look up the display + behaviour route for any message (`t` — the `chat`
 *  namespace, for the GUI's own words in the route; message text is data). */
export function getMessageRoute(msg: MsgItem, t: TFn = tFor('chat')): MessageRoute {
  switch (msg.kind) {
    case 'text':   return textRoute(msg)
    case 'tool':   return toolRoute(msg, t)
    case 'system': return systemRoute(msg)
    case 'prompt': return systemRoute({ kind: 'system', id: msg.id, sender: 'system', level: 'info', text: msg.question })
    case 'answer-view': return {
      color:       'orange',
      icon:        <IconCheck size={12} />,
      summary:     msg.title || t('route.yourAnswer'),
      defaultOpen: true,
      boxed:       false,
      ContentRenderer: AnswerViewContent as ComponentType<{ msg: MsgItem }>,
    }
    case 'ask-form': return {
      color:       'blue',
      icon:        <IconPencil size={12} />,
      summary:     msg.title || t('route.form'),
      defaultOpen: true,
      boxed:       false,
      ContentRenderer: AskFormContent as ComponentType<{ msg: MsgItem }>,
    }
  }
}
