// SPDX-License-Identifier: Apache-2.0
// TestCase — the shape every test-case file exports as default.
//
// Drop a file at `src/test-cases/<category>/<case>.ts` exporting a
// TestCase as default. The registry picks it up at startup; it
// appears in the harness tree, addressable at `/test/<category>/<case>`.
//
// Example:
//
//   // src/test-cases/base-types/string-basic.ts
//   import type { TestCase } from '../../test/types.ts'
//
//   const testCase: TestCase = {
//     id: 'base-types/string-basic',
//     category: 'base-types',
//     title: 'String — basic',
//     description: 'A plain string field with no content type.',
//     schema: { name: 'demo', root: 'Demo', types: { Demo: { type: 'object', fields: { name: { type: 'string' } } } } },
//     value: { name: '' },
//     mode: 'edit',
//   }
//
//   export default testCase

export type CaseMode = 'view' | 'edit' | 'create' | 'update' | 'filter'

/**
 * A test case definition. Each case file exports one of these as default.
 * The schema follows the canonical AST shape from app/ktw-display/src/schema/
 * (post-DSL-alignment 2026-05-01); typed loosely here as `unknown` to keep the
 * test harness independent of the schema lib's type churn during M7 (folder-
 * based content types) and build-canonical-ast.
 */
export interface TestCase {
  /** Unique id, conventionally `<category>/<case>`. Becomes part of the URL. */
  id: string
  /** Category for tree grouping (e.g. 'base-types', 'content-types', 'composites'). */
  category: string
  /** Human-readable title shown in the tree. */
  title: string
  /** Optional one-line description shown above the rendered case. */
  description?: string
  /** Canonical AST schema. See src/schema/. */
  schema: unknown
  /** Initial value passed to the renderer. */
  value?: unknown
  /** Render mode. Default: 'edit'. */
  mode?: CaseMode
  /** Optional browser-automation interaction script. Subtask 5 wires this. */
  script?: ScriptStep[]
  /** Free-form tags for filtering. */
  tags?: string[]
}

/** A single browser-automation interaction step. */
export type ScriptStep =
  | { action: 'fill'; selector: string; value: string }
  | { action: 'click'; selector: string }
  | { action: 'assert'; selector: string; text?: string; value?: unknown }
  | { action: 'wait'; ms: number }

/** Catalog entry returned from /test/api/cases. */
export interface CaseSummary {
  id: string
  category: string
  title: string
  description?: string
  url: string
  hasScript: boolean
}
