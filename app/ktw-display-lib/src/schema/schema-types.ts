// SPDX-License-Identifier: Apache-2.0
import { ContentRef } from "./schema-contents";

import { JsonValue } from "../types/json-types";

//= ========================================================
// Allow schema extensions
//= ========================================================

export type SchemaTypeExtension<T = {}> = {
  /**
   * Type of option
   */
  type: string;

  /**
   * Name of option
   */
  name?: string;
} & T;

//= ========================================================
// Schema Types : used to create JSON objects
// Detail of type handling is done with content types
// example:
//    - type string, content type : icon
//    - type string, content type : date
//= ========================================================

export interface BaseType {
  type: SchemaPredefinedTypeName;

  /**
   * defines the label to display for this field input
   */
  label?: string;

  /**
   * defines the placeholder to display for this field input
   */
  placeholder?: string;

  /**
   * defines a description to display as help text
   */
  description?: string;

  /**
   * A html detailled description of this field
   */
  informations?: string;

  /**
   * Type extensions (example : for UI)
   */
  extensions?: SchemaTypeExtension[];

  /**
   * When true, this type is optional in its parent context (a field
   * that may be absent). Mirrors the canonical AST's `Type.Optional`
   * (Go side, `omitempty` on false). Renderers use this to drive the
   * required-marker / submit-validation behaviour: absent or false →
   * required, true → optional.
   */
  optional?: boolean;
}

/**
 * Base type that carries a content envelope. The content shape is the open
 * `ContentRef` — either a bare name (resolves to `{ type: <name> }`) or an
 * inline object with `type` + arbitrary params. Per-renderer narrowing
 * happens at the renderer's TSX level via `FieldRendererFactory<SType, CType>`.
 *
 * DSL-aligned 2026-05-01 (M6): was `contentType`. M7 (2026-05-02): dropped
 * the per-base-type generics and switched to the open ContentRef envelope.
 */
export interface BaseTypeWithContent<
  TValue extends string | number | boolean = string | number | boolean,
> extends BaseType {
  default?: TValue;
  content?: ContentRef;

  /**
   * Opaque content-type params carried through from the canonical AST's
   * `Type.ContentParams` (Go side). Go does NOT interpret these — they ride
   * as a separate JSON key alongside `content` (the content-type NAME). The
   * UI lib merges them into the content envelope at lookup time
   * (`SchemaQuery.getTypeContent`), so a content-type's params (e.g. rich
   * `options: [{value,label,description,icon}]`) reach the renderer.
   */
  contentParams?: Record<string, JsonValue>;

  /**
   * Restricted value set. Canonical DSL source: `@enum(a,b,c)` or
   * the YAML `enum: [a, b, c]` block. The canonical Schema struct
   * (Go side: `Type.Enum []any`) puts this on every primitive type;
   * renderers that recognise it should produce a select-style input
   * instead of a free-text/number input.
   */
  enum?: TValue[];
}

export interface StringType extends BaseTypeWithContent<string> {
  type: "string";
}

export interface NumberType extends BaseTypeWithContent<number> {
  type: "number";
}

export interface BooleanType extends BaseTypeWithContent<boolean> {
  type: "boolean";
}

export type TypeWithContent = StringType | NumberType | BooleanType;
export type TypeWithContentName = TypeWithContent["type"];

/**
 * The field name in a object
 */
export type SchemaFieldName = string;
export type SchemaFieldVisibility = "create" | "update" | "view" | "filter"; // when is this field used in UI. ['create', 'update', 'view']
export type SchemaFieldDetailLevel = 1 | 2 | 3; // at wich detail level this field is visible. 1: minimal, 2: medium, 3: detailed (progressive disclosure)
export type SchemaFieldOptions = {
  alias?: string | string[];
  optional?: boolean;
  visibility?: SchemaFieldVisibility[];
  detail?: SchemaFieldDetailLevel;
};

export interface ObjectType extends BaseType {
  type: "object";

  idField?: SchemaFieldName | SchemaFieldName[];

  titleField?: SchemaFieldName | SchemaFieldName[];

  /**
   * defines the list of fields for this object (shape)
   */
  fields?: Record<SchemaFieldName, SchemaTypeRef<SchemaFieldOptions>>;

  /**
   * Allow any field of type TypeId
   * like Record<string, TypeId>
   */
  catchall?: SchemaTypeName;

  /**
   * Allow object extension
   */
  extends?: SchemaUserTypeName | SchemaUserTypeName[];
}

export type SchemaItemOptions = {};

export interface ArrayType extends BaseType {
  type: "array";
  items: SchemaTypeRef<SchemaItemOptions>;
  min?: number;
  max?: number;

  /**
   * An array field may declare its own content envelope (e.g. a multi-select
   * `enum-multi` carrying rich `options`). Mirrors the content-bearing
   * primitives; the form builder reads it for the array field directly
   * (the item type's content is for the per-item scalar, not the picker).
   */
  content?: ContentRef;
  contentParams?: Record<string, JsonValue>;
}

export interface AnyType extends BaseType {
  type: "any";
}

export interface NullType extends BaseType {
  type: "null";
}

export type SchemaChoiceOptions = {};

export interface ChoiceType extends BaseType {
  type: "choice";
  types: SchemaTypeRef<SchemaChoiceOptions>[];
}

// ============================================================================
// LINK MODE AND OPTIONS
// ============================================================================

