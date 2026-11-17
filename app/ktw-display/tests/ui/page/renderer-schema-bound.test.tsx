// SPDX-License-Identifier: Apache-2.0
// Smoke tests for the 4 schema-bound node renderers (S4 of
// display-dsl-framework). Each verifies inline schema delegation.
//
// Stubs the field-renderer lookup with a minimal factory so the
// builder pipelines can run end-to-end. Pattern mirrors
// `tests/ui-builders/form/visibility.test.ts`.

import { describe, expect, it } from 'bun:test'
import { renderToString } from 'react-dom/server'
import { MantineProvider } from '@mantine/core'

import {
  setRendererLookup,
  type FieldRendererFactory,
} from '../../../../ktw-display-lib/src/ui-builders/fields/index.ts'
import { UIPageView } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'
import type { UIPageNode } from '../../../../ktw-display-lib/src/ui-builders/page/index.ts'
import type { Schema } from '../../../../ktw-display-lib/src/schema/schema-types.ts'

// Stub the renderer lookup (eager — module-load time).
const stubFactory: FieldRendererFactory = {
  view: () => () => null,
  form: () => () => null,
  header: () => () => null,
  column: () => () => null,
}
setRendererLookup(() => stubFactory)

// =================================================================================================
// Schema fixture — minimal Book with id/title/author for all 4 surfaces
// =================================================================================================

const BOOK_SCHEMA: Schema = {
  name: 'book-test',
  root: 'Book',
  types: {
    Book: {
      type: 'object',
      idField: ['id'],
      titleField: ['title'],
      fields: {
        id: { type: 'string', label: 'ID' },
        title: { type: 'string', label: 'Title' },
        author: { type: 'string', label: 'Author' },
      },
    },
  },
}

const BOOK_LIST_SCHEMA: Schema = {
  name: 'book-list-test',
  root: 'BookList',
  types: {
    BookList: {
      type: 'array',
      items: 'Book',
    },
    Book: BOOK_SCHEMA.types.Book,
  },
}

// =================================================================================================
// Render helper
// =================================================================================================

function render(node: UIPageNode): string {
  return renderToString(
    <MantineProvider>
      <UIPageView node={node} />
    </MantineProvider>,
  )
}

// =================================================================================================
// Tests
// =================================================================================================

describe('schema-bound — form', () => {
  it('renders inline-schema form without crashing', () => {
    const html = render({
      kind: 'form',
      schema: BOOK_SCHEMA,
      mode: 'edit',
      data: { id: 'b1', title: 'Foundation', author: 'Asimov' },
    })
    expect(html.length).toBeGreaterThan(0)
    // Form renders without throwing — schema-bound delegation is inline
    // (no nested iframe), so the form's TSX is in the same DOM tree.
  })

  it('falls back to default form when formName missing', () => {
    const html = render({
      kind: 'form',
      schema: BOOK_SCHEMA,
      mode: 'create',
    })
    expect(html.length).toBeGreaterThan(0)
  })
})

describe('schema-bound — view', () => {
  it('renders inline-schema view with data', () => {
    const html = render({
      kind: 'view',
      schema: BOOK_SCHEMA,
      data: { id: 'b1', title: 'Foundation' },
    })
    expect(html.length).toBeGreaterThan(0)
  })

  it('respects emptyStrategy', () => {
    // Just verifies the prop forwards without crashing.
    const html = render({
      kind: 'view',
      schema: BOOK_SCHEMA,
      data: {},
      emptyStrategy: 'omit',
    })
    expect(html.length).toBeGreaterThan(0)
  })
})

