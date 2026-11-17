// SPDX-License-Identifier: Apache-2.0
// Open-shape content envelope.
//
// Pre-M7 this file held a closed discriminated union over every default
// content type (DateContent, MarkdownContent, AvatarContent, …). M7 moved
// the catalog into per-folder manifests under `app/ktw-display/content-types/`.
// 2026-05-03 strict cleanup: this file now keeps ONLY the generic envelope.
// Type-specific aliases moved to UI-layer types or deleted entirely:
//
//   - ContentColor / ContentColorPredefined / LabelVariant
//     ContentAlign / ContentTransformation
//     → src/ui-core/fields/types.ts (UI-layer display vocabulary)
//   - StringPattern* / DatePattern* / IconName / CodeLanguage* → deleted (no consumers;
//     re-add inside the relevant content-types/<name>/types.ts when those
//     types get implemented — that direction of import is allowed)
//
// Layering rule: src/ui/ is the library; content-types/ is its catalog.
// content-types → src/ui imports are fine; the reverse is forbidden.
//
// The supporting `FieldOption*` / `FieldOptions` family stays here — they're
// generic option helpers used across multiple content types (anything with a
// finite enum/select), parameterized by `T extends FieldOptionKey`. `options`
// itself is an optional field on the generic `ContentType` envelope below, so
// any content type (string/number/boolean) can carry it.
//
// New content types are authored as per-folder TS interfaces under
// `content-types/<name>/types.ts`. They extend `ContentType` and declare their
// own `type: '<name>'` discriminator + their own params. The renderer's TSX
// imports its own `<Name>Content` directly; the schema lib does not need to
// know about it.

//= ========================================================
// Field options (used by SchemaQuery + content-type configSchema)
//= ========================================================

export type FieldOptionKey = string | number | boolean;
export type FieldOption<T extends FieldOptionKey = FieldOptionKey> = {
  value: T;
  label: string;
  /** Optional explanatory sub-text rendered under the label in rich pickers. */
  description?: string;
  /** Optional Tabler icon name (e.g. 'IconBuildingSkyscraper') rendered
   *  alongside the label. Resolved by the ui-core `Icon` component. */
  icon?: string;
};
export type FieldOptionList<T extends FieldOptionKey = FieldOptionKey> = FieldOption<T>[];
export type FieldOptionMap<T extends FieldOptionKey = FieldOptionKey> = Record<
  T extends string | number ? T : string,
  string
>;
export type FieldOptions<T extends FieldOptionKey = FieldOptionKey> =
  | FieldOptionList<T>
  | FieldOptionMap<T>;

//= ========================================================
// The open content envelope
//= ========================================================

// JsonValue forward-import (used by ContentType's open index signature).
// We don't import via `import type { JsonValue }` here because that creates a
// circular dependency with schema-types.ts → schema-contents.ts. Re-declare a
// minimal alias instead — it's structurally identical to types/json-types.ts.
type ContentTypeParam =
  | string | number | boolean | null | undefined
  | ContentTypeParam[]
  | { [key: string]: ContentTypeParam };

/**
 * Content envelope — every content config extends this.
 * `type` is an open string — the runtime registry resolves it to a
 * `content-types/<type>/` folder. The index signature lets inline literals
 * carry arbitrary JSON-serializable params
 * (`{ type: 'slider', min: 0, max: 100 }`) without an excess-property check
 * failure, while still being friendly to `Jsonify<>`.
 *
 * Per-folder TS interfaces (e.g., `EmailContent`, `MarkdownContent`) extend
 * this and narrow `type` to a string literal + tighten params. They are NOT
 * declared here; each lives alongside its renderer at
 * `content-types/<name>/types.ts`.
 */
export type ContentType = {
  type: string;

  /**
   * Optional finite option set for any pickable content type (string / number /
   * boolean). Either the rich LIST form (`[{value,label,description?,icon?}]`)
   * or the compact MAP form (`{value: label}`) — both via `FieldOptions`.
   * Generic across `FieldOptionKey`, so a number field carries number-valued
   * options, a boolean field boolean-valued ones. Read via
   * `SchemaQuery.getContentTypeOptions` (normalizes either form to a list + map).
   * Optional — content types with no enum simply omit it.
   */
  options?: FieldOptions;

  /** Optional catch-all "any / all" option (e.g. a filter's "Tous"). */
  allOption?: FieldOption;

  [key: string]: ContentTypeParam;
};

/**
 * Shape accepted by `BaseTypeWithContent.content` and `ArrayType.items.content`,
 * etc. Either a bare name (resolves to `{ type: <name> }` at lookup time) or
 * an inline object with arbitrary params.
 */
export type ContentRef = string | ContentType;
