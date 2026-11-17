// SPDX-License-Identifier: Apache-2.0
// JsonObject was imported from @core/types in the original ktw_workspace home;
// inlined here during schema-module promotion (2026-05-01) to keep the lib self-contained.
type JsonObject = Record<string, unknown>;

import {
  ContentType,
  FieldOption,
  FieldOptionKey,
  FieldOptionList,
  FieldOptionMap,
  FieldOptions,
} from './schema-contents';
import {
  ObjectType,
  Schema,
  SchemaData,
  SchemaFieldOptions,
  SchemaType,
  SchemaTypeExtension,
  SchemaTypeName,
  SchemaTypeRef,
  TypeWithContent,
  TypeWithContentName,
} from './schema-types';

export type SchemaFieldInfo = {
  /** The object type that declares the field (matters with `extends:` inheritance). */
  definerType: ObjectType;
  fieldName: string;
  fieldType: SchemaType;
  /** Full SchemaFieldOptions (alias, optional, visibility, detail) carried through from the field declaration. */
  fieldOptions: SchemaFieldOptions;
  /** True iff `fieldType.type === 'array'`. Lifted here so callers don't have to recompute. */
  isArray: boolean;
};

export type OptionsInfo<T extends FieldOptionKey = FieldOptionKey> = {
  optionList?: FieldOptionList<T>;
  optionMap?: FieldOptionMap<T>;
  allOption?: FieldOption<T>;
};

//= ========================================================
// Kind-default content names
//= ========================================================
// Each base type with content has exactly one default content name.
// When a field omits `content`, this map dispatches the renderer.
// The runtime registry (see app/ktw-display/src/content-types/) is the
// source of truth for which content names exist + their renderers; the
// schema lib only needs to know the kind defaults.

export const KIND_DEFAULT_CONTENT: Record<TypeWithContentName, string> = {
  string: 'text',
  number: 'number',
  boolean: 'boolean',
};

export const ReferencableTypes: Record<SchemaTypeName, SchemaType> = {
  string: { type: 'string' },
  number: { type: 'number' },
  boolean: { type: 'boolean' },
  any: { type: 'any' },
  null: { type: 'null' },
  object: { type: 'object', catchall: 'any' },
  array: { type: 'array', items: 'any' },
};

export class SchemaQuery {
  //= =================================================================
  // Utilities
  //= =================================================================

  static nonNullArray<T>(item: T | T[] | undefined): T[] {
    // eslint-disable-next-line no-nested-ternary
    return item ? (Array.isArray(item) ? item : [item]) : [];
  }

  /**
   * Return true if the type is one of the system predefined types (ex : object, string, etc ...)
   */
  static isPredefined(typeOrTypeId: SchemaType | string): boolean {
    const typeId = typeof typeOrTypeId === 'string' ? typeOrTypeId : typeOrTypeId.type;
    return ['string', 'text', 'number', 'boolean', 'object', 'array', 'any', 'null', 'choice', 'link', 'tree'].includes(typeId);
  }

  /**
   * Return true if the type is one of the system predefined types (ex : object, string, etc ...)
   */
  static isObject(type: SchemaType): type is ObjectType {
    return type.type === 'object';
  }

  //= =================================================================
  // Type query
  //= =================================================================

  static isSchema(typeOrSchema: SchemaType | Schema): typeOrSchema is Schema {
    return !!(typeOrSchema as Schema).types && !!(typeOrSchema as Schema).root;
  }

  static isTypeRef(typeOrTypename: SchemaTypeRef): typeOrTypename is { $ref: string } {
    return typeof typeOrTypename === 'object' && '$ref' in typeOrTypename;
  }

  static getSchema(schema: Schema, typename?: string): Schema {
    if (typename)
      return {
        name: schema.name,
        types: schema.types,
        root: typename,
      };
    return schema;
  }

