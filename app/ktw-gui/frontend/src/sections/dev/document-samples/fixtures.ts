// SPDX-License-Identifier: Apache-2.0
// Sample data shown by the Documents › display engines gallery: the chart
// and media pages, the field-renderer coverage schema, a small draw.io
// diagram. Moved here from the per-kind debug pages the gallery replaced.
import type { Schema } from '@ktw/display-lib/schema'

const SAMPLE_VIDEO_MP4 = 'https://videos.pexels.com/video-files/29689685/12768616_2560_1440_24fps.mp4'
const SAMPLE_AUDIO_MP3 = 'https://www.w3schools.com/html/horse.mp3'
const SAMPLE_IMAGE     = 'https://images.unsplash.com/photo-1512314889357-e157c22f938d?q=80&w=1171&auto=format&fit=crop&ixlib=rb-4.1.0&ixid=M3wxMjA3fDB8MHxwaG90by1wYWdlfHx8fGVufDB8fHx8fA%3D%3D'
const SAMPLE_FILE_PDF  = 'https://file-examples.com/storage/fe77018b346a0bca3969a38/2017/10/file-example_PDF_500_kB.pdf'

export const CHART_DEMO_TREE = {
  kind: 'vbox',
  gap: 'md',
  children: [
    { kind: 'heading', text: 'DSL chart node — visual smoke test', level: 3 },
    {
      kind: 'text',
      content: 'All 5 chartKind variants in one tree. Bound via `dataField` is also exercised below.',
      color: 'dimmed',
      size: 'sm',
    },

    // Row 1 — line + bar
    {
      kind: 'grid',
      cols: 2,
      gap: 'md',
      children: [
        {
          kind: 'card',
          children: [
            {
              kind: 'chart',
              chartKind: 'line',
              title: 'Line — tickets last 7 days',
              dataKey: 'day',
              series: [
                { name: 'open', label: 'Open' },
                { name: 'closed', label: 'Closed' },
              ],
              data: [
                { day: 'Mon', open: 12, closed: 8 },
                { day: 'Tue', open: 15, closed: 11 },
                { day: 'Wed', open: 9, closed: 14 },
                { day: 'Thu', open: 18, closed: 12 },
                { day: 'Fri', open: 22, closed: 17 },
                { day: 'Sat', open: 10, closed: 9 },
                { day: 'Sun', open: 7, closed: 6 },
              ],
            },
          ],
        },
        {
          kind: 'card',
          children: [
            {
              kind: 'chart',
              chartKind: 'bar',
              title: 'Bar — sales by region',
              dataKey: 'region',
              series: [
                { name: 'q1', label: 'Q1' },
                { name: 'q2', label: 'Q2' },
              ],
              data: [
                { region: 'NA', q1: 320, q2: 410 },
                { region: 'EU', q1: 280, q2: 350 },
                { region: 'APAC', q1: 195, q2: 260 },
                { region: 'LATAM', q1: 90, q2: 130 },
              ],
            },
          ],
        },
      ],
    },

    // Row 2 — area + pie
    {
      kind: 'grid',
      cols: 2,
      gap: 'md',
      children: [
        {
          kind: 'card',
          children: [
            {
              kind: 'chart',
              chartKind: 'area',
              title: 'Area — daily active users',
              dataKey: 'date',
              curveType: 'monotone',
              series: [{ name: 'users', label: 'Users' }],
              data: [
                { date: 'Day 1', users: 120 },
                { date: 'Day 2', users: 145 },
                { date: 'Day 3', users: 132 },
                { date: 'Day 4', users: 178 },
                { date: 'Day 5', users: 210 },
                { date: 'Day 6', users: 195 },
                { date: 'Day 7', users: 240 },
              ],
            },
          ],
        },
        {
          kind: 'card',
          children: [
            {
              kind: 'chart',
              chartKind: 'pie',
              title: 'Pie — issue status',
              data: [
                { name: 'Open', value: 42 },
                { name: 'In progress', value: 18 },
                { name: 'Resolved', value: 31 },
                { name: 'Closed', value: 9 },
              ],
            },
          ],
        },
      ],
    },

    // Row 3 — donut spanning full width
    {
      kind: 'card',
      children: [
        {
          kind: 'chart',
          chartKind: 'donut',
          title: 'Donut — traffic sources',
          h: 280,
          thickness: 28,
          withLabels: true,
          data: [
            { name: 'Organic', value: 4823 },
            { name: 'Direct', value: 3120 },
            { name: 'Referral', value: 1408 },
            { name: 'Email', value: 967 },
            { name: 'Social', value: 612 },
          ],
        },
      ],
    },

    { kind: 'divider', label: 'Bound via dataField' },

    // Row 4 — chart bound via dataField (uses outer `data` from UIPage prop)
    {
      kind: 'card',
      children: [
        {
          kind: 'chart',
          chartKind: 'bar',
          title: 'Bar — bound via $.boundSeries',
          dataKey: 'category',
          series: [{ name: 'count', label: 'Count' }],
          dataField: '$.boundSeries',
        },
      ],
    },
  ],
}

