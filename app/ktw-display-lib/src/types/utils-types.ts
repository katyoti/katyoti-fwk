// SPDX-License-Identifier: Apache-2.0
/**
 * Common TypeScript types used across all API modules
 *
 * These types are shared across all modules and should be imported first.
 */

// ============================================================================
// COMMON TYPES
// ============================================================================

export type MaybePromise<T> = T | Promise<T>;

export async function resolveMaybePromise<T>(value: MaybePromise<T>): Promise<T> {
  if (value instanceof Promise) {
    return await value;
  }
  return value;
}

// ============================================================================

export type ValueOrFunction<T, Args extends any[] = any[]> = T | ((...args: Args) => T);

export function resolveValueOrFunction<TResult, Args extends any[] = any[]>(
  value: ValueOrFunction<TResult, Args> | undefined,
  args: Args,
  defaultValue: TResult,
): TResult {
  if (typeof value === 'function') {
    value = (value as (...args: Args) => TResult)(...args);
  }
  return value ?? defaultValue;
}