  static getType(schema: Schema, typeOrTypename?: SchemaTypeRef): SchemaType {
    if (typeOrTypename === undefined) typeOrTypename = schema.root;
    // Guard: a schema with no `root` leaves typeOrTypename undefined, which
    // used to fall through to `'type' in undefined` (an opaque TypeError that
    // crashed the whole runtime). Fail with a clear, catchable message instead.
    if (typeOrTypename === undefined || typeOrTypename === null) {
      throw new Error(`Schema "${schema?.name ?? '?'}" has no root type, and no type was supplied`);
    }
    if (this.isTypeRef(typeOrTypename!)) typeOrTypename = (typeOrTypename as { $ref: string }).$ref;
    if (typeof typeOrTypename !== 'string') {
      if ('type' in typeOrTypename!) return typeOrTypename as SchemaType;
      throw new Error(`Bad type ${JSON.stringify(typeOrTypename)}`);
    }
    const type = schema.types[typeOrTypename];
    if (!type) {
      if (ReferencableTypes[typeOrTypename]) {
        return { ...ReferencableTypes[typeOrTypename] };
      }
      throw new Error(`Unknown type ${typeOrTypename}`);
    }
    return this.getType(schema, type);
  }

  //= =================================================================
  // Type extensions
  //= =================================================================

  static getSchemaExtension<T extends SchemaTypeExtension = SchemaTypeExtension>(
    schema: Schema,
    typeOrTypename: SchemaType | string | undefined,
    extensionType: T['type'],
  ): T[] {
    const type = this.getType(schema, typeOrTypename);
    return (type.extensions?.filter((ext) => ext.type === extensionType) as T[]) ?? [];
  }

  static getFirstSchemaExtension<T extends SchemaTypeExtension = SchemaTypeExtension>(
    schema: Schema,
    typeOrTypename: SchemaType | string | undefined,
    extensionType: T['type'],
  ): T | undefined {
    const result = this.getSchemaExtension(schema, typeOrTypename, extensionType);
    return result.length > 0 ? result[0] : undefined;
  }

  static getNamedSchemaExtension<T extends SchemaTypeExtension = SchemaTypeExtension>(
    schema: Schema,
    typeOrTypename: SchemaType | string | undefined,
    extensionType: T['type'],
    extensionName: string | undefined,
    defaultsToUnamed: boolean = true,
  ): T | undefined {
    const allExtensions = this.getSchemaExtension(schema, typeOrTypename, extensionType);
    const searchedOne = extensionName ? allExtensions.find((ext) => ext.name === extensionName) : undefined;
    if (searchedOne) return searchedOne;
    if (defaultsToUnamed) return allExtensions.find((ext) => !ext.name);
    return undefined;
  }

  //= =================================================================
  // Options query
  //= =================================================================

  static getContentTypeOptions<TKey extends FieldOptionKey = FieldOptionKey>(contentType?: ContentType): OptionsInfo<TKey> {
    const options = contentType?.options as FieldOptions<TKey> | undefined;
    if (options === undefined) return {};
    return {
      optionList: this.toOptionList(options),
      optionMap: this.toOptionMap(options),
      allOption: contentType?.allOption as FieldOption<TKey> | undefined,
    };
  }

  static getTypeOptions<TKey extends FieldOptionKey = FieldOptionKey>(
    schema: Schema,
    typeOrTypename?: SchemaType | string,
  ): OptionsInfo<TKey> {
    const content = this.getTypeContent(schema, typeOrTypename);
    return this.getContentTypeOptions(content);
  }

  static toOptionList<T extends FieldOptionKey = FieldOptionKey>(options: FieldOptions<T>): FieldOptionList<T> {
    if (Array.isArray(options)) return options as FieldOptionList<T>;
    if (typeof options === 'object') return Object.entries<string>(options).map(([key, label]) => ({ value: key as T, label }));
    return [];
  }

  static toOptionKeys<T extends FieldOptionKey = FieldOptionKey>(options: FieldOptions<T>): T[] {
    return this.toOptionList(options).map(({ value }) => value);
  }

  static toOptionMap<T extends FieldOptionKey = FieldOptionKey>(options: FieldOptions<T>): FieldOptionMap<T> {
    function toKey<X>(value: X): X extends string | number ? X : string {
      switch (typeof value) {
        case 'string':
          return value as X extends string | number ? X : string;
        case 'number':
          return value as X extends string | number ? X : string;
        default:
          return `${value}` as X extends string | number ? X : string;
      }
    }
    if (Array.isArray(options)) {
      const result: FieldOptionMap<T> = {} as FieldOptionMap<T>;
      options.forEach(({ value, label }) => {
        result[toKey<T>(value)] = label;
      });
      return result;
    }
    if (typeof options === 'object') return options as FieldOptionMap<T>;
    return {} as FieldOptionMap<T>;
  }

  //= =================================================================
  // Content Query
  //= =================================================================

