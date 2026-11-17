# iframe-widget

Page-level iframe primitive for ktw-display-served widgets. **One iframe per top-level composition** (form / view / table / wizard / deck / dashboard / artifact) per the iframe-per-page invariant in [modules/display/host-iframe-wire.md](../../../../../../modules/display/host-iframe-wire.md).

## Why iframe?

Per-field rendering happens **inside** the iframe page via the in-page registry. The host (ktw-gui) does NOT round-trip per field. The iframe is the boundary where:

- The host owns: mounting, theme push, value injection, schema-changed reloads, action dispatch
- The iframe owns: schema interpretation, content-type renderers, validation, value composition, postMessage emission

## Usage

```tsx
import { IframeWidget } from './components/iframe-widget'

<IframeWidget kind="form" schemaName="Book" mode="edit" />
```

Optional props: `value`, `mode`, `universe` (defaults to displayed universe), `onStatusChange`, `onIframeMessage`, `sandbox`, `initialHeight`.

## How URL resolution works

1. Component mounts → calls `App.GetWidgetURL(universe, kind, schemaName, value, mode)`
2. The Wails binding routes via KTSP to the universe's active core, which delegates to `displaybridge.SchemaBridge.GetWidgetURL`, which forwards to bun's `/api/widget-url`
3. Bun composes `http://127.0.0.1:<port>/widget/<kind>?...&token=<sessionToken>` and returns it
4. The host appends `widgetId` to the URL (UUID per mount, used for inbound message correlation) and sets it as the iframe's `src`

If any step fails (Wails missing, universe inactive, bridge unreachable, bun down), the host falls back to a **dev-mode URL** built from `localStorage.getItem('display.devPort')` (default `9990`). Status flips from `'ok'` to `'fallback'`. If even the fallback iframe fails to load, status flips to `'down'`.

## postMessage protocol

All inbound messages from the iframe are:
1. **Origin-checked** — `event.origin` must equal the resolved URL's origin
2. **`isIframeMessage`-validated** — well-formed shape with one of the 7 known `type` discriminators
3. **`widgetId`-correlated** — must match this mount's id (drops sibling-widget noise when two widgets are mounted in the same window)

Type definitions live in [app/ktw-display/src/postmessage.ts](../../../../../ktw-display/src/postmessage.ts) — the GUI imports them via the `@display/*` tsconfig path mapping (`@display/postmessage`).

Both raw-stream (`onIframeMessage`) and per-kind callbacks (`onReady`, `onChange`, `onSubmit`, `onAction`, `onError`, `onResize`, `onRequestValue`) are supported. The raw stream fires alongside the per-kind dispatch — use it for debug/event-log panels; use the per-kind ones for typed handlers.

**Built-in side effects** (Subtask 5):
- `'ready'` → flips internal `iframeReady` flag, triggers initial theme push
- `'resize'` → applies clamped height to the container (`maxHeight` prop, default 800px)
- `'error'` → flips status to `'down'` with the error message in `data-error`
- Theme switch in the GUI (Mantine color scheme via `useApplySettings`) → re-pushes `ThemeMessage` to the iframe
- OS-level `prefers-color-scheme` change (when GUI theme = `'system'`) → re-pushes via matchMedia listener

Outbound helpers exposed for parent components: `postTheme`, `postValue`, `postSchemaChanged`, `postReload` — useful for injecting a value in response to `request-value`, forcing reload after schema mutation, etc.

## Status states

| Status | Meaning |
|---|---|
| `loading` | initial mount, awaiting URL resolution |
| `ok` | URL resolved via bridge + iframe loaded |
| `fallback` | dev URL in use (bridge / Wails / universe unreachable) |
| `down` | unrecoverable (no URL or iframe load failed even with fallback) |

The `useDisplayStatus()` hook is a separate, lightweight global health probe — useful for status badges (e.g. an IconRibbon dot) when no specific widget is mounted yet.

## File layout

```
iframe-widget/
├── IframeWidget.tsx        Main component
├── lifecycle.ts            Per-kind dispatch + outbound senders + theme/height helpers
├── use-display-status.ts   Global health hook
├── index.ts                Barrel exports
└── README.md               This file
```

## Layering

Imports postMessage types from `@display/postmessage` (single source of truth). The reverse direction (ktw-display importing from ktw-gui) is forbidden — bun has no React, no Wails, no host concerns. The iframe primitive is a one-way consumer.

## Related pattern

The architectural shape (URL via slot + widgetId correlation + per-kind dispatcher + outbound senders + theme/resize built-ins + page-boundary invariant) is the one to adopt whenever a separately-served UI surface is embedded in an iframe with host coordination. Every host ↔ iframe flow is catalogued in [modules/display/host-iframe-wire.md](../../../../../../modules/display/host-iframe-wire.md).