describe('schema-bound — table', () => {
  it('renders inline-schema table with array data', () => {
    const html = render({
      kind: 'table',
      schema: BOOK_LIST_SCHEMA,
      data: [
        { id: 'b1', title: 'Foundation', author: 'Asimov' },
        { id: 'b2', title: 'Dune', author: 'Herbert' },
      ],
    })
    expect(html.length).toBeGreaterThan(0)
  })

  it('handles empty data', () => {
    const html = render({
      kind: 'table',
      schema: BOOK_LIST_SCHEMA,
      data: [],
    })
    expect(html.length).toBeGreaterThan(0)
  })

  it('wraps a single object in an array (defensive)', () => {
    const html = render({
      kind: 'table',
      schema: BOOK_LIST_SCHEMA,
      data: { id: 'b1', title: 'Foundation', author: 'Asimov' } as never,
    })
    expect(html.length).toBeGreaterThan(0)
  })
})

describe('schema-bound — table rows open and act', () => {
  const rows = [{ id: 'b1', title: 'Foundation', author: 'Asimov' }, { id: 'b2', title: 'Dune', author: 'Herbert' }]
  const node = {
    kind: 'table', schema: BOOK_LIST_SCHEMA, data: rows,
    open: { navigate: { object: '${item.id}' } },
    actions: [{ label: 'Lend', uri: 'ktw-demo:library/lend', args: { id: '${item.id}' } }],
  } as UIPageNode

  it('draws each row its actions in a last column, live with a host', () => {
    const html = renderToString(
      <MantineProvider><UIPageView node={node} onAction={() => {}} /></MantineProvider>,
    )
    expect((html.match(/>Lend</g) ?? []).length).toBe(2)
    expect(html).toContain('aria-label="Actions"')
    expect(html).toContain('<tr style="cursor:pointer"')
  })

  it('without a host the row actions show disabled and a row opens nothing', () => {
    const html = render(node)
    expect(html).toContain('disabled')
    expect(html).not.toContain('<tr style="cursor:pointer"')
  })

  it('hasSchemaBound finds a schema-bound node anywhere a host must resolve one', async () => {
    const { hasSchemaBound } = await import('../../../../ktw-display-lib/src/ui-builders/page/index.ts')
    expect(hasSchemaBound({ kind: 'vbox', children: [{ kind: 'tabs', tabs: [{ label: 'a', content: { kind: 'form', schema: 'x:/data-type/y' } }] }] })).toBe(true)
    expect(hasSchemaBound({ kind: 'vbox', children: [{ kind: 'list', item: { kind: 'text', content: '' }, data: [{ kind: 'form' }] }] })).toBe(false)
  })
})

describe('schema-bound — wizard', () => {
  it('renders inline-schema wizard without crashing', () => {
    const html = render({
      kind: 'wizard',
      schema: BOOK_SCHEMA,
      mode: 'create',
    })
    expect(html.length).toBeGreaterThan(0)
  })
})

describe('schema-bound — composition', () => {
  it('renders a vbox containing form + table inline', () => {
    const html = render({
      kind: 'vbox',
      gap: 'md',
      children: [
        { kind: 'heading', text: 'Library' },
        {
          kind: 'table',
          schema: BOOK_LIST_SCHEMA,
          data: [{ id: 'b1', title: 'Foundation', author: 'Asimov' }],
        },
        {
          kind: 'form',
          schema: BOOK_SCHEMA,
          mode: 'create',
        },
      ],
    })
    expect(html).toContain('Library')
    // Both schema-bound nodes inlined into the same DOM tree (no nested iframes).
    expect(html.length).toBeGreaterThan(0)
  })
})

describe('schema-bound — diagnostic paths', () => {
  it('a schema named by id and not resolved by the host shows a diagnostic', () => {
    const html = render({ kind: 'form', schema: 'ktw-demo:/data-type/book' })
    expect(html).toContain('not resolved')
    expect(html).toContain('ktw-demo:/data-type/book')
  })

  it('an unresolved ui block shows a diagnostic', () => {
    const html = render({ kind: 'view', schema: { name: 'b', root: 'B', types: { B: { type: 'object' } } }, ui: { type: 'ui-view' } } as never)
    expect(html).toContain('unresolved ui block')
  })

  it('a node without a schema shows a diagnostic', () => {
    const html = render({ kind: 'view' })
    expect(html).toContain('has no schema')
  })
})
