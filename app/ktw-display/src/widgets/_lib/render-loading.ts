// SPDX-License-Identifier: Apache-2.0
// render-loading — the shared "assistant is working" loading screen for
// any widget route reached BEFORE its content has streamed in.
//
// Per US-S3 UX feedback (2026-06-12) a widget route reached without its
// injected payload (`?schema=` / `?page=` / `?value=`) must NEVER render
// fabricated demo data — the flash of fake content reads as a bug. This
// module renders a single, high-level graphical loading moment instead:
// the KaTyoti robot mascot inside a rotating golden energy halo over a
// dark-amber stage, with a caption explaining the assistant is thinking
// and streaming the result live, the number of bytes streamed so far, and
// a rotating (lighthearted) activity line.
//
// Reusable across every widget kind. The streaming consumer (DocumentTab
// via UIPage/UIForm/…) reaches the route with `?loading=1` while content
// isn't ready; the route swaps in the real payload once it anchors and
// this screen unmounts. Dev/debug fixtures (which mount WITHOUT
// `?loading=1`) are unaffected.
//
// Live byte count: the host already pushes the partial payload to every
// mounted iframe via a `{type:'value'}` postMessage (IframeWidget's
// pushValue effect) once we post `ready`. This screen listens for those
// and reports the largest payload size seen — no host-side plumbing.

import { MASCOT_DATA_URI } from '@ktw/display-lib/ui-core/loading/mascot-data.ts'

/** Human noun per widget kind — drives the caption ("building your …"). */
function kindNoun(kind: string): string {
  switch (kind) {
    case 'page':   return 'interface'
    case 'form':   return 'form'
    case 'table':  return 'table'
    case 'tree-table': return 'tree'
    case 'view':   return 'view'
    case 'wizard': return 'wizard'
    case 'drawio': return 'diagram'
    case 'load':   return 'content'
    default:       return 'content'
  }
}

/** Lighthearted activity lines cycled while the assistant streams. Kept
 *  tame + on-brand; one is picked at random every few seconds. */
const ACTIVITIES: string[] = [
  'Waking up the robots',
  'Untangling the neural spaghetti',
  'Polishing the gold trim',
  'Teaching pixels to behave',
  'Negotiating with the stream',
  'Summoning components from the void',
  'Aligning the layout chakras',
  'Convincing electrons to cooperate',
  'Reticulating splines',
  'Warming up the hamster wheel',
  'Herding async cats',
  'Bribing the render pipeline',
  'Counting bytes, twice for luck',
  'Consulting the rubber duck',
  'Feeding the AI a fresh coffee',
  'Greasing the widget hinges',
  'Tuning the golden frequencies',
  'Folding the data origami',
]

export interface LoadingOptions {
  /** Overrides the default "Building your <noun>" headline. */
  title?: string
  /** Overrides the default streaming explainer subtitle. */
  subtitle?: string
}

