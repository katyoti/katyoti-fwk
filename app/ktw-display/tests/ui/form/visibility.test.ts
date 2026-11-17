// SPDX-License-Identifier: Apache-2.0
// Visibility migration regression test — pins the negative→positive
// migration of the `notNew`/`notEdit` predicates to positive
// `visibility?: SchemaFieldVisibility[]` filtering applied during gui-form
// Subtask 3 (2026-05-03).
//
// Field-side AND group-side visibility are exercised against
// `buildFormGroups()` (the @internal filter-pipeline entry point exposed
// for testability). The test stubs the field-renderer registry with a
// no-op factory and bypasses Mantine rendering entirely — pure builder
// semantics are under test.
//
// CRUD-mode mapping (locked in `ui-form.builder.ts`):
//   - isNew=true  → 'create' token
//   - isNew=false → 'update' token
//   - 'view'/'filter' tokens are honored only on read-only surfaces; a form
//     never matches them, so a group/field with `visibility: ['view']` is
//     hidden in BOTH form modes.
//   - Empty/absent `visibility` arrays mean "show in every mode".

import { describe, expect, it } from 'bun:test'

import type { ObjectType, Schema } from '../../../../ktw-display-lib/src/schema/schema-types.ts'
import {
  setRendererLookup,
  type FieldRendererFactory,
} from '../../../../ktw-display-lib/src/ui-builders/fields/index.ts'
import {
  buildFormGroups,
  buildUIForms,
  toUIFormDef,
} from '../../../../ktw-display-lib/src/ui-builders/form/index.ts'
import type { UIFormDef } from '../../../../ktw-display-lib/src/ui-builders/form/index.ts'
import type { UITexts } from '../../../../ktw-display-lib/src/ui-core/i18n/index.ts'

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

const stubTexts: UITexts = {
  t: (key: string, options?: Record<string, unknown>) =>
    (options?.defaultValue as string | undefined) ?? key,
  language: 'en',
  withNamespace: () => stubTexts,
}

const stubFactory: FieldRendererFactory = {
  view: () => () => null,
  form: () => () => null,
  header: () => () => null,
  column: () => () => null,
}

// Wire at module-load time — `buildFormGroups` calls in the `describe`
// blocks below evaluate eagerly (before any `beforeAll` would run), so
// the renderer lookup MUST be active by then.
setRendererLookup(() => stubFactory)

// ---------------------------------------------------------------------------
// Schema fixture — Demo with two named ui-form extensions exercising both
// group-level and field-level visibility filtering
// ---------------------------------------------------------------------------

const SCHEMA: Schema = {
  name: 'visibility-test',
  root: 'Demo',
  types: {
    Demo: {
      type: 'object',
      idField: ['id'],
      titleField: ['title'],
      fields: {
        id: { type: 'string', label: 'ID' },
        title: { type: 'string', label: 'Title' },
        slug: { type: 'string', label: 'Slug' },
        created_at: { type: 'string', label: 'Created' },
      },
      extensions: [
        {
          type: 'ui-form',
          name: 'with-visibility',
          groups: [
            {
              title: 'Identity',
              visibility: ['create', 'update'],
              fields: ['id', 'title'],
            },
            {
              title: 'Slug',
              visibility: ['update'],
              fields: ['slug'],
            },
            {
              title: 'Audit',
              visibility: ['view'],
              fields: ['created_at'],
            },
            {
              title: 'Always',
              fields: ['title'],
            },
          ],
        } as UIFormDef,
        {
          type: 'ui-form',
          name: 'compact',
          groups: [
            {
              fields: [
                'title',
                { id: 'slug', visibility: ['update'] },
                { id: 'created_at', visibility: ['create'] },
              ],
            },
          ],
        } as UIFormDef,
      ],
    },
  },
}

// ---------------------------------------------------------------------------