  /**
   * Resolve a field type's content envelope (the `{ type: <name>, ...params }`
   * the renderer registry dispatches on).
   *
   *   - If the type isn't a content-bearing kind (string/number/boolean), returns undefined.
   *   - If `field.content` is unset, returns `{ type: KIND_DEFAULT_CONTENT[kind] }`.
   *   - If `field.content` is a string, returns `{ type: <string> }`.
   *   - If `field.content` is an object, returns it as-is (already in canonical form).
   *
   * The runtime catalog (app/ktw-display/content-types/) is the source of truth
   * for which content names actually have renderers; this method just normalizes
   * the ref into the open envelope shape.
   */
  static getTypeContent(schema: Schema, typeOrTypename?: SchemaTypeRef): ContentType | undefined {
    const type = this.getType(schema, typeOrTypename);
    const defaultName = KIND_DEFAULT_CONTENT[type.type as TypeWithContentName];
    if (defaultName === undefined) return undefined;
    return this.toContentEnvelope(type as TypeWithContent, defaultName);
  }

  /**
   * Merge a content-bearing type's `content` (name) + `contentParams` (opaque
   * pass-through from the Go AST) into a single content envelope. This is the
   * bridge: Go emits `content` + `contentParams` as separate JSON keys (it does
   * not interpret the params), so a content-type's params — notably rich
   * `options: [{value,label,description,icon}]` — only reach the renderer once
   * folded back into the envelope here.
   *
   *   - `content` is an object → already canonical (inline params); returned as-is.
   *   - `content` is a string / absent → `{ type, ...contentParams }`.
   *
   * `defaultName` is the kind-default content name used when `content` is absent.
   */
  static toContentEnvelope(
    type: { content?: ContentType | string; contentParams?: Record<string, unknown> },
    defaultName: string,
  ): ContentType {
    const ref = type.content;
    if (ref !== undefined && ref !== null && typeof ref === 'object') return ref as ContentType;
    const params = type.contentParams ?? {};
    // A data type's long form may write its params inline: `gate-dots(labels)`.
    const inline = typeof ref === 'string' ? this.parseContentRef(ref) : undefined;
    const name = inline?.name || defaultName;
    return { type: name, ...(inline?.params ?? {}), ...params } as ContentType;
  }

  /**
   * parseContentRef reads a content reference with its inline params, as the
   * shorthand writes them: `name`, `name(flag)`, `name(key:value, flag)`. A
   * bare word is a flag (true); a value reads as a number or a boolean when it
   * is one, else as text.
   */
  static parseContentRef(ref: string): { name: string; params: Record<string, unknown> } {
    const m = /^\s*([^()\s]+)\s*(?:\((.*)\))?\s*$/.exec(ref);
    if (!m) return { name: ref.trim(), params: {} };
    const params: Record<string, unknown> = {};
    for (const part of (m[2] ?? '').split(',').map((s) => s.trim()).filter(Boolean)) {
      const i = part.indexOf(':');
      if (i < 0) { params[part] = true; continue; }
      const k = part.slice(0, i).trim();
      const raw = part.slice(i + 1).trim();
      params[k] = raw === 'true' ? true : raw === 'false' ? false : raw !== '' && !Number.isNaN(Number(raw)) ? Number(raw) : raw;
    }
    return { name: m[1], params };
  }

  //= =================================================================
  // Field Query
  //= =================================================================

  private static toField(
    schema: Schema,
    field: SchemaTypeRef<SchemaFieldOptions>,
  ): { fieldType: SchemaType; fieldOptions: SchemaFieldOptions; isArray: boolean } {
    const fieldType = this.getType(schema, field);
    const opts = typeof field === 'object' && !this.isTypeRef(field as SchemaTypeRef) ? (field as SchemaFieldOptions) : {};
    const { alias, optional = false, visibility, detail } = opts;
    return {
      fieldType,
      fieldOptions: { alias, optional, visibility, detail },
      isArray: fieldType.type === 'array',
    };
  }

