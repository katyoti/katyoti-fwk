// SPDX-License-Identifier: Apache-2.0
// Demo schema fixtures for the ui-form / ui-view / ui-table debug pages.
//
// The debug pages demonstrate the composition components by driving them
// through the REAL host injection path: a host-supplied schema (and value)
// passed via `schemaInline` / `valueInline`, base64url-encoded into the
// iframe URL — exactly what a production artifact mount does. There is no
// server-side "demo mode"; the bun `/widget/*` routes render whatever
// schema the host injects.
//
// These are deliberately rich (multiple field content types, multiple
// ui-* extension variants, view-only groups) so the debug surface
// exercises the renderers' full capability range.

/** Shared Book object type — bibliographic + reading-status + audit fields. */
const BOOK_TYPE = {
  type: 'object',
  label: 'Book',
  idField: ['id'],
  titleField: ['title'],
  fields: {
    id: { type: 'string', label: 'ID' },
    title: { type: 'string', label: 'Title', description: 'Title of the book.' },
    author_email: {
      type: 'string',
      label: 'Author email',
      content: { type: 'email' },
      optional: true,
    },
    notes: {
      type: 'string',
      label: 'Notes',
      content: { type: 'markdown', maxLength: 500 },
      optional: true,
    },
    rating: {
      type: 'number',
      label: 'Rating',
      content: { type: 'rating', max: 5 },
      default: 3,
    },
    progress_pct: {
      type: 'number',
      label: 'Progress (%)',
      content: { type: 'slider', min: 0, max: 100 },
      default: 0,
    },
    is_finished: { type: 'boolean', label: 'Finished?', default: false },
    created_at: {
      type: 'string',
      label: 'Created at',
      description: 'Audit field — view-only.',
    },
  },
}

/** Form schema: Book with two ui-form variants (default groups + compact single). */
export const FORM_DEMO_SCHEMA = {
  name: 'form-widget-demo',
  version: 1,
  root: 'Book',
  types: {
    Book: {
      ...BOOK_TYPE,
      extensions: [
        {
          type: 'ui-form',
          name: 'default',
          layout: 'groups',
          submit_label: 'Save book',
          cancel_label: 'Discard',
          groups: [
            { title: 'Bibliographic', cols: 2, fields: ['title', 'author_email'] },
            { title: 'Reading status', cols: 2, fields: ['rating', 'progress_pct', 'is_finished'] },
            { title: 'Notes', cols: 1, fields: ['notes'] },
            { title: 'Audit', visibility: ['view'], fields: ['id', 'created_at'] },
          ],
        },
        {
          type: 'ui-form',
          name: 'compact',
          layout: 'single',
          submit_label: 'Quick save',
          groups: [{ fields: ['title', 'rating', 'is_finished'] }],
        },
      ],
    },
  },
}

// Per-field detail levels (1=always, 2=medium, 3=detailed) so the view's
// L1/L2/L3 toggle visibly changes what's shown:
//   L1 → title, rating
//   L2 → + author_email, progress_pct, is_finished
//   L3 → + notes, id, created_at (audit)
// `actions: []` suppresses the default Close/Edit/Delete buttons — this is
// a read-only demonstration surface, not an editable record.
/** View schema: Book with two ui-view variants (detail-pane + inline). */
export const VIEW_DEMO_SCHEMA = {
  name: 'view-widget-demo',
  version: 1,
  root: 'Book',
  types: {
    Book: {
      ...BOOK_TYPE,
      extensions: [
        {
          type: 'ui-view',
          name: 'default',
          layout: 'detail-pane',
          actions: [],
          groups: [
            {
              title: 'Bibliographic',
              fields: [{ id: 'title', detail: 1 }, { id: 'author_email', detail: 2 }],
            },
            {
              title: 'Reading status',
              fields: [
                { id: 'rating', detail: 1 },
                { id: 'progress_pct', detail: 2 },
                { id: 'is_finished', detail: 2 },
              ],
            },
            { title: 'Notes', fields: [{ id: 'notes', detail: 3 }] },
            {
              title: 'Audit',
              visibility: ['view'],
              fields: [{ id: 'id', detail: 3 }, { id: 'created_at', detail: 3 }],
            },
          ],
        },
        {
          type: 'ui-view',
          name: 'compact',
          layout: 'inline',
          actions: [],
          groups: [{ fields: ['title', 'rating', 'is_finished'] }],
        },
      ],
    },
  },
}

/** Sample Book value — used as the view demo's value and the form demo's
 *  edit-mode initial value. */
