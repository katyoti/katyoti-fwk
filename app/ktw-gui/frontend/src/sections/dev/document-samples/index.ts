// SPDX-License-Identifier: Apache-2.0
// The sample documents of the Documents › display engines gallery — one list
// per engine id. A ui-component names its schema inline or by data-type id; an
// id (and a `ui` block) is resolved in Go before display, so the "by id"
// samples need a displayed universe that hosts ktw-demo.
import type { Document } from '../../../components/document/engines'
import {
  CONFLUENCE_STATUS_SCHEMA, CONFLUENCE_STATUS_VALUE, FORM_DEMO_SCHEMA, SAMPLE_BOOK, TABLE_DEMO_SCHEMA,
  TABLE_DEMO_VALUE, TREE_TABLE_DEMO_SCHEMA, TREE_TABLE_DEMO_VALUE, VIEW_DEMO_SCHEMA, VIEW_TREE_DEMO_SCHEMA,
  VIEW_TREE_DEMO_VALUE, WIZARD_DEMO_SCHEMA,
} from '../demo-schemas'
import { CHART_DEMO_DATA, CHART_DEMO_TREE, MEDIA_TIMELINE_TREE, PRIMITIVES_SCHEMA, SIMPLE_DRAWIO } from './fixtures'
import DASHBOARD_HTML from './dashboard.html?raw'
import C4_DRAWIO from './c4-internet-banking.drawio?raw'
import DOSSIER_DRAWIO from './dossier-frame.drawio?raw'
// The slidedeck module's own demo deck, read where it lives.
import SYSTEM_DEMO_DECK from '../../../../../../../modules/slidedeck/fixtures/system-demo.deck.md?raw'
import { MEDIA_TYPE_DECK } from '../../../generated/media-types'

const MARKDOWN = `# Library report

The library lends **books** to _members_; this report shows how a loan flows.

## Loans this month

| Branch | Loans | Late |
|---|---|---|
| Centre | 412 | 9 |
| North | 188 | 3 |
| Harbour | 97 | 1 |

## How a loan works

\`\`\`mermaid
flowchart LR
  M[Member] -->|asks| D{Available?}
  D -- yes --> L[Loan 21 days]
  D -- no --> R[Reservation]
  L --> B[Returned]
\`\`\`

## Who talks to whom

\`\`\`mermaid
sequenceDiagram
  Member->>Desk: borrow(book)
  Desk->>Catalog: reserve(book)
  Catalog-->>Desk: ok
  Desk-->>Member: due date
\`\`\`

> Late loans are down 30% since reminders moved to e-mail.

\`\`\`go
func dueDate(t time.Time) time.Time { return t.AddDate(0, 0, 21) }
\`\`\`
`

const MERMAID = `flowchart TD
  subgraph Desk
    A[Scan card] --> B{Book on shelf?}
  end
  subgraph Catalog
    B -- no --> C[Reserve]
    C --> D[Notify member]
  end
  B -- yes --> E[Issue loan]`

const GO = `package library

import "time"

// Loan is one book lent to one member.
type Loan struct {
	Book, Member string
	Due          time.Time
}

// Late reports whether the loan is past its due date.
func (l Loan) Late(now time.Time) bool { return now.After(l.Due) }
`

const SVG = `<svg xmlns="http://www.w3.org/2000/svg" width="240" height="120" viewBox="0 0 240 120">
<rect width="240" height="120" rx="12" fill="#228be6"/><text x="120" y="68" font-family="sans-serif" font-size="20" fill="white" text-anchor="middle">image engine</text></svg>`

const PDF_BASE64 =
  'JVBERi0xLjQKMSAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMiAwIFIgPj4KZW5kb2JqCjIgMCBvYmoKPDwgL1R5cGUgL1BhZ2VzIC9LaWRzIFszIDAgUl0gL0NvdW50IDEgPj4KZW5kb2JqCjMgMCBvYmoKPDwgL1R5cGUgL1BhZ2UgL1BhcmVudCAyIDAgUiAvTWVkaWFCb3ggWzAgMCA0MjAgMjAwXSAvQ29udGVudHMgNCAwIFIgL1Jlc291cmNlcyA8PCAvRm9udCA8PCAvRjEgNSAwIFIgPj4gPj4gPj4KZW5kb2JqCjQgMCBvYmoKPDwgL0xlbmd0aCA2MiA+PgpzdHJlYW0KQlQgL0YxIDIwIFRmIDQwIDExMCBUZCAoU2FtcGxlIFBERiAtIHBkZiBkaXNwbGF5IGVuZ2luZSkgVGogRVQKZW5kc3RyZWFtCmVuZG9iago1IDAgb2JqCjw8IC9UeXBlIC9Gb250IC9TdWJ0eXBlIC9UeXBlMSAvQmFzZUZvbnQgL0hlbHZldGljYSA+PgplbmRvYmoKeHJlZgowIDYKMDAwMDAwMDAwMCA2NTUzNSBmIAowMDAwMDAwMDA5IDAwMDAwIG4gCjAwMDAwMDAwNTggMDAwMDAgbiAKMDAwMDAwMDExNSAwMDAwMCBuIAowMDAwMDAwMjQxIDAwMDAwIG4gCjAwMDAwMDAzNTMgMDAwMDAgbiAKdHJhaWxlcgo8PCAvU2l6ZSA2IC9Sb290IDEgMCBSID4+CnN0YXJ0eHJlZgo0MjMKJSVFT0YK'


