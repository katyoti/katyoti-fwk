---
name: display-ui-demo
advertised: false
description: Demonstrates the display-ui-component tool — emits 3 shapes of UI artifacts (a single view, an ad-hoc dashboard, a streamed slide list), each opens as a new tab in the GUI
type: task
iconName: layout-dashboard
icon: "🖼"
hashtags: [demo, ui, dsl, gui, streaming]
category: demo
config:
  capability: smart
  plugins:
    - standard
---

# Display Demo

You are demonstrating the `display-ui-component` tool, which renders structured UI
in new GUI tabs. The user can interact with each tab (forms submit, tables
filter, slides navigate). You'll emit three increasingly-rich shapes to show
the full range of the tool.

Follow these steps in order. **Do not deviate from the JSON shapes shown below**
— they are validated server-side and a typo will reject the call.

## Step 1 — Read the topic from your input

The topic is provided in your input — read it from the `FORM_INPUT` block (key `topic`), or from the message text when run from the CLI. Do NOT use `ask-user`.

`<topic>` is that value. If it is "Books" or
similar concrete-noun, it works as the schema/data subject. If it is
something abstract like "team standup", invent a plausible record/data shape
that fits.

## Step 2 — Shape 1: a rich editable form

Call `display-ui-component` with `kind: "form"` and the following structure.
The schema must cover **at least 7 fields spanning multiple content types**
so the form exercises the full renderer registry: plain text, email,
rating, slider, checkbox, date, markdown textarea. Substitute every
`<placeholder>` for real data tied to your `<topic>`.

```json
{
  "artifactId": "art-shape1-form",
  "kind": "form",
  "title": "Shape 1 — rich editable form",
  "mode": "edit",
  "schema": {
    "name": "<TopicName>",
    "root": "<TopicName>",
    "types": {
      "<TopicName>": {
        "type": "object",
        "idField": ["id"],
        "titleField": ["title"],
        "fields": {
          "id":           { "type": "string",  "label": "ID" },
          "title":        { "type": "string",  "label": "Title", "description": "Short, scannable; goes in lists and tabs." },
          "owner_email":  { "type": "string",  "label": "Owner email", "content": { "type": "email" }, "optional": true },
          "priority":     { "type": "number",  "label": "Priority", "content": { "type": "rating", "max": 5 }, "default": 3 },
          "completion":   { "type": "number",  "label": "Completion", "content": { "type": "slider", "min": 0, "max": 100 }, "default": 0, "description": "Percent of work done — drives the progress badge in lists." },
          "is_active":    { "type": "boolean", "label": "Active?", "default": true },
          "due_date":     { "type": "string",  "label": "Due date", "content": { "type": "date" }, "optional": true },
          "notes":        { "type": "string",  "label": "Notes", "content": { "type": "markdown", "maxLength": 500 }, "optional": true, "description": "Free-form context; markdown is rendered." }
        },
        "extensions": [
          {
            "type": "ui-form",
            "name": "default",
            "groups": [
              { "title": "Identity",   "cols": 2, "fields": ["title", "owner_email"] },
              { "title": "Status",     "cols": 2, "fields": ["priority", "completion", "is_active", "due_date"] },
              { "title": "Notes",      "cols": 1, "fields": ["notes"] },
              { "title": "Audit",      "cols": 1, "fields": ["id"], "visibility": ["view"] }
            ]
          }
        ]
      }
    }
  },
  "data": {
    "id":          "<short-stable-id>",
    "title":       "<a real, substantive title for the topic>",
    "owner_email": "<a plausible email>",
    "priority":    <integer 1-5>,
    "completion":  <integer 0-100>,
    "is_active":   <true or false>,
    "due_date":    "<YYYY-MM-DD ~7-30 days out>",
    "notes":       "<2-3 sentences of substantive markdown notes about this record — what it tracks, why it matters>"
  }
}
```

Tab opens. User sees an **editable form** with field types grouped into
sections (Identity, Status, Notes, Audit). The Status row shows a 5-star
priority rating, a 0-100 slider for completion, a checkbox, and a date
picker side-by-side. Notes is a markdown textarea. This single artifact
demonstrates form rendering depth — multiple content types, named-group
layout, optional fields, defaults, and the `visibility: ["view"]` filter
that hides `id` from edit mode.

## Step 3 — Shape 2: a rich, multi-section dashboard (page DSL)

Call `display-ui-component` with `kind: "page"` and a DSL tree showing a
sophisticated dashboard. **Make this look like a real product UI** — multiple
sections, mixed node kinds, embedded data tables, status badges, structured
information. Adapt EVERY label, value, badge, and table row to the user's topic.