export const SAMPLE_BOOK = {
  id: 'book-001',
  title: 'The Pragmatic Programmer',
  author_email: 'andy@example.com',
  notes: '# Thoughts\n\nClassic. Re-read every 5 years.',
  rating: 5,
  progress_pct: 100,
  is_finished: true,
  created_at: '2026-01-15T10:30:00Z',
}

/** Table schema: Library (Array<Book>) with ui-table columns + ui-filter. */
export const TABLE_DEMO_SCHEMA = {
  name: 'library-demo',
  version: 1,
  root: 'BookList',
  types: {
    BookList: {
      type: 'array',
      label: 'Library',
      items: 'Book',
      extensions: [
        {
          type: 'ui-table',
          name: 'default',
          columns: [
            { id: 'id', label: 'ID', width: 110 },
            { id: 'title', label: 'Title', sortable: true },
            { id: 'author_email', label: 'Author' },
            { id: 'rating', label: 'Rating', align: 'center', width: 80 },
            { id: 'progress_pct', label: 'Progress', align: 'right', width: 100 },
            { id: 'is_finished', label: 'Finished', align: 'center', width: 100 },
          ],
          filters: true,
          pagination: true,
          pageSize: 10,
          striped: true,
          highlightOnHover: true,
        },
        { type: 'ui-filter', filters: ['title', 'author_email'] },
      ],
    },
    Book: BOOK_TYPE,
  },
}

const TITLES = [
  'The Pragmatic Programmer', 'Clean Code', 'Designing Data-Intensive Applications',
  'Domain-Driven Design', 'Refactoring', 'Code Complete', 'The Mythical Man-Month',
  'Working Effectively with Legacy Code', 'Patterns of Enterprise Application Architecture',
  'Release It!', 'Site Reliability Engineering', 'Accelerate', 'The Phoenix Project',
  'Continuous Delivery', 'The DevOps Handbook', 'Building Microservices',
  'Hands-On Concurrency with Rust', 'Database Internals', 'High Performance Browser Networking',
  'Computer Networks', 'Operating Systems: Three Easy Pieces', 'Algorithms to Live By',
  'Structure and Interpretation of Computer Programs', 'Crafting Interpreters', 'Programming Rust',
]

const AUTHORS = [
  'thomas@example.com', 'martin@example.com', 'kleppmann@example.com', 'evans@example.com',
  'fowler@example.com', 'mcconnell@example.com', 'brooks@example.com',
  'feathers@example.com', 'fowler@example.com', 'nygard@example.com',
  'beyer@example.com', 'forsgren@example.com', 'kim@example.com', 'humble@example.com',
  'gene.kim@example.com', 'newman@example.com', 'mcnamara@example.com', 'petrov@example.com',
  'grigorik@example.com', 'tanenbaum@example.com', 'arpaci@example.com', 'christian@example.com',
  'abelson@example.com', 'nystrom@example.com', 'blandy@example.com',
]

/** 25 fabricated Book rows for the table demo. */
export const TABLE_DEMO_VALUE = TITLES.map((title, i) => ({
  id: `book-${String(i + 1).padStart(3, '0')}`,
  title,
  author_email: AUTHORS[i],
  rating: 1 + ((i * 7) % 5),
  progress_pct: i % 3 === 0 ? 100 : (i * 11) % 100,
  is_finished: i % 3 === 0,
  notes: i % 5 === 0 ? `Re-read book #${i + 1} every 5 years.` : '',
}))

/** 3-step onboarding schema for the UIWizard acceptance demo. Injected
 *  host-side via `schemaInline` — the production `/widget/wizard` route
 *  carries no demo data of its own. */
export const WIZARD_DEMO_SCHEMA = {
  name: 'onboarding-wizard-demo',
  version: 1,
  root: 'OnboardingProfile',
  types: {
    OnboardingProfile: {
      type: 'object',
      label: 'Onboarding profile',
      idField: ['email'],
      titleField: ['name'],
      fields: {
        name: { type: 'string', label: 'Full name' },
        email: { type: 'string', label: 'Email', content: { type: 'email' } },
        role: { type: 'string', label: 'Role' },
        team: { type: 'string', label: 'Team' },
        theme: { type: 'string', label: 'Theme', default: 'light' },
        notify_email: { type: 'boolean', label: 'Email notifications', default: true },
        notify_inapp: { type: 'boolean', label: 'In-app notifications', default: true },
      },
      extensions: [
        {
          type: 'ui-wizard',
          name: 'default',
          navigation: 'linear',
          steps: [
            {
              id: 'identity',
              title: 'Who are you?',
              description: 'The basics — name and how to reach you.',
              fields: ['name', 'email'],
            },
            {
              id: 'role',
              title: 'Your role',
              description: 'Tell us about your work.',
              fields: ['role', 'team'],
            },
            {
              id: 'preferences',
              title: 'Preferences',
              description: 'Set how you want to be notified.',
              fields: ['theme', 'notify_email', 'notify_inapp'],
            },
          ],
        },
      ],
    },
  },
}

