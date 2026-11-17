// SPDX-License-Identifier: Apache-2.0
// Schema lib — consumer-only. Go owns parsing + validation + codegen
//  (2026-05-22). This lib provides:
//   - schema-types: AST type definitions consumed by iframe-side generators
//   - schema-contents: content-type vocabulary + option helpers
//   - schema-query: SchemaQuery class for walking parsed schemas
// schema-builder, schema-json, schema-utils REMOVED 2026-05-22 — zero callers.
export * from './schema-contents';
export * from './schema-query';
export * from './schema-types';
