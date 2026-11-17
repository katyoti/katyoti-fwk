// SPDX-License-Identifier: Apache-2.0
/**
 * A service's refusal, said in the person's language.
 *
 * A person-facing refusal (not found, a validation, a hook's refusal, a
 * transition a guard refused, a size budget…) reaches the GUI with its message
 * KEY and params beside its English text: a binding result carries
 * `{error, errorMsg: {key, params}}` (`message` / `messageMsg` for a refused
 * move), a KTSP call rejects with a KtspError carrying `msg`. The GUI shows
 * `errors:<key>` with the params, and the English text when its locales lack
 * the key — so a service's new refusal reads at once, in English, until it is
 * translated.
 */
import { useCallback } from 'react'
import { tFor, useT, type TFn } from './index'

/** A message by key — usermsg.Msg on the Go side. */
export interface ErrorMsg {
  key: string
  params?: Record<string, unknown>
}

/** What a binding result or a thrown error may carry. */
export interface KeyedError {
  error?: string
  errorMsg?: ErrorMsg | null
}

/** The error's text in the `errors` namespace's language (t), else its English text. */
export function errorTextWith(t: TFn, text: string | undefined, msg?: ErrorMsg | null): string {
  const fallback = text ?? ''
  if (!msg?.key) return fallback
  return t(msg.key, { ...(msg.params ?? {}), defaultValue: fallback || msg.key })
}

/** The text of a binding result's error, or of a thrown error (KtspError
 *  carries `msg`), in the GUI language. Outside React. */
export function errorText(err: KeyedError | Error | string | unknown): string {
  return errorTextWith(tFor('errors'), ...parts(err))
}

/** `errorText` in a component, re-rendering on a language change. */
export function useErrorText(): (err: KeyedError | Error | string | unknown) => string {
  const t = useT('errors')
  return useCallback((err) => errorTextWith(t, ...parts(err)), [t])
}

function parts(err: unknown): [string | undefined, ErrorMsg | null | undefined] {
  if (err == null) return [undefined, undefined]
  if (typeof err === 'string') return [err, undefined]
  const e = err as { error?: string; errorMsg?: ErrorMsg | null; msg?: ErrorMsg; message?: string }
  if (err instanceof Error) return [e.message, e.msg]
  return [e.error ?? e.message, e.errorMsg ?? e.msg]
}
