// SPDX-License-Identifier: Apache-2.0
// Test harness shell — two-pane page served at /test when --test flag is on.
//
// Activation: ktw-display --test (CLI flag only; no env var fallback)
//
// Layout: left tree of test cases + right pane with iframe + sticky outcome
// strip. Cases populate the tree via /test/api/cases (subtask 2). Each case is
// addressable via /test/<category>/<case> (subtask 3) and gets rendered via
// the shared schema-walker in widgets/_lib/render-schema.ts.
//
// Case format: modules/display/yaml-test-cases.md.

const HARNESS_HTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>ktw-display test harness</title>
  <style>
    * { box-sizing: border-box; }
    html, body { margin: 0; padding: 0; height: 100%; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      background: #fafafa; color: #1a1a1a;
      display: grid;
      grid-template-columns: 280px 1fr;
      grid-template-rows: 48px 1fr 200px;
      grid-template-areas:
        "header header"
        "tree   pane"
        "tree   outcome";
      height: 100vh;
    }
    header {
      grid-area: header;
      padding: 0 16px;
      background: #1a1a1a; color: #f0f0f0;
      display: flex; align-items: center; gap: 12px;
      border-bottom: 1px solid #333;
    }
    header h1 { font-size: 14px; font-weight: 600; margin: 0; }
    header .meta { font-size: 11px; color: #999; }
    header .badge {
      font-size: 10px; padding: 2px 6px; border-radius: 3px;
      background: #4a90e2; color: white;
    }
    aside.tree {
      grid-area: tree;
      background: white;
      border-right: 1px solid #e0e0e0;
      overflow-y: auto;
      padding: 12px;
    }
    aside.tree h2 { font-size: 12px; text-transform: uppercase; color: #6c757d; margin: 16px 0 6px 0; font-weight: 600; }
    aside.tree h2:first-child { margin-top: 0; }
    aside.tree .case {
      display: block; padding: 6px 10px; border-radius: 4px;
      cursor: pointer; font-size: 13px; color: #1a1a1a;
      text-decoration: none;
    }
    aside.tree .case:hover { background: #f1f3f5; }
    aside.tree .case.active { background: #e7f5ff; color: #1971c2; font-weight: 500; }
    aside.tree .empty { font-size: 12px; color: #6c757d; font-style: italic; padding: 12px; }
    main.pane {
      grid-area: pane;
      background: white;
      overflow: hidden;
      display: flex; flex-direction: column;
    }
    main.pane .breadcrumb {
      padding: 8px 16px;
      font-size: 12px; color: #6c757d;
      border-bottom: 1px solid #e0e0e0;
      background: #f8f9fa;
    }
    main.pane iframe {
      flex: 1; width: 100%; border: 0; background: white;
    }
    section.outcome {
      grid-area: outcome;
      background: #1a1a1a;
      color: #e0e0e0;
      border-top: 1px solid #333;
      padding: 8px 16px;
      overflow: auto;
      font-family: ui-monospace, SFMono-Regular, monospace;
      font-size: 11px;
    }
    section.outcome h3 {
      margin: 0 0 6px 0; font-size: 11px; color: #999; font-weight: 600; text-transform: uppercase;
      letter-spacing: 0.5px;
    }
    section.outcome .empty { color: #666; font-style: italic; }
    section.outcome .event { margin-bottom: 6px; padding: 4px 6px; background: #2a2a2a; border-radius: 3px; }
    section.outcome .event .ts { color: #6cb6ff; }
    section.outcome .event .label { color: #f5b400; font-weight: 600; }
  </style>
</head>
<body>
  <header>
    <h1>ktw-display test harness</h1>
    <span class="badge" data-testid="harness.mode">--test</span>
    <span class="meta">Live spec-by-example for the data-DSL catalog · interactive showcase + browser-automation E2E surface</span>
  </header>

  <aside class="tree" data-testid="harness.tree">
    <div class="empty">Loading cases...</div>
  </aside>

  <main class="pane" data-testid="harness.frame">
    <div class="breadcrumb" data-testid="harness.breadcrumb" id="breadcrumb">No case selected</div>
    <iframe
      name="harness-frame"
      id="harness-frame"
      data-testid="harness.iframe"
      title="Test case content"
      srcdoc="&lt;body style='font-family:-apple-system,sans-serif;color:#6c757d;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;text-align:center;font-size:14px'&gt;&lt;div&gt;&lt;p&gt;Pick a test case from the tree.&lt;/p&gt;&lt;p style='font-size:12px;margin-top:16px'&gt;Cases live at &lt;code style='background:#f1f3f5;padding:2px 6px;border-radius:3px;font-family:ui-monospace,monospace;font-size:11px'&gt;app/ktw-display/src/test-cases/&amp;lt;category&amp;gt;/&amp;lt;case&amp;gt;.ts&lt;/code&gt;&lt;/p&gt;&lt;/div&gt;&lt;/body&gt;"
    ></iframe>
  </main>

  <section class="outcome" data-testid="harness.outcome">
    <h3>Outcome</h3>
    <div id="outcome-events" class="empty">no events yet — interact with a case to see postMessage flow</div>
  </section>

  <script>
    // Subtask 1 placeholder JS — populated in subtasks 2 (case enumeration)
    // and 3 (per-case routing). For now the tree shows a loading state
    // and the right pane shows a static placeholder.

    const treeEl = document.querySelector('[data-testid="harness.tree"]');
    const eventsEl = document.getElementById('outcome-events');
    const breadcrumb = document.querySelector('[data-testid="harness.breadcrumb"]');

    // Stub: in subtask 2 this fetches /test/api/cases and renders the tree.
    fetch('/test/api/cases')
      .then(r => r.json())
      .then(cases => renderTree(cases))
      .catch(err => {
        treeEl.innerHTML = '<div class="empty">No /test/api/cases endpoint yet — see subtask 2 (case enumeration)</div>';
      });

    function renderTree(cases) {
      if (!Array.isArray(cases) || cases.length === 0) {
        treeEl.innerHTML = '<div class="empty">No test cases found. Drop a .ts file in src/test-cases/&lt;category&gt;/.</div>';
        return;
      }
      // Group by category
      const byCategory = {};
      cases.forEach(c => {
        if (!byCategory[c.category]) byCategory[c.category] = [];
        byCategory[c.category].push(c);
      });
      treeEl.innerHTML = '';
      Object.keys(byCategory).sort().forEach(cat => {
        const h = document.createElement('h2');
        h.textContent = cat;
        treeEl.appendChild(h);
        byCategory[cat].sort((a, b) => a.title.localeCompare(b.title)).forEach(c => {
          const a = document.createElement('a');
          a.className = 'case';
          a.href = c.url;
          a.target = 'harness-frame'; // load into the iframe instead of replacing the whole page
          a.textContent = c.title;
          a.setAttribute('data-testid', 'case.' + c.id);
          a.dataset.caseId = c.id;
          a.dataset.caseTitle = c.title;
          a.dataset.caseCategory = c.category;
          a.addEventListener('click', () => {
            // Update breadcrumb + active highlight on click. The actual
            // iframe navigation happens via target="harness-frame".
            breadcrumb.textContent = c.category + ' / ' + c.title;
            treeEl.querySelectorAll('a.case.active').forEach(el => el.classList.remove('active'));
            a.classList.add('active');
          });
          treeEl.appendChild(a);
        });
      });
    }

    // Listen for postMessage events from the case iframe (when subtask 3 lands)
    window.addEventListener('message', (event) => {
      const data = event.data;
      if (!data || typeof data !== 'object') return;
      const ts = new Date().toISOString().slice(11, 19);
      const label = data.type || 'event';
      const payload = JSON.stringify(data);
      if (eventsEl.classList.contains('empty')) {
        eventsEl.classList.remove('empty');
        eventsEl.innerHTML = '';
      }
      const div = document.createElement('div');
      div.className = 'event';
      div.innerHTML = '<span class="ts">' + ts + '</span> <span class="label">' + label + '</span> ' + escapeHtml(payload);
      eventsEl.insertBefore(div, eventsEl.firstChild);
      // cap at 10 events
      while (eventsEl.children.length > 10) eventsEl.removeChild(eventsEl.lastChild);
    });

    function escapeHtml(s) {
      return s.replace(/[&<>]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;'}[c]));
    }
  </script>
</body>
</html>`

/**
 * harnessHTML returns the test harness shell. Served at /test when the
 * --test CLI flag is active.
 */
export function harnessHTML(_req: Request): Response {
  return new Response(HARNESS_HTML, {
    status: 200,
    headers: {
      'content-type': 'text/html; charset=utf-8',
      'cache-control': 'no-cache',
    },
  })
}