  /**
   * return the set of all field from this object and all extended objects
   * @param schema
   * @returns
   * ###TODO : recurse down to 'extends' types and handle multi fields cases, test if 'id' field exists
   */
  static getIdAndTitleField<T extends JsonObject = JsonObject>(
    schema: Schema,
    typeOrTypename?: SchemaType | string,
  ): {
    idField: (item: T) => string;
    titleField: (item: T) => string;
  } {
    const type = this.getType(schema, typeOrTypename);
    const typeName = typeof typeOrTypename === 'string' ? typeOrTypename : `typeof ${type.type}`;
    if (!this.isObject(type)) throw new Error(`Type ${typeName} of type ${type.type} has no fields`);

    let { idField = 'id', titleField = 'id' } = type;

    if (typeof idField === 'string') idField = [idField];
    if (!Array.isArray(idField)) throw new Error(`Type ${typeName} of type ${type.type} has no string or string[] idField`);

    if (typeof titleField === 'string') titleField = [titleField];
    if (!Array.isArray(titleField)) throw new Error(`Type ${typeName} of type ${type.type} has no string or string[] titleField`);

    // Filter out undefined/null fields BEFORE stringifying to avoid the
    // common bug where rowId({}) returns the literal string "undefined"
    // (because [undefined].map(String).join(' ') === 'undefined' — truthy).
    // With the filter, an item with no id resolves to '' (falsy), letting
    // callers fall back to a "New" placeholder cleanly.
    const stringify = (row: T, keys: string[]): string =>
      keys.map((key) => row[key]).filter((v) => v !== undefined && v !== null).join(' ');
    return {
      idField: (row: T) => stringify(row, idField as string[]),
      titleField: (row: T) => stringify(row, titleField as string[]),
    };
  }

  /**
   * return the set of all field from this object and all extended objects
   * @param schema
   * @param onlyRequired
   * @returns
   */
  static getFieldInfo(
    schema: Schema,
    typeOrTypename: SchemaTypeRef | undefined,
    fieldName: string,
    alreadyVisited: Set<SchemaType> = new Set(),
  ): SchemaFieldInfo | undefined {
    const type = this.getType(schema, typeOrTypename);
    // recursion gard
    alreadyVisited.add(type);

    let foundField: SchemaFieldInfo | undefined;
    if (!this.isObject(type)) throw new Error(`Type ${schema.root} of type ${type.type} has no fields`);

    const res = type.fields && type.fields[fieldName];
    if (res) {
      const { fieldType, fieldOptions, isArray } = this.toField(schema, res);
      foundField = {
        definerType: type,
        fieldName,
        fieldType,
        fieldOptions,
        isArray,
      };
    }

    if (!foundField) {
      // visite extended types
      const extended = this.nonNullArray(type.extends);
      for (const extendedTypeName of extended) {
        const extendedType = this.getType(schema, extendedTypeName);
        if (alreadyVisited.has(extendedType))
          throw new Error(`Recursion detected for extension ${extendedTypeName} in stack ${alreadyVisited.values}`);
        foundField = this.getFieldInfo(schema, extendedType, fieldName, alreadyVisited);
        if (foundField) break;
      }
    }
    alreadyVisited.delete(type);
    return foundField;
  }

  /**
   * The field a dotted path names (`need.summary`): each segment but the
   * last a field of an object type (a named type resolved, extended types
   * included), the last the field returned. A plain name is getFieldInfo's.
   * Undefined when a segment is missing or not an object.
   */
  static getFieldInfoAt(
    schema: Schema,
    typeOrTypename: SchemaTypeRef | undefined,
    path: string,
  ): SchemaFieldInfo | undefined {
    const names = path.split('.');
    let type: SchemaTypeRef | undefined = typeOrTypename;
    try {
      for (let i = 0; i < names.length - 1; i++) {
        const fi = this.getFieldInfo(schema, type, names[i]);
        if (!fi || fi.isArray) return undefined;
        let next = this.getType(schema, fi.fieldType);
        if (next.type !== 'object' && schema.types?.[next.type] !== undefined) next = this.getType(schema, next.type);
        if (!this.isObject(next)) return undefined;
        type = next;
      }
      const leaf = this.getFieldInfo(schema, type, names[names.length - 1]);
      return leaf ? { ...leaf, fieldName: path } : undefined;
    } catch {
      return undefined;
    }
  }

  static getMandatoryFieldInfo(
    schema: Schema,
    typeOrTypename: SchemaTypeRef | undefined,
    fieldName: string,
    alreadyVisited: Set<SchemaType> = new Set(),
  ): SchemaFieldInfo {
    const field = this.getFieldInfo(schema, typeOrTypename, fieldName, alreadyVisited);
    if (!field) throw new Error(`Field ${fieldName} not found in type ${typeOrTypename}`);
    return field;
  }

