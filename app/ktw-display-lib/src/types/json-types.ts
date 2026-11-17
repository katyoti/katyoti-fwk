// SPDX-License-Identifier: Apache-2.0
// JsonValue/Jsonify were imported from @core/types in the original ktw_workspace home;
// inlined here during schema-module promotion (2026-05-01) to keep the lib self-contained.
export type JsonPrimitive = string | number | boolean | null;
export type JsonValue =
  | JsonPrimitive
  | JsonValue[]
  | { [key: string]: JsonValue };
export type JsonObject = { [key: string]: JsonValue };
export type JsonArray = JsonValue[];
export type Jsonify<T> = T extends JsonPrimitive
  ? T
  : T extends Array<infer U>
    ? Jsonify<U>[]
    : T extends object
      ? { [K in keyof T]: Jsonify<T[K]> }
      : never;