export const CHART_DEMO_DATA = {
  boundSeries: [
    { category: 'A', count: 12 },
    { category: 'B', count: 19 },
    { category: 'C', count: 7 },
    { category: 'D', count: 24 },
  ],
}

export const MEDIA_TIMELINE_TREE = {
  kind: 'vbox',
  gap: 'lg',
  children: [
    { kind: 'heading', text: 'Page-DSL media + timeline — visual smoke test', level: 3 },
    {
      kind: 'text',
      content:
        'Every new node kind shipped by gui-media + gui-timeline MVPs, in one tree. Use this page to confirm rendering before exposing the kinds to LLM authoring.',
      color: 'dimmed',
      size: 'sm',
    },

    // ── Row 1: image + video side by side ──────────────────────────────
    {
      kind: 'section',
      title: '1 — Image (with caption) + Video',
      description: 'image gained a `caption` field; video is a new kind wrapping native <video>.',
      children: [
        {
          kind: 'grid',
          cols: 2,
          gap: 'md',
          children: [
            {
              kind: 'card',
              children: [
                {
                  kind: 'image',
                  src: SAMPLE_IMAGE,
                  alt: 'A scenic Unsplash photograph',
                  height: 220,
                  fit: 'cover',
                  radius: 'sm',
                  caption: 'A scenic landscape (Unsplash sample) — caption renders dimmed below.',
                },
              ],
            },
            {
              kind: 'card',
              children: [
                {
                  kind: 'video',
                  src: SAMPLE_VIDEO_MP4,
                  mime: 'video/mp4',
                  controls: true,
                  height: 220,
                  radius: 'sm',
                  caption: 'Big Buck Bunny sample — controls visible; click to play.',
                },
              ],
            },
          ],
        },
      ],
    },

    // ── Row 2: audio + file ────────────────────────────────────────────
    {
      kind: 'section',
      title: '2 — Audio + File',
      description: 'Both are new kinds. Audio uses native player; file renders a download card.',
      children: [
        {
          kind: 'grid',
          cols: 2,
          gap: 'md',
          children: [
            {
              kind: 'card',
              children: [
                {
                  kind: 'audio',
                  src: SAMPLE_AUDIO_MP3,
                  mime: 'audio/mpeg',
                  controls: true,
                  label: 'A short button sound sample',
                },
              ],
            },
            {
              kind: 'card',
              children: [
                {
                  kind: 'file',
                  src: SAMPLE_FILE_PDF,
                  name: 'dummy.pdf',
                  mime: 'application/pdf',
                  sizeBytes: 13264,
                  description: 'W3C sample PDF — click Download to save.',
                  downloadAs: 'gui-media-sample.pdf',
                },
              ],
            },
          ],
        },
      ],
    },

    // ── Row 3: timeline (full width) ───────────────────────────────────
    {
      kind: 'section',
      title: '3 — Timeline',
      description:
        'Vertical event timeline (Mantine Timeline). Each item has a date label, title, and optional markdown body. activeIndex marks how many bullets are "completed".',
      children: [
        {
          kind: 'card',
          children: [
            {
              kind: 'timeline',
              activeIndex: 3,
              color: 'blue',
              items: [
                {
                  date: '2026-01-15',
                  title: 'Kickoff',
                  body: '**Stakeholder alignment** + RACI distribution. Three teams in scope.\n\n- Strategy lead: assigned\n- Architecture lead: assigned',
                  color: 'blue',
                },
                {
                  date: '2026-02-20',
                  title: 'Discovery complete',
                  body: 'Workshops with operations + finance produced a [target architecture brief](https://example.com/brief). Ready for design.',
                  color: 'blue',
                },
                {
                  date: '2026-03-12',
                  title: 'MVP delivery',
                  body: 'First end-to-end flow demoed to the customer. Sign-off received.\n\n```\nbuild  ✓\ndeploy ✓\nsignoff ✓\n```',
                  color: 'green',
                },
                {
                  date: '2026-05-01',
                  title: 'Pilot expansion',
                  body: 'Three more business units onboarded. Telemetry shows 28% uplift in cycle time.',
                  color: 'yellow',
                },
                {
                  date: 'Q3 2026',
                  title: 'GA target',
                  body: 'Pending go/no-go in late June.',
                },
              ],
            },
          ],
        },
      ],
    },

    {
      kind: 'hbox',
      gap: 'sm',
      children: [
        { kind: 'badge', text: 'gui-media MVP', color: 'teal', variant: 'light' },
        { kind: 'badge', text: 'gui-timeline MVP', color: 'indigo', variant: 'light' },
        { kind: 'badge', text: 'page-DSL', color: 'gray', variant: 'outline' },
      ],
    },
  ],
}

