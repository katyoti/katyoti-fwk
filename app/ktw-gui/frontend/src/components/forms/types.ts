// SPDX-License-Identifier: Apache-2.0
/**
 * Form definition types — mirrors the Go formdef package.
 * These are the shapes the Go backend produces from FormDefinition YAML.
 */

export interface FormSchema {
  jsonSchema: JsonSchema
  uiSchema: UISchema
  data: Record<string, any>
  title: string
}

// ── JSON Schema (subset relevant to forms) ──────────────────────────────

export interface JsonSchema {
  type: string
  properties?: Record<string, JsonSchemaProperty>
  required?: string[]
  title?: string
  description?: string
}

export interface JsonSchemaProperty {
  type?: string
  title?: string
  description?: string
  default?: any
  readOnly?: boolean
  format?: string // date, time, date-time, email, uri, etc.
  minLength?: number
  maxLength?: number
  minimum?: number
  maximum?: number
  pattern?: string
  oneOf?: Array<{ const: string; title: string }> // enum options
  // Nested
  properties?: Record<string, JsonSchemaProperty>
  required?: string[]
  items?: JsonSchemaProperty
}

// ── UI Schema (JSON Forms-compatible layout) ─────────────────────────────

export type UISchema = UISchemaElement

export type UISchemaElement =
  | UISchemaControl
  | UISchemaLayout
  | UISchemaCategorization
  | UISchemaCategory

export interface UISchemaControl {
  type: 'Control'
  scope: string // e.g. "#/properties/name"
  label?: string
  options?: Record<string, any> // multi, format, optionDetails, etc.
}

export interface UISchemaLayout {
  type: 'VerticalLayout' | 'HorizontalLayout' | 'Group'
  label?: string
  elements: UISchemaElement[]
}

export interface UISchemaCategorization {
  type: 'Categorization'
  elements: UISchemaCategory[]
}

export interface UISchemaCategory {
  type: 'Category'
  label: string
  elements: UISchemaElement[]
}