describe('ui-form builder — visibility migration', () => {
  describe('toUIFormDef — extension lookup + fallback', () => {
    it('returns { type: "ui-form" } when no extension matches', () => {
      const def = toUIFormDef(SCHEMA)
      expect(def).toEqual({ type: 'ui-form' })
    })

    it('returns the named extension when its name matches', () => {
      const def = toUIFormDef(SCHEMA, 'with-visibility')
      expect(def.name).toBe('with-visibility')
      expect(def.groups?.length).toBe(4)
    })

    it('passes a UIFormDef object straight through', () => {
      const inline = { type: 'ui-form' as const, name: 'inline' }
      const def = toUIFormDef(SCHEMA, inline)
      expect(def).toBe(inline)
    })
  })

  describe('group-level visibility — getGroupsToRender(isNew, item)', () => {
    const def = toUIFormDef(SCHEMA, 'with-visibility')
    const groups = buildFormGroups(SCHEMA, def, stubTexts, [])

    it('shows groups with [create, update] in BOTH modes', () => {
      expect(groupTitles(groups, true)).toContain('Identity')
      expect(groupTitles(groups, false)).toContain('Identity')
    })

    it('shows [update]-only groups only when isNew=false', () => {
      expect(groupTitles(groups, true)).not.toContain('Slug')
      expect(groupTitles(groups, false)).toContain('Slug')
    })

    it('hides [view]-only groups in BOTH form modes', () => {
      expect(groupTitles(groups, true)).not.toContain('Audit')
      expect(groupTitles(groups, false)).not.toContain('Audit')
    })

    it('shows groups with no visibility array in BOTH modes', () => {
      expect(groupTitles(groups, true)).toContain('Always')
      expect(groupTitles(groups, false)).toContain('Always')
    })

    it('produces the right group count per mode', () => {
      // create: Identity + Always = 2
      expect(groupTitles(groups, true).length).toBe(2)
      // update: Identity + Slug + Always = 3
      expect(groupTitles(groups, false).length).toBe(3)
    })
  })

  describe('field-level visibility — getFieldsToRender(isNew, item)', () => {
    const def = toUIFormDef(SCHEMA, 'compact')
    const groups = buildFormGroups(SCHEMA, def, stubTexts, [])

    // The compact form has a single auto-built group renderer; we read
    // back the inner `getFieldsToRender` callback by re-running
    // buildFormGroups (the layout factory normally captures it). Since
    // we don't have a layout-side hook, we re-derive against the
    // group's UIFormFields by calling the group renderer with a stub
    // — but easier: walk the renderer count via a direct re-call. The
    // `groups[0].renderer` is a React function; instead we count the
    // fields by re-invoking the inner callback the layout would call.
    //
    // The cleanest path is to re-derive the fields callback from the
    // form definition + the schema, but that re-implements the filter.
    // So instead we inspect the group's `getGroupsToRender` output —
    // each entry is a UIFormGroup whose `.renderer(props)` invokes
    // the inner field callback. We can't introspect through the React
    // closure cheaply.
    //
    // Approach: invoke `getGroupsToRender(isNew, item)` to confirm the
    // group exists, then directly call `buildFormGroups` semantics by
    // probing field counts via the same path: a mini-render into a
    // collector array. Since the group renderer is a React component
    // returning JSX (just nested element objects), we count children.
    //
    // Pragmatic compromise: call the renderer with a stub props bag
    // and count the rendered child elements. JSX is plain objects in
    // bun-test (no React DOM needed).

    it('field with [update] is hidden in create mode', () => {
      const rendered = groups.getGroupsToRender(true, {})
      expect(rendered.length).toBe(1)
      const fieldCount = countRenderedFields(rendered[0].renderer({ isNew: true, isSubmitting: false, item: {} }))
      // create mode: title (always) + created_at ([create]); slug ([update]) hidden
      expect(fieldCount).toBe(2)
    })

    it('field with [create] is hidden in update mode', () => {
      const rendered = groups.getGroupsToRender(false, {})
      expect(rendered.length).toBe(1)
      const fieldCount = countRenderedFields(rendered[0].renderer({ isNew: false, isSubmitting: false, item: {} }))
      // update mode: title + slug; created_at hidden
      expect(fieldCount).toBe(2)
    })
  })

  describe('buildUIForms — multi-form discovery', () => {
    it('keys forms by extension name', () => {
      const forms = buildUIForms({ schema: SCHEMA, texts: stubTexts })
      expect(Object.keys(forms).sort()).toEqual(['compact', 'default', 'with-visibility'])
    })

    it('synthesizes a "default" entry when no extension claims it', () => {
      const forms = buildUIForms({ schema: SCHEMA, texts: stubTexts })
      // "default" is auto-built from { type: 'ui-form' } — produces a
      // single auto-group containing every field
      expect(forms['default']).toBeDefined()
      expect(forms['default'].definition.groups).toBeUndefined()
    })

    it('does NOT clobber an explicitly-named "default" form', () => {
      // A schema with an explicit `name: 'default'` extension keeps its
      // groups (synthesis only fires when forms['default'] is missing).
      const demo = SCHEMA.types.Demo as ObjectType
      const schemaWithDefault: Schema = {
        ...SCHEMA,
        types: {
          Demo: {
            type: 'object',
            idField: ['id'],
            titleField: ['title'],
            fields: demo.fields,
            extensions: [
              {
                type: 'ui-form',
                name: 'default',
                groups: [{ title: 'Custom', fields: ['title'] }],
              } as UIFormDef,
            ],
          },
        },
      }
      const forms = buildUIForms({ schema: schemaWithDefault, texts: stubTexts })
      expect(forms['default'].definition.groups?.[0].title).toBe('Custom')
    })
  })
})

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Resolve the rendered group titles for a given mode by hitting the
 *  builder's actual `getGroupsToRender` callback — NOT a re-implementation. */
