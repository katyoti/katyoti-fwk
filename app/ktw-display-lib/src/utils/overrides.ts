// SPDX-License-Identifier: Apache-2.0
/**
 * Override Utilities
 *
 * System for overriding default values with functions or direct values
 */

/**
 * Type for override values
 * Can be either:
 * - A function that takes the default value and returns a new value
 * - A direct value to replace the default
 */
export type OverrideValue<T> = T | ((defaultValue: T) => T);

/**
 * Type for override configuration
 * Allows overriding any property of T with either a function or a direct value
 */
export type Overrides<T> = {
  [K in keyof T]?: OverrideValue<T[K]>;
};

/**
 * Applies overrides to a default object
 *
 * @param defaults - The default object
 * @param overrides - The overrides to apply
 * @returns A new object with overrides applied
 *
 * @example
 * ```typescript
 * const defaults = { name: 'John', age: 30 };
 * const overrides = {
 *   name: (defaultName) => `${defaultName} Doe`,
 *   age: 25
 * };
 * const result = doOverrides(defaults, overrides);
 * // result = { name: 'John Doe', age: 25 }
 * ```
 */
export function doOverrides<T extends Record<string, any>>(
  defaults: T,
  overrides?: Overrides<T>,
): T {
  if (!overrides || Object.keys(overrides).length === 0) {
    return defaults;
  }

  const result = { ...defaults } as T;

  for (const key in overrides) {
    if (key in overrides) {
      const overrideValue = overrides[key];
      if (overrideValue !== undefined) {
        const keyTyped = key as keyof T;
        const defaultValue = defaults[keyTyped];
        
        if (typeof overrideValue === 'function') {
          // Override is a function, call it with the default value
          const overrideFn = overrideValue as (val: T[keyof T]) => T[keyof T];
          (result as any)[keyTyped] = overrideFn(defaultValue);
        } else {
          // Override is a direct value, use it directly
          (result as any)[keyTyped] = overrideValue;
        }
      }
    }
  }

  return result;
}

/**
 * Type helper for deep overrides (nested objects)
 * Allows overriding nested properties
 */
export type DeepOverrides<T> = {
  [K in keyof T]?: T[K] extends Record<string, any>
    ? OverrideValue<T[K]> | DeepOverrides<T[K]>
    : OverrideValue<T[K]>;
};

/**
 * Applies deep overrides to a default object (recursive)
 *
 * @param defaults - The default object
 * @param overrides - The deep overrides to apply
 * @returns A new object with deep overrides applied
 *
 * @example
 * ```typescript
 * const defaults = { user: { name: 'John', age: 30 } };
 * const overrides = {
 *   user: {
 *     name: (defaultName) => `${defaultName} Doe`,
 *     age: 25
 *   }
 * };
 * const result = doDeepOverrides(defaults, overrides);
 * // result = { user: { name: 'John Doe', age: 25 } }
 * ```
 */
export function doDeepOverrides<T extends Record<string, any>>(
  defaults: T,
  overrides?: DeepOverrides<T>,
): T {
  if (!overrides || Object.keys(overrides).length === 0) {
    return defaults;
  }

  const result = { ...defaults } as T;

  for (const key in overrides) {
    if (key in overrides) {
      const overrideValue = overrides[key];
      if (overrideValue !== undefined) {
        const defaultValue = defaults[key];
        if (typeof overrideValue === 'function') {
          // Override is a function, call it with the default value
          const newValue = (overrideValue as (val: any) => any)(defaultValue);
          (result as any)[key] = newValue;
        } else if (typeof overrideValue === 'object' && overrideValue !== null && !Array.isArray(overrideValue)) {
          // Override is a nested object, recurse
          if (typeof defaultValue === 'object' && defaultValue !== null && !Array.isArray(defaultValue)) {
            (result as any)[key] = doDeepOverrides(defaultValue as Record<string, any>, overrideValue as DeepOverrides<any>);
          } else {
            // Default is not an object, use override as direct value
            (result as any)[key] = overrideValue;
          }
        } else {
          // Override is a direct value (primitive or array), use it directly
          (result as any)[key] = overrideValue;
        }
      }
    }
  }

  return result;
}