const LOADING_CSS = `
  :root { color-scheme: dark; }
  * { box-sizing: border-box; }
  html, body { margin: 0; height: 100%; }
  body {
    font-family: 'Montserrat', system-ui, -apple-system, Segoe UI, sans-serif;
    color: #f3e9d6;
    overflow: hidden;
    background:
      radial-gradient(120% 90% at 50% 38%, #2a1d08 0%, #181206 42%, #0c0905 100%);
  }
  .stage {
    position: fixed; inset: 0;
    display: flex; flex-direction: column;
    align-items: center; justify-content: center;
    gap: 26px; padding: 24px; text-align: center;
  }
  .stage::before {
    content: ''; position: absolute; inset: -20%;
    background:
      radial-gradient(40% 40% at 18% 22%, rgba(245,177,61,0.10), transparent 70%),
      radial-gradient(45% 45% at 82% 80%, rgba(245,177,61,0.08), transparent 70%);
    animation: drift 9s ease-in-out infinite alternate;
    pointer-events: none;
  }
  @keyframes drift { from { transform: translate3d(-2%, -1%, 0); } to { transform: translate3d(2%, 1%, 0); } }

  /* --- Mascot + animated halo --- */
  .orb { position: relative; width: 300px; height: 300px; }
  .ring {
    position: absolute; inset: 0; border-radius: 50%;
    -webkit-mask: radial-gradient(closest-side, transparent 70%, #000 72%, #000 93%, transparent 96%);
            mask: radial-gradient(closest-side, transparent 70%, #000 72%, #000 93%, transparent 96%);
    filter: blur(0.6px); mix-blend-mode: screen;
  }
  .ring.a {
    background: conic-gradient(from 0deg, transparent 0deg, #f5b13d 55deg, #ffe7a8 90deg, transparent 150deg, transparent 360deg);
    animation: spin 4.6s linear infinite; opacity: .8;
  }
  .ring.b {
    background: conic-gradient(from 180deg, transparent 0deg, #c8841f 45deg, #ffd680 80deg, transparent 140deg, transparent 360deg);
    animation: spinB 6.4s linear infinite; opacity: .55; transform: scale(.9);
  }
  @keyframes spin  { to { transform: rotate(360deg); } }
  @keyframes spinB { from { transform: scale(.9) rotate(0); } to { transform: scale(.9) rotate(-360deg); } }

  .glow {
    position: absolute; inset: 24%; border-radius: 50%;
    background: radial-gradient(closest-side, rgba(245,177,61,0.4), rgba(245,177,61,0.05) 70%, transparent 75%);
    animation: breathe 2.8s ease-in-out infinite;
  }
  @keyframes breathe { 0%,100% { opacity: .5; transform: scale(.92); } 50% { opacity: 1; transform: scale(1.06); } }

  .orbit { position: absolute; inset: 0; animation: spin 7.5s linear infinite; }
  .orbit.slow { animation-duration: 12s; animation-direction: reverse; }
  .dot {
    position: absolute; top: 50%; left: 50%; width: 6px; height: 6px; border-radius: 50%;
    background: #ffe7a8; box-shadow: 0 0 8px 2px rgba(245,177,61,0.9); margin: -3px 0 0 -3px;
  }
  .dot.d1 { transform: translate(140px, 0); }
  .dot.d2 { transform: rotate(140deg) translate(146px, 0) scale(.7); }
  .dot.d3 { transform: rotate(255deg) translate(134px, 0) scale(.85); }

  /* The mascot raster — centered, full-quality, floating bob + gold glow. */
  .bot {
    position: absolute; inset: 6%;
    background: center/contain no-repeat;
    filter: drop-shadow(0 10px 22px rgba(0,0,0,0.5)) drop-shadow(0 0 18px rgba(245,177,61,0.3));
    animation: bob 3.4s ease-in-out infinite;
  }
  @keyframes bob {
    0%,100% { transform: translateY(4px) scale(0.99); }
    50%     { transform: translateY(-7px) scale(1.01); }
  }

  /* --- Caption --- */
  .copy { max-width: 480px; display: flex; flex-direction: column; align-items: center; gap: 9px; }
  .title { font-size: 19px; font-weight: 600; letter-spacing: .2px; color: #ffe7a8; }
  .title b { color: #f5b13d; font-weight: 700; }
  .sub { font-size: 13.5px; line-height: 1.5; color: #c9b894; }

  .pill {
    display: inline-flex; align-items: center; gap: 7px;
    padding: 4px 12px; border-radius: 999px;
    background: rgba(245,177,61,0.12); border: 1px solid rgba(245,177,61,0.28);
    font-size: 12.5px; color: #ffd98a; font-variant-numeric: tabular-nums;
  }
  .pill .blip { width: 7px; height: 7px; border-radius: 50%; background: #f5b13d;
    box-shadow: 0 0 7px 1px rgba(245,177,61,0.9); animation: blip 1.1s ease-in-out infinite; }
  @keyframes blip { 0%,100% { opacity: .35; } 50% { opacity: 1; } }

  .bar { position: relative; width: 240px; height: 4px; border-radius: 4px;
    background: rgba(245,177,61,0.14); overflow: hidden; }
  .bar::after { content: ''; position: absolute; top: 0; left: 0; height: 100%; width: 40%;
    border-radius: 4px; background: linear-gradient(90deg, transparent, #f5b13d, #ffe7a8, transparent);
    animation: sweep 1.5s ease-in-out infinite; }
  @keyframes sweep { 0% { left: -45%; } 100% { left: 105%; } }

  .act { font-size: 12.5px; color: #b9a988; min-height: 16px; }
  .act .dots::after { content: ''; animation: ellipsis 1.6s steps(4, end) infinite; }
  @keyframes ellipsis { 0% { content: ''; } 25% { content: '.'; } 50% { content: '..'; } 75% { content: '...'; } }

  @media (prefers-reduced-motion: reduce) {
    .ring.a, .ring.b, .orbit, .orbit.slow, .stage::before { animation: none; }
    .glow, .bot { animation-duration: 6s; }
    .bar::after, .pill .blip { animation-duration: 3s; }
  }
`