function groupTitles(groups: ReturnType<typeof buildFormGroups>, isNew: boolean): string[] {
  return groups
    .getGroupsToRender(isNew, {})
    .map((g) => g.title ?? '<untitled>')
}

/** Count the field-level children inside a rendered group. The group
 *  renderer is the Mantine layout from `fol_group_layout`; it returns JSX
 *  whose children include one `<Box>` per filtered field. We can't run
 *  React DOM in bun test, but JSX is plain objects, so we walk the tree
 *  and count nodes that look like `FieldRendererWrapper` children.
 *
 *  This is brittle to layout changes — if `fol_group_layout` swaps
 *  Mantine SimpleGrid for a different parent, the walk needs updating.
 *  Tradeoff is acceptable for now: the visibility migration is the test
 *  target, not the visual layout. */
function countRenderedFields(node: unknown): number {
  if (!node || typeof node !== 'object') return 0
  // The Paper element has children; walk until we find SimpleGrid which
  // has one child per field (each field is wrapped in a Box).
  const visit = (n: unknown): number => {
    if (!n || typeof n !== 'object') return 0
    const props = (n as { props?: { children?: unknown; grid?: unknown } }).props
    if (!props) return 0
    // A group section (FormGroupSection) carries its field grid in a
    // `grid` prop, not as children.
    const { children = props.grid } = props
    if (Array.isArray(children)) {
      // A SimpleGrid has one child per field; we recognize it by being
      // an array whose entries are themselves React-elements with keys
      // matching `field-N`. Anything else, recurse.
      const isFieldArray = children.every(
        (c: unknown) =>
          !!c &&
          typeof c === 'object' &&
          typeof (c as { key?: unknown }).key === 'string' &&
          ((c as { key: string }).key as string).startsWith('field-'),
      )
      if (isFieldArray) return children.length
      return children.reduce<number>((sum, c) => sum + visit(c), 0)
    }
    if (children) return visit(children)
    return 0
  }
  return visit(node)
}