export const PRIMITIVES_SCHEMA: Schema = {
  name: 'ui-primitives',
  version: 1,
  description: 'Renderer coverage gallery — every wired content-type in ui-builders/fields/factory.ts.',
  root: 'AllPrimitives',
  types: {
    AllPrimitives: {
      type: 'object',
      label: 'All primitives',
      titleField: ['text_field'],
      fields: {
        // ── strings ────────────────────────────────────────────────
        text_field: {
          type: 'string',
          label: 'Text',
          description: 'Default TextInput.',
        },
        email_field: {
          type: 'string',
          label: 'Email',
          content: { type: 'email' },
          description: 'HTML5 email validation.',
        },
        date_field: {
          type: 'string',
          label: 'Date',
          content: { type: 'date' },
          description: 'Mantine DateInput; ISO date string.',
        },
        markdown_field: {
          type: 'string',
          label: 'Markdown',
          content: { type: 'markdown', maxLength: 500 },
          description: 'Tabbed write / preview; react-markdown + remark-gfm in view mode.',
        },
        // ── numbers ───────────────────────────────────────────────
        number_field: {
          type: 'number',
          label: 'Number',
          description: 'Default NumberInput.',
        },
        slider_field: {
          type: 'number',
          label: 'Slider',
          content: { type: 'slider', min: 0, max: 100 } as any,
          default: 50,
          description: 'Mantine Slider — min/max/step from content.',
        },
        rating_field: {
          type: 'number',
          label: 'Rating',
          content: { type: 'rating', max: 5 } as any,
          default: 3,
          description: 'Star rating; max from content.',
        },
        // ── booleans (3 editAs variants) ──────────────────────────
        boolean_checkbox: {
          type: 'boolean',
          label: 'Boolean (checkbox)',
          content: { type: 'boolean', editAs: 'checkbox' } as any,
          description: 'editAs:checkbox (default).',
        },
        boolean_switch: {
          type: 'boolean',
          label: 'Boolean (switch)',
          content: { type: 'boolean', editAs: 'switch' } as any,
          description: 'editAs:switch.',
        },
        boolean_flag_protected: {
          type: 'boolean',
          label: 'Boolean (flag — protected)',
          content: { type: 'boolean', editAs: 'flag', flagIcon: 'protected' } as any,
          description: 'editAs:flag + flagIcon:protected (shield-lock orange / shield green).',
        },
        boolean_flag_warning: {
          type: 'boolean',
          label: 'Boolean (flag — warning)',
          content: { type: 'boolean', editAs: 'flag', flagIcon: 'warning' } as any,
          description: 'editAs:flag + flagIcon:warning (bulb orange / check green).',
        },
        // ── select with options (rich vs plain) ───────────────────
        // The `options` field is generic on ContentType, so string / number /
        // boolean all accept it. With icon+description → a rich dropdown (icon
        // glyph + sub-text per row); without → a plain Select. Map form
        // (`{value: label}`) is also accepted and renders as a plain Select.
        doc_type_rich: {
          type: 'string',
          label: 'Document type (string, RICH)',
          content: {
            type: 'text',
            options: [
              { value: 'etude', label: "Étude d'architecture", description: 'Dossier d’architecture complet', icon: 'IconBuildingSkyscraper' },
              { value: 'matrice', label: 'Matrice de flux', description: 'Tableau des flux interapplicatifs', icon: 'IconTable' },
              { value: 'compte-rendu', label: 'Compte-rendu', description: 'Situation à un instant T', icon: 'IconFileText' },
            ],
          },
          description: 'String + rich options → Select with icon + sub-text per row.',
        },
        env_plain: {
          type: 'string',
          label: 'Environment (string, plain)',
          content: {
            type: 'text',
            options: [
              { value: 'dev', label: 'Development' },
              { value: 'staging', label: 'Staging' },
              { value: 'prod', label: 'Production' },
            ],
          },
          description: 'String + plain options → simple dropdown (no icon / sub-text).',
        },
        status_map: {
          type: 'string',
          label: 'Status (string, map form)',
          content: {
            type: 'text',
            options: { draft: 'Draft', review: 'In review', published: 'Published' },
          },
          description: 'String + FieldOptionMap (value→label) → simple dropdown.',
        },
        severity_rich: {
          type: 'number',
          label: 'Severity (number, RICH)',
          content: {
            type: 'number',
            options: [
              { value: 1, label: 'Low', description: 'Minor impact', icon: 'IconLeaf' },
              { value: 2, label: 'Medium', description: 'Noticeable impact', icon: 'IconBolt' },
              { value: 3, label: 'High', description: 'Severe impact', icon: 'IconFlame' },
            ],
          },
          description: 'Number + rich options → Select; picked value stored back as a number.',
        },
        port_plain: {
          type: 'number',
          label: 'Port (number, plain)',
          content: {
            type: 'number',
            options: [
              { value: 80, label: '80 (HTTP)' },
              { value: 443, label: '443 (HTTPS)' },
              { value: 5432, label: '5432 (Postgres)' },
            ],
          },
          description: 'Number + plain options → simple dropdown; value stored as a number.',
        },
        consent_rich: {
          type: 'boolean',
          label: 'Consent (boolean, RICH)',
          content: {
            type: 'boolean',
            options: [
              { value: true, label: 'I agree', description: 'Accept the terms', icon: 'IconCircleCheck' },
              { value: false, label: 'I decline', description: 'Reject the terms', icon: 'IconCircleX' },
            ],
          },
          description: 'Boolean + rich options → Select; picked value stored back as a boolean.',
        },
        toggle_plain: {
          type: 'boolean',
          label: 'Feature (boolean, plain)',
          content: {
            type: 'boolean',
            options: [
              { value: true, label: 'Enabled' },
              { value: false, label: 'Disabled' },
            ],
          },
          description: 'Boolean + plain options → simple dropdown; value stored as a boolean.',
        },
        // ── arrays / enum-multi ───────────────────────────────────
        resources_rich: {
          type: 'array',
          items: { type: 'string' },
          content: {
            type: 'enum-multi',
            editAs: 'checkboxes',
            options: [
              { value: 'confluence', label: 'Confluence', description: 'Pages & specs', icon: 'IconBrandConfluence' },
              { value: 'leanix', label: 'LeanIX', description: 'Declared catalog', icon: 'IconSitemap' },
              { value: 'dynatrace', label: 'Dynatrace', description: 'Observed topology', icon: 'IconActivity' },
            ],
          },
          label: 'Resources (enum-multi, RICH)',
          description: 'enum-multi + rich options → checkbox group with icon + sub-text per option.',
          optional: true,
        } as any,
        tags_free: {
          type: 'array',
          items: { type: 'string' },
          label: 'Tags (free entry)',
          description: 'TagsInput — type + Enter to add. Dossier `parent_da` equivalent.',
          optional: true,
        } as any,
        tags_bounded: {
          type: 'array',
          items: { type: 'string', enum: ['alpha', 'beta', 'gamma', 'delta'] } as any,
          content: { type: 'enum-multi', editAs: 'multiselect' } as any,
          label: 'Tags (bounded enum)',
          description: 'MultiSelect — fixed option list (editAs:multiselect).',
          optional: true,
        } as any,
      },
    } as any,
  },
} as Schema