/**
 * Defines possible cardinalities for entity relationships.
 */
export type LinkMode = "one2one" | "one2many" | "many2one" | "many2many";

// ============================================================================
// LINK TYPE (SchemaType for entity relationships)
// ============================================================================

/**
 * Link type for entity-to-entity relationships.
 * Used to define fields that link to other entities.
 *
 * - type: must be 'link'
 * - target: Target entity type (e.g., 'user', 'resource')
 * - link_mode: Cardinality mode (optional, inferred if not provided)
 * - target_id_field: ID field name in target entity (optional, inferred if not provided)
 * - link_type: Semantic link type (optional, default: 'associated')
 * - reverse_field: Field name for reverse relationship (optional)
 * - display_field: Field to display for linked entities (optional)
 * - allow_create: Allow creating new target entities inline (default: true)
 * - searchable: Is the link field searchable (default: true)
 * - default_value: Default value (ID or IDs of linked entities)
 *
 * For usage, see examples in Python comments.
 */
export interface LinkType extends BaseType {
  type: "link";

  target: string;

  /** Target entity ID field (default: determined from target entity's idField). DSL-aligned 2026-05-01: was `targetIdField`. */
  target_id?: string;

  /** Link cardinality mode: one2one, one2many, many2one, many2many (optional, inferred if missing). DSL-aligned 2026-05-01: was `linkMode`. */
  mode?: LinkMode;

  /** Semantic link type (default: 'associated'). DSL-aligned 2026-05-01: was `linkType`. */
  link_type?: string;

  /** Field name for reverse relationship on target entity. DSL-aligned 2026-05-01: was `reverseField`. */
  reverse?: string;

  /** Field to display for linked entities (default: target entity's titleField). DSL-aligned 2026-05-01: was `displayField`. */
  display?: string;

  /** Allow creating new target entities inline (default: true). DSL-aligned 2026-05-01: was `allowCreate`. */
  allow_create?: boolean;

  /** Enable search when selecting targets (default: true) */
  searchable?: boolean;

  /** Default linked entity ID(s) */
  default?: string | string[];
}

// ============================================================================
// TREE TYPE (extends LinkType for hierarchical self-references)
// ============================================================================

/**
 * Tree type for hierarchical self-referencing relationships.
 * Extends LinkType for same-entity parent-child relationships.
 *
 * - type: must be 'tree'
 * - target: Target entity type (hierarchy node type)
 * - mode: always 'many2one'
 * - link_type: always 'part_of'
 * - root_value: Value for root nodes (default: null)
 * - allow_circular: Allow circular references (default: false)
 * - max_depth: Maximum tree depth allowed (optional)
 * - show_path: Show breadcrumb path in tree view (default: true)
 * - drag_drop: Enable drag-drop reordering in tree view (default: true)
 *
 * DSL-aligned 2026-05-01 — see MIGRATION.md for the field-name renames.
 */
export interface TreeType extends Omit<
  LinkType,
  "type" | "mode" | "link_type"
> {
  type: "tree";

  /** mode always 'many2one' for trees */
  mode: "many2one";

  /** link_type always 'part_of' for hierarchy */
  link_type: "part_of";

  /** Value for root nodes (default: null for top-level entities). DSL-aligned 2026-05-01: was `rootValue`. */
  root_value?: string | null;

  /** Allow circular references (default: false). DSL-aligned 2026-05-01: was `allowCircular`. */
  allow_circular?: boolean;

  /** Maximum tree depth (default: unlimited). DSL-aligned 2026-05-01: was `maxDepth`. */
  max_depth?: number;

  /** Show breadcrumb path in tree view (default: true). DSL-aligned 2026-05-01: was `showPath`. */
  show_path?: boolean;

  /** Enable drag-drop reordering in tree view (default: true). DSL-aligned 2026-05-01: was `enableDragDrop`. */
  drag_drop?: boolean;
}

export type SchemaPrimitiveType = StringType | NumberType | BooleanType;
export type SchemaRelationType = LinkType | TreeType;
export type SchemaComplexType =
  | ObjectType
  | ArrayType
  | AnyType
  | NullType
  | ChoiceType;
export type SchemaType =
  | SchemaPrimitiveType
  | SchemaRelationType
  | SchemaComplexType;

export type SchemaPredefinedTypeName = SchemaType["type"];
export type SchemaUserTypeName = Exclude<string, SchemaPredefinedTypeName>;
export type SchemaTypeName = SchemaPredefinedTypeName | SchemaUserTypeName;
export type SchemaTypeRef<T extends Record<string, any> = {}> =
  | (SchemaType & T)
  | SchemaTypeName
  | ({ $ref: SchemaTypeName } & T);

/**
 * The data that a schema can model
 */
export type SchemaData = JsonValue;

/**
 * A schema with all referenced types.
 *
 * Field naming follows the schema DSL (modules/framework/schema/dsl.md).
 * See `app/ktw-display/src/schema/MIGRATION.md` for the migration log of fields renamed
 * from the original ktw_workspace lib shape.
 */
export type Schema = {
  readonly name: string;
  readonly version?: number; // integer monotonic; was `string` before 2026-05-01
  readonly description?: string;
  readonly imports?: string[];
  readonly root: SchemaUserTypeName; // renamed from `typeName` 2026-05-01
  readonly types: Record<string, SchemaTypeRef>;
};