/** Build the animated loading Response for a widget route reached with
 *  `?loading=1` and no payload. `kind` drives the caption noun; `opts`
 *  overrides the headline / subtitle for callers wanting bespoke copy. */
export function renderLoadingResponse(kind: string, opts: LoadingOptions = {}): Response {
  const noun = kindNoun(kind)
  const title = opts.title ?? `KaTyoti is building your <b>${noun}</b>`
  const subtitle =
    opts.subtitle ??
    'The assistant is thinking and streaming the result — it appears here live as it arrives.'

  const html = `<!doctype html>
<html lang="en"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>${LOADING_CSS}</style>
</head>
<body>
  <div class="stage" role="status" aria-live="polite" aria-label="Loading ${noun}">
    <div class="orb">
      <div class="glow"></div>
      <div class="ring a"></div>
      <div class="ring b"></div>
      <div class="orbit"><span class="dot d1"></span><span class="dot d2"></span></div>
      <div class="orbit slow"><span class="dot d3"></span></div>
      <div class="bot" style="background-image:url('${MASCOT_DATA_URI}')"></div>
    </div>
    <div class="copy">
      <div class="title">${title}</div>
      <div class="sub">${subtitle}</div>
      <div class="pill"><span class="blip"></span><span id="bytes">warming up…</span></div>
      <div class="bar" aria-hidden="true"></div>
      <div class="act"><span id="act">Waking up the robots</span><span class="dots"></span></div>
    </div>
  </div>
<script>
  (function () {
    var widgetId = new URLSearchParams(location.search).get('widgetId') || '';
    // Standard ready handshake so the host unhides the iframe at once —
    // and starts pushing the partial payload (value messages) to us.
    parent.postMessage({ type: 'ready', widget: ${JSON.stringify(kind)}, widgetId: widgetId, ts: Date.now() }, '*');

    // --- Live "bytes streamed so far" from the host's value pushes ---
    var bytesEl = document.getElementById('bytes');
    var enc = (typeof TextEncoder !== 'undefined') ? new TextEncoder() : null;
    var maxBytes = 0;
    function fmt(n) {
      if (n < 1024) return n + ' B streamed';
      if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB streamed';
      return (n / 1048576).toFixed(2) + ' MB streamed';
    }
    window.addEventListener('message', function (e) {
      var d = e && e.data;
      if (!d || typeof d !== 'object' || d.type !== 'value') return;
      if (d.widgetId && widgetId && d.widgetId !== widgetId) return;
      try {
        var s = JSON.stringify(d.value);
        if (s == null) return;
        var n = enc ? enc.encode(s).length : s.length;
        if (n > maxBytes) { maxBytes = n; bytesEl.textContent = fmt(n); }
      } catch (_) { /* unserializable — ignore */ }
    });

    // --- Rotating lighthearted activity line ---
    var ACTS = ${JSON.stringify(ACTIVITIES)};
    var actEl = document.getElementById('act');
    var last = -1;
    function tick() {
      var i = Math.floor(Math.random() * ACTS.length);
      if (i === last) i = (i + 1) % ACTS.length;
      last = i;
      actEl.textContent = ACTS[i];
    }
    setInterval(tick, 2600);
  })();
</script>
</body></html>`

  return new Response(html, {
    status: 200,
    headers: {
      'content-type': 'text/html; charset=utf-8',
      'cache-control': 'no-cache',
    },
  })
}