// =================================================================================================
// Tree-table demo — a hierarchical node set (parent_id linked) with a `state`
// column, rendered via the `ui-tree-table` extension. Exercises nested indented
// rows + expand/collapse + status badges (ov1).
// =================================================================================================

const TREE_NODE_TYPE = {
  type: 'object',
  label: 'Node',
  idField: ['id'],
  titleField: ['title'],
  fields: {
    id: { type: 'string', label: 'ID' },
    parent_id: { type: 'string', label: 'Parent', optional: true },
    title: { type: 'string', label: 'Page' },
    state: { type: 'string', label: 'State' },
  },
}

export const TREE_TABLE_DEMO_SCHEMA = {
  name: 'tree-table-demo',
  version: 1,
  root: 'TreeNodes',
  types: {
    TreeNodes: {
      type: 'array',
      label: 'Sync tree',
      items: 'TreeNode',
      extensions: [
        {
          type: 'ui-tree-table',
          name: 'default',
          columns: [
            { id: 'title', label: 'Page' },
            { id: 'state', label: 'State', align: 'center', width: 140 },
          ],
          idField: 'id',
          parentField: 'parent_id',
          stateField: 'state',
          defaultExpanded: true,
          indentPx: 22,
          highlightOnHover: true,
        },
      ],
    },
    TreeNode: TREE_NODE_TYPE,
  },
}

export const TREE_TABLE_DEMO_VALUE = [
  { id: '1', parent_id: '', title: 'Espace Confluence AUP', state: 'done' },
  { id: '2', parent_id: '1', title: 'Architecture de données', state: 'done' },
  { id: '3', parent_id: '2', title: 'STELLAR — Dynatrace', state: 'processing' },
  { id: '4', parent_id: '2', title: 'COSMOS & Co', state: 'download' },
  { id: '5', parent_id: '1', title: 'Diagrammes', state: 'pending' },
  { id: '6', parent_id: '5', title: 'Layout examples', state: 'discovered' },
  { id: '7', parent_id: '5', title: 'Panneaux et sections', state: 'failed' },
  { id: '8', parent_id: '1', title: 'Navigation et liens', state: 'skip' },
]

// =================================================================================================
// View-with-tree demo — a `SyncRun` OBJECT rendered as a ui-view whose `nodes`
// field (an array-of-objects) renders as a TREE-TABLE via `content:{type:
// 'tree-table'}`. Proves the recursive rich-field goal.
// =================================================================================================

export const VIEW_TREE_DEMO_SCHEMA = {
  name: 'sync-run-demo',
  version: 1,
  root: 'SyncRun',
  types: {
    SyncRun: {
      type: 'object',
      label: 'Sync run',
      idField: ['workspace'],
      titleField: ['workspace'],
      fields: {
        workspace: { type: 'string', label: 'Workspace' },
        verb: { type: 'string', label: 'Verb' },
        pages_total: { type: 'number', label: 'Pages discovered' },
        succeeded: { type: 'number', label: 'Succeeded' },
        failed: { type: 'number', label: 'Failed' },
        nodes: {
          type: 'array',
          label: 'Pages',
          items: 'TreeNode',
          // Array-of-objects → tree-table. `extension` is an inline ui-tree-table
          // spec (could instead be a string naming an extension on TreeNode).
          content: {
            type: 'tree',
            extension: {
              parentField: 'parent_id',
              stateField: 'state',
              idField: 'id',
              columns: [
                { id: 'title', label: 'Page' },
                { id: 'state', label: 'State', align: 'center', width: 140 },
              ],
            },
          },
        },
        // Same rows, `content:{type:'table'}` → FLAT table (no hierarchy) — shows
        // the table content-type alongside the tree in one view.
        flat_pages: {
          type: 'array',
          label: 'Pages (flat table)',
          items: 'TreeNode',
          content: {
            type: 'table',
            extension: {
              columns: [
                { id: 'id', label: 'ID', width: 70 },
                { id: 'title', label: 'Page', sortable: true },
                { id: 'state', label: 'State', align: 'center', width: 140 },
              ],
              striped: true,
              highlightOnHover: true,
            },
          },
        },
      },
      extensions: [
        {
          type: 'ui-view',
          name: 'default',
          layout: 'card',
          groups: [
            { id: 'summary', label: 'Summary', cols: 3, fields: ['workspace', 'verb', 'pages_total', 'succeeded', 'failed'] },
            { id: 'tree', label: 'Page tree (tree-table)', fields: ['nodes'] },
            { id: 'flat', label: 'Pages (flat table)', fields: ['flat_pages'] },
          ],
        },
      ],
    },
    TreeNode: TREE_NODE_TYPE,
  },
}

