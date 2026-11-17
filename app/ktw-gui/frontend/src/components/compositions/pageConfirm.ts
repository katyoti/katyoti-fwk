// SPDX-License-Identifier: Apache-2.0
/**
 * The texts of a page action's confirmation dialog (PageHost): what the action
 * declares — its label, its confirmation text, already in the person's language
 * — else an i18n key in the `workspace` namespace (`pageConfirm.*`, and the
 * shared `common.cancel`), so no button speaks another language than the page.
 */

/** A text of the dialog: the action's own, or an i18n key to translate. */
export type ConfirmText = { text: string } | { key: `pageConfirm.${'title' | 'body' | 'run'}` | 'common.cancel' }

export interface ConfirmableAction {
  label?: string
  confirm?: string
}

export interface ConfirmTexts {
  title: ConfirmText
  body: ConfirmText
  cancel: ConfirmText
  run: ConfirmText
}

/** The dialog's title, body and buttons for an action. */
export function confirmTextsFor(action: ConfirmableAction | undefined): ConfirmTexts {
  const own = (s: string | undefined, key: 'title' | 'body' | 'run'): ConfirmText =>
    s ? { text: s } : { key: `pageConfirm.${key}` }
  return {
    title: own(action?.label, 'title'),
    body: own(action?.confirm, 'body'),
    cancel: { key: 'common.cancel' },
    run: own(action?.label, 'run'),
  }
}