The dashboard must include:
- A `section` header at the top with title + description
- A primary 4-column KPI grid (pick 4 metrics that make sense for the topic)
- A `grid` (cols: 2) of 2 charts: one **line chart** showing a time-series
  trend (5-7 data points) and one **pie chart** showing a 3-5 slice category
  breakdown. Use the `chart` atomic node with `chartKind: "line"` /
  `chartKind: "pie"` (see Step 3.5 below for shape).
- A 2-column split: status card (badges + list) on the left, an embedded
  `table` (with inline `schema`) on the right showing 4-5 rows of
  related data
- A `tabs` node with 2-3 tabs grouping related sub-sections
  (e.g. "Overview" / "Activity" / "Notes" — each with markdown content)
- A **media + milestones** band: a 2-column `grid` of an `image` (with
  `caption`) and a `file` download card, plus a full-width `video` and a
  full-width `timeline` of 4-5 milestones. Pick plausible public sample
  URLs (or topic-relevant ones if you have them) — the iframe just loads
  whatever URL you supply
- A footer `hbox` of action-style badges
- Use `id` on every node (the LLM names them like "kpi-active", "table-recent",
  "tab-overview") so future cross-turn patches can address them

```json
{
  "artifactId": "art-shape2-dashboard",
  "kind": "page",
  "title": "Shape 2 — dashboard",
  "dslTree": {
    "kind": "vbox",
    "gap": "lg",
    "children": [
      {
        "kind": "section",
        "id": "header",
        "title": "<Topic> — Operational Dashboard",
        "description": "<one-sentence summary of what this dashboard tracks>",
        "children": [
          {
            "kind": "grid",
            "id": "kpi-grid",
            "cols": 4,
            "children": [
              { "id": "kpi-1", "kind": "kpi", "label": "<Metric 1>", "value": "<value>", "trend": "+N%", "description": "<context>" },
              { "id": "kpi-2", "kind": "kpi", "label": "<Metric 2>", "value": "<value>", "trend": "-N", "description": "<context>" },
              { "id": "kpi-3", "kind": "kpi", "label": "<Metric 3>", "value": "<value>", "description": "<context>" },
              { "id": "kpi-4", "kind": "kpi", "label": "<Metric 4>", "value": "<value>", "trend": "stable", "description": "<context>" }
            ]
          }
        ]
      },
      {
        "kind": "grid",
        "id": "charts-grid",
        "cols": 2,
        "gap": "md",
        "children": [
          {
            "kind": "chart",
            "id": "chart-trend",
            "chartKind": "line",
            "title": "<a label that fits the topic, e.g. 'Tickets — last 7 days'>",
            "h": 240,
            "dataKey": "label",
            "series": [
              { "name": "value", "label": "<series label>" }
            ],
            "data": [
              { "label": "Day 1", "value": <number> },
              { "label": "Day 2", "value": <number> },
              { "label": "Day 3", "value": <number> },
              { "label": "Day 4", "value": <number> },
              { "label": "Day 5", "value": <number> },
              { "label": "Day 6", "value": <number> },
              { "label": "Day 7", "value": <number> }
            ]
          },
          {
            "kind": "chart",
            "id": "chart-breakdown",
            "chartKind": "pie",
            "title": "<a label, e.g. 'By category'>",
            "h": 240,
            "data": [
              { "name": "<category 1>", "value": <number> },
              { "name": "<category 2>", "value": <number> },
              { "name": "<category 3>", "value": <number> },
              { "name": "<category 4>", "value": <number> }
            ]
          }
        ]
      },
      {
        "kind": "grid",
        "cols": 2,
        "gap": "md",
        "children": [
          {
            "kind": "card",
            "id": "status-card",
            "children": [
              { "kind": "heading", "text": "Status", "level": 4 },
              { "kind": "divider" },
              {
                "kind": "hbox",
                "gap": "xs",
                "wrap": true,
                "children": [
                  { "kind": "badge", "text": "<Status A>", "color": "green" },
                  { "kind": "badge", "text": "<Status B>", "color": "yellow" },
                  { "kind": "badge", "text": "<Status C>", "color": "blue", "variant": "outline" },
                  { "kind": "badge", "text": "<Status D>", "color": "gray", "variant": "outline" }
                ]
              },
              { "kind": "divider", "label": "Latest" },
              { "kind": "markdown", "content": "<3-4 bullet list of latest events or signals — written substantively for the topic>" }
            ]
          },
          {
            "kind": "card",
            "id": "data-card",
            "children": [
              { "kind": "heading", "text": "<Plural noun related to topic — e.g. 'Recent items'>", "level": 4 },
              {
                "kind": "table",
                "id": "data-table",
                "schema": {
                  "name": "<EntityList>",
                  "root": "<EntityList>",
                  "types": {
                    "<EntityList>": { "type": "array", "items": "<Entity>" },
                    "<Entity>": {
                      "type": "object",
                      "idField": ["id"],
                      "titleField": ["name"],
                      "fields": {
                        "id": { "type": "string", "label": "ID" },
                        "name": { "type": "string", "label": "<Field 1 label>" },
                        "status": { "type": "string", "label": "Status" },
                        "metric": { "type": "number", "label": "<Field 3 label>" }
                      }
                    }
                  }
                },
                "data": [
                  { "id": "1", "name": "<row 1>", "status": "active", "metric": 12 },
                  { "id": "2", "name": "<row 2>", "status": "pending", "metric": 7 },
                  { "id": "3", "name": "<row 3>", "status": "active", "metric": 23 },
                  { "id": "4", "name": "<row 4>", "status": "archived", "metric": 0 },
                  { "id": "5", "name": "<row 5>", "status": "active", "metric": 9 }
                ]
              }
            ]
          }
        ]
      },
      {
        "kind": "section",
        "id": "media-milestones",
        "title": "Resources & milestones",
        "description": "Reference materials + a milestone timeline for <topic>.",
        "children": [
          {
            "kind": "grid",
            "id": "media-grid",
            "cols": 2,
            "gap": "md",
            "children": [
              {
                "kind": "card",
                "id": "media-image-card",
                "children": [
                  { "kind": "heading", "text": "Reference visual", "level": 5 },
                  {
                    "kind": "image",
                    "id": "media-image",
                    "src": "<a public image URL relevant to the topic — e.g. an Unsplash photo>",
                    "alt": "<topic-relevant alt text>",
                    "height": 200,
                    "fit": "cover",
                    "radius": "sm",
                    "caption": "<one-sentence caption tying the image to the topic>"
                  }
                ]
              },
              {
                "kind": "card",
                "id": "media-file-card",
                "children": [
                  { "kind": "heading", "text": "Companion document", "level": 5 },
                  {
                    "kind": "file",
                    "id": "media-file",
                    "src": "<a public file URL — sample PDF, dataset, report>",
                    "name": "<descriptive filename including extension>",
                    "mime": "<application/pdf or application/json or text/csv>",
                    "sizeBytes": <plausible size>,
                    "description": "<one-line description of what the file contains>",
                    "downloadAs": "<friendly download name>"
                  }
                ]
              }
            ]
          },
          {
            "kind": "card",
            "id": "media-video-card",
            "children": [
              { "kind": "heading", "text": "Watch the walkthrough", "level": 5 },
              {
                "kind": "video",
                "id": "media-video",
                "src": "<a public mp4 URL — feel free to use 'https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4' as a stand-in if no topic-specific clip>",
                "mime": "video/mp4",
                "controls": true,
                "height": 280,
                "radius": "sm",
                "caption": "<one-sentence caption tying the video to the topic>"
              }
            ]
          },
          {
            "kind": "card",
            "id": "milestones-card",
            "children": [
              { "kind": "heading", "text": "Milestones", "level": 5 },
              {
                "kind": "timeline",
                "id": "milestones-timeline",
                "color": "blue",
                "activeIndex": 3,
                "items": [
                  { "date": "<YYYY-MM-DD past>",  "title": "<Milestone 1 — early>",       "body": "<2-3 sentences of substantive markdown about what happened>", "color": "blue"  },
                  { "date": "<YYYY-MM-DD past>",  "title": "<Milestone 2 — mid>",         "body": "<2-3 sentences>", "color": "blue"  },
                  { "date": "<YYYY-MM-DD past>",  "title": "<Milestone 3 — recent done>", "body": "<2-3 sentences>", "color": "green" },
                  { "date": "<YYYY-MM-DD soon>",  "title": "<Milestone 4 — in progress>", "body": "<2-3 sentences about what's underway>", "color": "yellow" },
                  { "date": "<rough future label, e.g. 'Q4 2026'>", "title": "<Milestone 5 — planned>", "body": "<1-2 sentences about the plan>" }
                ]
              }
            ]
          }
        ]
      },
      {
        "kind": "tabs",
        "id": "tabs",
        "tabs": [
          {
            "id": "tab-overview",
            "label": "Overview",
            "content": {
              "kind": "vbox",
              "gap": "sm",
              "children": [
                { "kind": "heading", "text": "Overview", "level": 4 },
                { "kind": "markdown", "content": "<2-3 paragraphs of substantive overview content for the topic — written like a real status report, not lorem ipsum>" }
              ]
            }
          },
          {
            "id": "tab-activity",
            "label": "Activity",
            "content": {
              "kind": "vbox",
              "gap": "sm",
              "children": [
                { "kind": "heading", "text": "Recent Activity", "level": 4 },
                { "kind": "markdown", "content": "<4-6 bulleted list of recent activity entries, each with a timestamp-like prefix and a substantive description>" }
              ]
            }
          },
          {
            "id": "tab-notes",
            "label": "Notes",
            "content": {
              "kind": "vbox",
              "gap": "sm",
              "children": [
                { "kind": "heading", "text": "Notes", "level": 4 },
                { "kind": "markdown", "content": "<a freeform notes block with a code block somewhere inside — use a fenced code block ``` for an example/snippet relevant to the topic>" }
              ]
            }
          }
        ]
      },
      {
        "kind": "hbox",
        "gap": "sm",
        "children": [
          { "kind": "badge", "text": "Last updated: just now", "color": "gray", "variant": "outline" },
          { "kind": "badge", "text": "Source: <topic-relevant source>", "color": "indigo", "variant": "light" },
          { "kind": "link", "text": "View full report →", "href": "https://example.com/report" }
        ]
      }
    ]
  }
}
```

Tab opens. User sees a real-looking operational dashboard: section header,
4 KPIs across the top, a 2-column row mixing a status card with badges/list
and a 5-row data table, a **Resources & milestones** band (image + file
card, then video, then a 5-event timeline), a 3-tab info panel, and a
footer with metadata badges and a link.

## Step 4 — Shape 3: a streamed slide list (page DSL)

Call `display-ui-component` with `kind: "page"` and a `vbox` of 5 slide-cards.
Each slide is a card containing a heading + markdown:

```json
{
  "artifactId": "art-shape3-slides",
  "kind": "page",
  "title": "Shape 3 — slide list",
  "dslTree": {
    "kind": "vbox",
    "gap": "lg",
    "children": [
      { "kind": "heading", "text": "<Topic>: 5 things to know", "level": 1 },
      { "kind": "card", "children": [
        { "kind": "heading", "text": "1. <Slide 1 title>", "level": 3 },
        { "kind": "markdown", "content": "<3-4 sentences of substantive markdown content>" }
      ]},
      { "kind": "card", "children": [
        { "kind": "heading", "text": "2. <Slide 2 title>", "level": 3 },
        { "kind": "markdown", "content": "<3-4 sentences>" }
      ]},
      { "kind": "card", "children": [
        { "kind": "heading", "text": "3. <Slide 3 title>", "level": 3 },
        { "kind": "markdown", "content": "<3-4 sentences>" }
      ]},
      { "kind": "card", "children": [
        { "kind": "heading", "text": "4. <Slide 4 title>", "level": 3 },
        { "kind": "markdown", "content": "<3-4 sentences>" }
      ]},
      { "kind": "card", "children": [
        { "kind": "heading", "text": "5. <Slide 5 title>", "level": 3 },
        { "kind": "markdown", "content": "<3-4 sentences>" }
      ]}
    ]
  }
}
```

Tab opens. User sees a scroll of 5 cards, each a "slide" of substantive content.
Because the JSON is larger, the user MAY see slides appearing progressively as
the partial-JSON parser successfully resolves them.

## Step 5 — Confirm completion

Call `complete` with a summary like:
**"Generated three UI artifacts for `<topic>`: a rich editable form, an
ad-hoc dashboard, and a 5-slide list. Each opened as a new tab in the
GUI."**

## Notes for the LLM

- **Don't combine shapes** — emit three separate `display-ui-component` calls.
  Each shape exercises a different code path (schema-bound form with
  named groups vs page DSL with KPIs vs page DSL with vbox-of-cards).
- **Substantive content** — each slide / KPI / markdown body must actually be
  about the user's topic. No filler, no Lorem ipsum, no apologies. Write
  thoughtfully — the user is judging real-looking output.
- **JSON discipline** — emit ONE clean JSON object per tool call. The
  partial-JSON parser is forgiving but mid-sentence retries cost the user
  time. Don't second-guess yourself mid-tool-call.
- **A `schema` is inline or a data-type URI** (`ktw-demo:/data-type/book`): the host
  resolves an id, with its `variant`, before the page renders. All schemas in this
  demo are inline.
- **Don't use `persistent: true`** — these are one-shot artifacts. They auto-
  complete when the tool call finishes.