export const VIEW_TREE_DEMO_VALUE = {
  workspace: 'AUP — Architecture',
  verb: 'pull',
  pages_total: 8,
  succeeded: 6,
  failed: 1,
  nodes: TREE_TABLE_DEMO_VALUE,
  flat_pages: TREE_TABLE_DEMO_VALUE,
}

// =================================================================================================
// Confluence status — the sync-status table-tree matching the `confluence status`
// TUI: Local (✓/✎ + version) · direction arrows · Remote (✓ + version) · Page
// (tree) · Status. Columns use the tree-table `kind` renderers (version /
// direction / state); the tree indent rides the `title` column (treeColumn).
// =================================================================================================

const STATUS_NODE_TYPE = {
  type: 'object',
  label: 'Page',
  idField: ['id'],
  titleField: ['title'],
  fields: {
    id: { type: 'string', label: 'ID' },
    parent_id: { type: 'string', label: 'Parent' },
    title: { type: 'string', label: 'Page' },
    local_version: { type: 'string', label: 'Local' },
    local_state: { type: 'string', label: 'Local state' },
    direction: { type: 'string', label: '' },
    remote_version: { type: 'string', label: 'Remote' },
    remote_state: { type: 'string', label: 'Remote state' },
    status: { type: 'string', label: 'Status' },
  },
}

export const CONFLUENCE_STATUS_SCHEMA = {
  name: 'confluence-status',
  version: 1,
  root: 'ConfluenceStatus',
  types: {
    ConfluenceStatus: {
      type: 'object',
      label: 'Confluence status',
      idField: ['summary'],
      titleField: ['summary'],
      fields: {
        summary: { type: 'string', label: 'Summary' },
        pages: {
          type: 'array',
          label: 'Pages',
          items: 'StatusNode',
          content: {
            type: 'tree',
            extension: {
              idField: 'id',
              parentField: 'parent_id',
              stateField: 'status',
              treeColumn: 'title',
              indentPx: 22,
              columns: [
                { id: 'local_version', label: 'Local', kind: 'version', stateField: 'local_state', align: 'center', width: 90 },
                { id: 'direction', label: '', kind: 'direction', align: 'center', width: 56 },
                { id: 'remote_version', label: 'Remote', kind: 'version', stateField: 'remote_state', align: 'center', width: 90 },
                { id: 'title', label: 'Page' },
                { id: 'status', label: 'Status' },
              ],
            },
          },
        },
      },
      extensions: [
        {
          type: 'ui-view',
          name: 'default',
          layout: 'card',
          groups: [
            { id: 'sum', label: 'Status', fields: ['summary'] },
            { id: 'pages', label: 'Pages', fields: ['pages'] },
          ],
        },
      ],
    },
    StatusNode: STATUS_NODE_TYPE,
  },
}

const uptodate = (v: string) => ({ local_version: v, local_state: 'clean', direction: 'skip', remote_version: v, remote_state: 'clean', status: 'up to date' })

export const CONFLUENCE_STATUS_VALUE = {
  summary: '18 page(s): 14 skip · 4 upload',
  pages: [
    { id: 'p1', parent_id: '', title: 'ERP Acme — Vues synthétiques', local_version: 'v2', local_state: 'updated', direction: 'upload', remote_version: 'v2', remote_state: 'clean', status: 'local updated' },
    { id: 'p2', parent_id: '', title: 'ERP Acme — Solutions', ...uptodate('v1') },
    { id: 'p3', parent_id: '', title: 'ERP Acme — Odoo', ...uptodate('v1') },
    { id: 'p3a', parent_id: 'p3', title: 'ERP Acme — Odoo — Produit', ...uptodate('v2') },
    { id: 'p3b', parent_id: 'p3', title: 'ERP Acme — ODOO S.A. — Éditeur', ...uptodate('v2') },
    { id: 'p3c', parent_id: 'p3', title: 'ERP Acme — Odoo — Sources', ...uptodate('v2') },
    { id: 'p6', parent_id: '', title: 'ERP Acme — MI5 Pilot', ...uptodate('v2') },
    { id: 'p6a', parent_id: 'p6', title: 'ERP Acme — MI5 Pilot — Produit', ...uptodate('v1') },
    { id: 'p6b', parent_id: 'p6', title: 'ERP Acme — MI5 Pilot — Sources', ...uptodate('v1') },
    { id: 'p6c', parent_id: 'p6', title: 'ERP Acme — MI5 Automotive — Éditeur', ...uptodate('v1') },
  ],
}