const UI = 'application/vnd.ktw.ui+json'
const DRAWIO = 'application/vnd.jgraph.mxfile'
const BOOK_ID = 'ktw-demo:/data-type/book'

export const SAMPLES: Record<string, Document[]> = {
  markdown: [{ mediaType: 'text/markdown', title: 'library-report.md — tables, code, two mermaid diagrams', content: MARKDOWN }],
  html: [{ mediaType: 'text/html', title: 'operations dashboard — KPIs, charts, sortable table (interactive)', content: DASHBOARD_HTML }],
  mermaid: [{ mediaType: 'text/vnd.mermaid', title: 'loan-flow.mmd', content: MERMAID }],
  text: [{ mediaType: 'text/x-go', title: 'loan.go', content: GO }],
  'ui-components': [
    { mediaType: UI, title: 'view — inline schema', content: { kind: 'view', schema: VIEW_DEMO_SCHEMA, data: SAMPLE_BOOK, mode: 'view' } },
    { mediaType: UI, title: 'form — inline schema', content: { kind: 'form', schema: FORM_DEMO_SCHEMA, data: SAMPLE_BOOK, mode: 'edit' } },
    { mediaType: UI, title: 'table — inline schema (click a row)', content: { kind: 'table', schema: TABLE_DEMO_SCHEMA, data: TABLE_DEMO_VALUE } },
    { mediaType: UI, title: 'tree-table — nested rows and status badges', content: { kind: 'tree-table', schema: TREE_TABLE_DEMO_SCHEMA, data: TREE_TABLE_DEMO_VALUE } },
    { mediaType: UI, title: 'view — a field rendered as an embedded tree', content: { kind: 'view', schema: VIEW_TREE_DEMO_SCHEMA, data: VIEW_TREE_DEMO_VALUE } },
    { mediaType: UI, title: 'view — Confluence sync status', content: { kind: 'view', schema: CONFLUENCE_STATUS_SCHEMA, data: CONFLUENCE_STATUS_VALUE } },
    { mediaType: UI, title: 'wizard — multi-step (submit it)', content: { kind: 'wizard', schema: WIZARD_DEMO_SCHEMA, mode: 'create' } },
    {
      // Both nodes bound to $.record: the view follows the form as you type.
      mediaType: UI, title: 'page — every field renderer, form and view in sync',
      content: {
        kind: 'page',
        data: { record: {} },
        dslTree: { kind: 'vbox', gap: 'lg', children: [
          { kind: 'heading', level: 3, text: 'Form mode' },
          { kind: 'form', schema: PRIMITIVES_SCHEMA, mode: 'edit', dataField: '$.record' },
          { kind: 'heading', level: 3, text: 'View mode — follows the form' },
          { kind: 'view', schema: PRIMITIVES_SCHEMA, dataField: '$.record' },
        ] },
      },
    },
    { mediaType: UI, title: 'page — charts (line, bar, area, pie, donut)', content: { kind: 'page', dslTree: CHART_DEMO_TREE, data: CHART_DEMO_DATA } },
    { mediaType: UI, title: 'page — video, audio, file, timeline', content: { kind: 'page', dslTree: MEDIA_TIMELINE_TREE } },
    {
      mediaType: UI, title: 'page — KPIs over a view by id',
      content: {
        kind: 'page',
        data: { loans: 697, late: 13, book: SAMPLE_BOOK },
        dslTree: {
          kind: 'vbox', gap: 'md', children: [
            { kind: 'hbox', gap: 'md', children: [
              { kind: 'kpi', label: 'Loans', dataField: '$.loans' },
              { kind: 'kpi', label: 'Late', dataField: '$.late' },
            ] },
            { kind: 'view', schema: BOOK_ID, dataField: '$.book' },
          ],
        },
      },
    },
    { mediaType: UI, title: 'view — schema by id', content: { kind: 'view', schema: BOOK_ID, data: SAMPLE_BOOK } },
    { mediaType: UI, title: 'form — by id, variant compact', content: { kind: 'form', schema: BOOK_ID, variant: 'compact', data: SAMPLE_BOOK, mode: 'edit' } },
    {
      mediaType: UI, title: 'view — by id with an inline ui block',
      content: {
        kind: 'view', schema: BOOK_ID, data: SAMPLE_BOOK,
        ui: { name: 'summary', groups: [{ title: 'Summary', cols: 2, fields: ['title', 'rating'] }] },
      },
    },
  ],
  drawio: [
    { mediaType: DRAWIO, title: 'c4-internet-banking.drawio — C4 container view (replay it as a stream)', content: C4_DRAWIO },
    { mediaType: DRAWIO, title: 'dossier-frame.drawio — dossier frame, 49 cells', content: DOSSIER_DRAWIO },
    { mediaType: DRAWIO, title: SIMPLE_DRAWIO.filename, content: SIMPLE_DRAWIO.xml },
  ],
  deck: [{ mediaType: MEDIA_TYPE_DECK, title: 'system-demo.deck.md — the slidedeck module presented with itself', content: SYSTEM_DEMO_DECK }],
  image: [{ mediaType: 'image/svg+xml', title: 'badge.svg', content: SVG }],
  pdf: [{ mediaType: 'application/pdf', title: 'sample.pdf', content: PDF_BASE64, encoding: 'base64' }],
  download: [{ mediaType: 'application/octet-stream', title: 'archive.bin', content: 'AAECAwQ=', encoding: 'base64' }],
}
