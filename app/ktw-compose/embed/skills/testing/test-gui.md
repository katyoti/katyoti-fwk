---
name: test-gui
description: "Test the ktw-gui UI by driving a dev-mode instance with a browser tool: navigate, click, screenshot, read console diagnostics. Load to verify screens and click-paths the way a user sees them."
advertised: false
hashtags: [testing, gui, browser, ui]
category: guide
---

# GUI testing (ktw-gui dev + a browser tool)

The only method that tests what the user actually sees.

## 1. Start the GUI in dev mode

Run the GUI's dev server so the frontend is reachable over HTTP (the desktop
window has no URL for a browser to drive): `cd app/ktw-gui && wails dev`.

**Use the Wails bridge URL `http://localhost:34115`, NOT the raw Vite URL
(`http://localhost:5173`).** Only the `:34115` page has the `window.go` bridge
injected, so backend (Wails-bound Go) calls resolve there; on `:5173` every
backend call fails and data-driven screens render empty.

**Pick the universe from the ribbon.** `wails dev` refuses `-appargs
--universe`: open the page, click the universe's ribbon entry, then
**Activate** — its hub, members and data then serve the page as in the built
binary. Then **start on the element under test by its address** rather than
clicking your way there: navigate the browser to
`http://localhost:34115/#/open/<universe>/<place>/<path>` (`object/<id>/<tab>`,
`object/<id>/information/cap/<capability>`, `screen/<screen>/<section>`,
`home/<entry>`, `rail/<entry>`; every element of the GUI has such an address). A built GUI starts there with
`ktw-gui --open <address>`, and a case's GUI with `ktw-compose scenario gui
<case> --open <address>` (its universe when the address names none). Click
only what the test is about. Never test on the development universe: test on a universe of your own,
best a scenario case's (`ktw-compose scenario gui <case>` prints the steps;
`ktw-compose:/skill/test-scenario-case`): the GUI joins the case's own hub
(`scenario start`), and is closed before the case runs again — one writer per
workspace.

**A GUI started without the case's environment is the development
installation.** `wails dev` or `ktw-gui` launched from your own shell reads
`~/.config/katyoti`: the person's real universes, their hub, their data. Start
a case's GUI only through its `open-gui` script, or give `wails dev` exactly the
environment that script sets (`KTW_KATYOTI_ROOT=<case>/machine/root`,
`KTW_NO_INSTALLATION=1`, `KTW_CACHE=<case>/machine/cache`, and `KTW_DATA`,
`KTW_HOME`, `KTW_WORKSPACE`, `KTW_UNIVERSE` emptied). Before the first click,
check that the universe picker lists only the case's universe. If it shows any
other universe, stop the GUI.

**Start without the welcome screens.** A universe's first guide and « Quoi de
neuf » open by themselves over the screen under test. Turn them off for the
start: `KTW_NO_WELCOME=1` in the GUI's environment (`wails dev`, `katyoti gui`)
or `ktw-gui --no-welcome`. You can also turn them off for the universe: a
scenario case's `universe.no_welcome: true` creates its universe with them off.
Test the welcome screens themselves only on a start without either.

**The universe needs every sidecar whose screens you test.** A panel is served
by the sidecar that publishes it: the module dashboard needs `ktw-code` and
`ktw-gocode` among the case's members, What's Next needs `ktw-methods`. Add the
missing members to the case's `Univers.yaml` before you activate it.

**Freeze the GUI build when other sessions work in the same tree.** `wails dev`
rebuilds and restarts the GUI whenever a Go file under `app/ktw-gui` changes —
an edit by another session kills the activated universe mid-pass. Start it with
`wails dev -nogorebuild -skipbindings -noreload` (no Go rebuild, no binding
regeneration — which would also rewrite `wailsjs/` files others are editing —
no frontend reload); restart it yourself when you want a new build.

To exercise a layout/scroll/overflow behavior WITHOUT data, inject a probe into
the real rendered ancestor with the browser tool's script evaluation (§2).

## 2. Drive it with a browser tool

Use the browser automation tool your environment provides (a headless Chrome
driver, Playwright, an agent's browser): one persistent browser session shared
across commands, with console, network and errors captured. It needs four
verbs — navigate to a URL, click and type, take a screenshot (visual
evidence), read the diagnostics (console errors and failed requests) — and a
script evaluation in the page for structured reads.

**REQUIRED: pass a tool-call's JSON args through a POSIX shell (the Bash tool),
never PowerShell.** PowerShell 5.1 mangles JSON arguments that contain spaces
and quotes — it re-tokenizes them and the call fails. Git Bash with
single-quoted JSON passes the payload intact. Give the script evaluation a
single-quoted JSON arg (`\"` for inner quotes):

```bash
# Bash tool — single-quoted JSON, \" for inner quotes:
'{"script":"(()=>{ return document.title })()"}'
```

A script evaluation usually runs the script at top level, so a bare `return` throws
`Illegal return statement` — wrap your code in an IIFE `(()=>{ ... })()`.
Avoid regex backslashes (`\d`, `\s`) in the script string; they double-escape
through the shell and corrupt the JSON. Read the tool's own description
before the first call to confirm its input shape.

Use the tool's verbs to click, type, and assert. Close its browser session
when done, then kill any `ktw-gui / ktw-hub /
ktw-agents / wails` processes the dev run spawned. **Never leave two GUIs on
one workspace** — CWM is single-writer-per-workspace (lockfile); the second
conflicts.

**A declared panel** (`object-panels/`) shows as a tab on every object its
filter admits: open such an object, pick the tab, run each act on the page and
check the panel re-reads after it. The pane's answer behind the screen is
cheaper to assert first (`object-pane/Detail` through the case's hub).

## 3. The verdict loop

1. Open the element under test by its address (`#/open/<address>`);
   screenshot the baseline.
2. Perform the click-path the test is about; screenshot after each
   meaningful step.
3. Run `diagnostics` after every step — a clean-looking screen with console
   errors is a failing test.
4. Assert the end state (visible text, panel content) and keep the
   screenshots as evidence.

## Rules

- The data behind a screen is cheaper to test at the feed
  (`ktw-compose:/skill/test-terminal-protocol`) or the catalog
  (`ktw-compose:/skill/test-cli-probe`) — use this method for the
  presentation layer, not for data correctness.
- GUI runs are the slowest loop: start on an address and script the
  click-path so re-runs are cheap.