export const SIMPLE_DRAWIO: { xml: string; filename: string } = {
  filename: 'sample.drawio',
  xml: `<mxfile host="app.diagrams.net" modified="2026-05-07T00:00:00.000Z" type="device">
  <diagram name="Sample" id="sample-1">
    <mxGraphModel dx="800" dy="600" grid="1" gridSize="10" guides="1" tooltips="1" connect="1" arrows="1" fold="1" page="1" pageScale="1" pageWidth="850" pageHeight="600" math="0" shadow="0">
      <root>
        <mxCell id="0" />
        <mxCell id="1" parent="0" />
        <mxCell id="2" vertex="1" parent="1"
          style="rounded=1;whiteSpace=wrap;html=1;fillColor=#dae8fc;strokeColor=#6c8ebf;"
          value="Frontend">
          <mxGeometry x="80" y="160" width="160" height="60" as="geometry" />
        </mxCell>
        <mxCell id="3" vertex="1" parent="1"
          style="rounded=1;whiteSpace=wrap;html=1;fillColor=#d5e8d4;strokeColor=#82b366;"
          value="API gateway">
          <mxGeometry x="360" y="160" width="160" height="60" as="geometry" />
        </mxCell>
        <mxCell id="4" vertex="1" parent="1"
          style="rounded=1;whiteSpace=wrap;html=1;fillColor=#ffe6cc;strokeColor=#d79b00;"
          value="Order service">
          <mxGeometry x="640" y="80" width="160" height="60" as="geometry" />
        </mxCell>
        <mxCell id="5" vertex="1" parent="1"
          style="rounded=1;whiteSpace=wrap;html=1;fillColor=#f8cecc;strokeColor=#b85450;"
          value="Inventory service">
          <mxGeometry x="640" y="240" width="160" height="60" as="geometry" />
        </mxCell>
        <mxCell id="6" edge="1" parent="1" source="2" target="3"
          style="edgeStyle=orthogonalEdgeStyle;rounded=1;orthogonalLoop=1;jettySize=auto;html=1;"
          value="HTTPS">
          <mxGeometry relative="1" as="geometry" />
        </mxCell>
        <mxCell id="7" edge="1" parent="1" source="3" target="4"
          style="edgeStyle=orthogonalEdgeStyle;rounded=1;html=1;"
          value="REST">
          <mxGeometry relative="1" as="geometry" />
        </mxCell>
        <mxCell id="8" edge="1" parent="1" source="3" target="5"
          style="edgeStyle=orthogonalEdgeStyle;rounded=1;html=1;"
          value="REST">
          <mxGeometry relative="1" as="geometry" />
        </mxCell>
      </root>
    </mxGraphModel>
  </diagram>
</mxfile>`,
}