  /**
   * return the set of all field from this object and all extended objects
   * @param schema
   * @param onlyRequired only add the fields that are not optional
   * @param duplicates if true, add all fields, even if they are already defined in a parent object
   * @returns
   */
  static getFieldInfos(
    schema: Schema,
    typeOrTypename: SchemaType | string | undefined,
    onlyRequired: boolean = false,
    duplicates: boolean = false,
    foundFields: SchemaFieldInfo[] = [],
    alreadyVisited: Set<SchemaType> = new Set(),
  ): SchemaFieldInfo[] {
    const type = this.getType(schema, typeOrTypename);

    // recursion gard
    alreadyVisited.add(type);

    if (!this.isObject(type)) throw new Error(`Type ${schema.root} of type ${type.type} has no fields`);
    if (!type.fields) throw new Error(`Type ${schema.root} has no fields`);

    // visite extended types
    const extended = this.nonNullArray(type.extends);
    extended.forEach((extendedTypeName) => {
      const extendedType = this.getType(schema, extendedTypeName);
      if (alreadyVisited.has(extendedType))
        throw new Error(`Recursion detected for extension ${extendedType} in stack ${alreadyVisited.values}`);
      else this.getFieldInfos(schema, extendedType, onlyRequired, duplicates, foundFields, alreadyVisited);
    });

    // visite fields
    for (const fieldName in type.fields) {
      if (Object.hasOwn(type.fields, fieldName)) {
        const { fieldType, fieldOptions, isArray } = this.toField(schema, type.fields[fieldName]);

        // eslint-disable-next-line no-continue
        if (onlyRequired && fieldOptions.optional) continue;

        // eslint-disable-next-line no-continue
        if (!duplicates && foundFields.find((f) => f.fieldName === fieldName)) continue;

        foundFields.push({
          definerType: type,
          fieldName,
          fieldType,
          fieldOptions,
          isArray,
        });
      }
    }

    alreadyVisited.delete(type);
    return foundFields;
  }

  //= =================================================================
  // Default Values
  //= =================================================================

  static getDefaultValue(schema: Schema, typeOrTypename?: SchemaTypeRef): SchemaData {
    let type = this.getType(schema, typeOrTypename);
    // A field/item shape `{ type: 'SomeNamedType' }` is returned by getType
    // UNTOUCHED (it has a `type` key), so a named ref (e.g. a nested object like
    // `ScopeContext`, or an enum-typed field) never resolves to its definition
    // and would hit the `default` throw below ("Unknown schema type"). Resolve
    // declared named types here. Primitives (`string`, `number`, `object`, …)
    // are not in `schema.types`, so they pass straight through.
    if (type.type !== undefined && schema.types?.[type.type] !== undefined) {
      type = this.getType(schema, type.type);
    }
    switch (type.type) {
      case 'string':
        return type.default ?? '';
      case 'number':
        return type.default ?? 0;
      case 'boolean':
        return type.default ?? false;
      case 'null':
        return null;
      case 'any':
        return null;
      case 'choice': {
        const innerTypeRef = type.types[0];
        return this.getDefaultValue(schema, innerTypeRef);
      }
      case 'array': {
        // A declared default (sources ticked at creation) is the list it opens with.
        const declared = (type as { default?: unknown }).default;
        if (Array.isArray(declared)) return [...declared];
        const aresult: any[] = [];
        if (type.min && type.min > 0) {
          const innerTypeRef = type.items;
          const item = this.getDefaultValue(schema, innerTypeRef);
          for (let i = 0; i < type.min; i++) {
            aresult.push(item);
          }
        }
        return aresult;
      }
      case 'object': {
        // Every required field gets its default; an optional one only when it
        // declares a `default:` — a form opens with the values its schema
        // states (a language, a checkbox ticked), and an optional field
        // without one stays absent.
        const result: any = {};
        const fields = this.getFieldInfos(schema, type);
        fields.forEach((field) => {
          if (field.fieldOptions.optional && (field.fieldType as { default?: unknown }).default === undefined) return;
          result[field.fieldName] = this.getDefaultValue(schema, field.fieldType);
        });
        return result;
      }
      case 'link':
        return [];

      case 'tree':
        return null;

      default:
        throw new Error(`Unknown schema type ${type}`);
    }
  }
}
