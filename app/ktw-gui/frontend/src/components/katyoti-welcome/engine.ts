// SPDX-License-Identifier: Apache-2.0
/* KaTyoti canvas engine: starfield + light effects + connectors + flowing particles. */
import { CONNECTORS, BRAIN_STREAM, CORE, W, H, Connector } from './data';

const PAL: Record<string, [string, string]> = {
  team: ['#ffcf6b', '#e0a93f'],
  agent: ['#fff0c2', '#ffd27a'],
  delivery: ['#ffd98a', '#f0b24a'],
  task: ['#ffe0a0', '#e9b24a'],
  core: ['#ffe9b0', '#f3c049'],
};
const clamp = (v: number, a: number, b: number) => Math.max(a, Math.min(b, v));
const ease = (t: number) => t * t * (3 - 2 * t);
const lerp = (a: number, b: number, t: number) => a + (b - a) * t;
const rnd = (a: number, b: number) => a + Math.random() * (b - a);

const SCHED: Record<string, { start: number; stagger: number; dur: number }> = {
  team: { start: 0.95, stagger: 0.12, dur: 0.62 },
  agent: { start: 1.55, stagger: 0.14, dur: 0.60 },
  delivery: { start: 2.15, stagger: 0.10, dur: 0.55 },
  task: { start: 1.30, stagger: 0.00, dur: 0.55 },
};

interface Part { p: number; sp: number; dir: number; sz: number; }
interface Conn extends Connector {
  _from: [number, number]; _to: [number, number];
  revealStart: number; revealDur: number; parts: Part[]; emitAcc: number;
}
interface Star { x: number; y: number; r: number; a: number; tw: number; ph: number; gold: boolean; vx: number; vy: number; }

export class CanvasScene {
  private canvas: HTMLCanvasElement;
  private ctx: CanvasRenderingContext2D;
  private reduced: boolean;
  private t0: number;
  private last: number;
  private stars: Star[] = [];
  private mesh: [Star, Star, number][] = [];
  private rings: { r0: number; t: number; max: number }[] = [];
  private brainP: { p: number; sp: number; off: number; sz: number }[] = [];
  private conns: Conn[] = [];
  private cw = 2; private ch = 2; private dpr = 1; private sx = 1; private sy = 1;
  private raf = 0; private lastRing = 0; private brainAcc = 0;
  private ro?: ResizeObserver;

  constructor(canvas: HTMLCanvasElement) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d')!;
    this.reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    this.t0 = performance.now();
    this.last = this.t0;
    this.buildConnectors();
    this.resize();
    this.ro = new ResizeObserver(() => this.resize());
    this.ro.observe(canvas.parentElement || canvas);
    this.raf = requestAnimationFrame(this.loop);
  }

  destroy() { cancelAnimationFrame(this.raf); this.ro?.disconnect(); }

  private buildConnectors() {
    const groups: Record<string, number> = {};
    const coreInset = 110;
    const nodeInset: Record<string, number> = { team: 12, agent: 52, delivery: 30, task: 6 };
    const isCore = (a: [number, number]) => a[0] === CORE[0] && a[1] === CORE[1];
    const inset = (a: [number, number], other: [number, number], amt: number): [number, number] => {
      const dx = other[0] - a[0], dy = other[1] - a[1], L = Math.hypot(dx, dy) || 1;
      return [a[0] + dx / L * amt, a[1] + dy / L * amt];
    };
    this.conns = CONNECTORS.map((c) => {
      groups[c.group] = groups[c.group] || 0;
      const idx = groups[c.group]++;
      const s = SCHED[c.group] || SCHED.team;
      const fAmt = isCore(c.from) ? coreInset : (nodeInset[c.group] ?? 10);
      const tAmt = isCore(c.to) ? coreInset : (nodeInset[c.group] ?? 10);
      return { ...c, _from: inset(c.from, c.to, fAmt), _to: inset(c.to, c.from, tAmt),
        revealStart: s.start + idx * s.stagger, revealDur: s.dur, parts: [], emitAcc: 0 } as Conn;
    });
  }

  private resize() {
    const par = this.canvas.parentElement || this.canvas;
    const rect = par.getBoundingClientRect();
    const cw = Math.max(2, rect.width), ch = Math.max(2, rect.height);
    const dpr = Math.min(window.devicePixelRatio || 1, 2);
    this.canvas.width = Math.round(cw * dpr);
    this.canvas.height = Math.round(ch * dpr);
    this.canvas.style.width = cw + 'px';
    this.canvas.style.height = ch + 'px';
    this.cw = cw; this.ch = ch; this.dpr = dpr;
    this.sx = cw / W; this.sy = ch / H;
    this.initStars();
  }

  private initStars() {
    const n = Math.round(clamp((this.cw * this.ch) / 13000, 90, 220));
    this.stars = [];
    for (let i = 0; i < n; i++) {
      this.stars.push({ x: Math.random(), y: Math.random(), r: rnd(0.4, 1.7), a: rnd(0.05, 0.5),
        tw: rnd(0.4, 1.6), ph: rnd(0, Math.PI * 2), gold: Math.random() < 0.35,
        vx: rnd(-0.004, 0.004), vy: rnd(-0.004, 0.004) });
    }
    this.mesh = [];
    const pts = this.stars.filter((_, i) => i % 2 === 0);
    const maxD = 0.11;
    for (let i = 0; i < pts.length; i++) {
      for (let j = i + 1; j < pts.length; j++) {
        const dx = pts[i].x - pts[j].x, dy = (pts[i].y - pts[j].y) * (this.ch / this.cw);
        const d = Math.hypot(dx, dy);
        if (d < maxD) this.mesh.push([pts[i], pts[j], 1 - d / maxD]);
      }
    }
  }

  private m(px: [number, number]) { return { x: px[0] * this.sx, y: px[1] * this.sy }; }

  private pathPoint(c: Conn, p: number) {
    const f = this.m(c._from), t = this.m(c._to);
    const mx = (f.x + t.x) / 2, my = (f.y + t.y) / 2;
    const dx = t.x - f.x, dy = t.y - f.y, len = Math.hypot(dx, dy) || 1;
    const nx = -dy / len, ny = dx / len;
    const cs = (c.curve || 0) * this.sx;
    const cx = mx + nx * cs, cy = my + ny * cs;
    const u = 1 - p;
    return { x: u * u * f.x + 2 * u * p * cx + p * p * t.x, y: u * u * f.y + 2 * u * p * cy + p * p * t.y };
  }

  private loop = (now: number) => {
    const dt = Math.min(0.05, (now - this.last) / 1000);
    this.last = now;
    this.draw((now - this.t0) / 1000, dt);
    this.raf = requestAnimationFrame(this.loop);
  };

  private draw(t: number, dt: number) {
    const ctx = this.ctx, cw = this.cw, ch = this.ch;
    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.clearRect(0, 0, cw, ch);
    ctx.fillStyle = '#040303';
    ctx.fillRect(0, 0, cw, ch);

    const core = this.m(CORE);
    const bloom = (0.5 + 0.5 * Math.sin(t * 0.8)) * 0.18 + 0.5;
    const ignite = ease(clamp(t / 0.9, 0, 1));
    const g = ctx.createRadialGradient(core.x, core.y, 0, core.x, core.y, Math.max(cw, ch) * 0.46);
    g.addColorStop(0, `rgba(128,92,28,${0.34 * bloom * ignite})`);
    g.addColorStop(0.3, `rgba(66,46,14,${0.16 * ignite})`);
    g.addColorStop(0.66, 'rgba(14,10,3,0.05)');
    g.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, cw, ch);

    ctx.globalCompositeOperation = 'lighter';
    for (const m of this.mesh) {
      const a = m[2] * 0.05 * ignite;
      if (a < 0.005) continue;
      ctx.strokeStyle = `rgba(180,140,60,${a})`;
      ctx.lineWidth = 0.5;
      ctx.beginPath();
      ctx.moveTo(m[0].x * cw, m[0].y * ch);
      ctx.lineTo(m[1].x * cw, m[1].y * ch);
      ctx.stroke();
    }
    for (const s of this.stars) {
      if (!this.reduced) {
        s.x += s.vx * dt; s.y += s.vy * dt;
        if (s.x < 0) s.x += 1; if (s.x > 1) s.x -= 1;
        if (s.y < 0) s.y += 1; if (s.y > 1) s.y -= 1;
      }
      const tw = 0.55 + 0.45 * Math.sin(t * s.tw + s.ph);
      const a = s.a * tw * ignite;
      ctx.fillStyle = s.gold ? `rgba(255,200,90,${a})` : `rgba(255,245,225,${a * 0.8})`;
      ctx.beginPath(); ctx.arc(s.x * cw, s.y * ch, s.r, 0, 7); ctx.fill();
    }

    for (const c of this.conns) {
      const rev = ease(clamp((t - c.revealStart) / c.revealDur, 0, 1));
      if (rev > 0) this.drawConnector(c, rev, t, dt);
    }
    this.drawBrainStream(t, dt);
    this.drawRings(t, dt, core);
    ctx.globalCompositeOperation = 'source-over';
  }

  private drawConnector(c: Conn, rev: number, t: number, dt: number) {
    const ctx = this.ctx;
    const pal = PAL[c.group] || PAL.team;
    const N = 26;
    const upto = Math.max(2, Math.floor(N * rev));
    ctx.globalCompositeOperation = 'lighter';
    if (c.dotted) { ctx.setLineDash([2, 7]); ctx.lineDashOffset = -t * 22; }

    ctx.strokeStyle = pal[1]; ctx.lineWidth = 4.5; ctx.globalAlpha = 0.16;
    ctx.beginPath();
    for (let i = 0; i <= upto; i++) {
      const pt = this.pathPoint(c, (i / upto) * rev);
      i === 0 ? ctx.moveTo(pt.x, pt.y) : ctx.lineTo(pt.x, pt.y);
    }
    ctx.stroke();

    ctx.strokeStyle = pal[0]; ctx.lineWidth = c.dotted ? 1.4 : 1.9; ctx.globalAlpha = c.dotted ? 0.7 : 0.92;
    ctx.beginPath();
    for (let i = 0; i <= upto; i++) {
      const pt = this.pathPoint(c, (i / upto) * rev);
      i === 0 ? ctx.moveTo(pt.x, pt.y) : ctx.lineTo(pt.x, pt.y);
    }
    ctx.stroke();

    if (!c.dotted) {
      ctx.strokeStyle = '#fff2d4'; ctx.lineWidth = 0.8; ctx.globalAlpha = 0.55;
      ctx.beginPath();
      for (let i = 0; i <= upto; i++) {
        const pt = this.pathPoint(c, (i / upto) * rev);
        i === 0 ? ctx.moveTo(pt.x, pt.y) : ctx.lineTo(pt.x, pt.y);
      }
      ctx.stroke();
    }
    ctx.setLineDash([]); ctx.globalAlpha = 1;

    if (rev > 0.85) {
      const e = this.pathPoint(c, 1);
      const pulse = 0.6 + 0.4 * Math.sin(t * 2 + c.from[0]);
      const r = Math.max(6, 7 * this.sx * pulse * 2.2);
      const gg = ctx.createRadialGradient(e.x, e.y, 0, e.x, e.y, r);
      gg.addColorStop(0, pal[0]); gg.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.globalAlpha = 0.5; ctx.fillStyle = gg;
      ctx.beginPath(); ctx.arc(e.x, e.y, r, 0, 7); ctx.fill();
      ctx.globalAlpha = 1;
    }

    if (rev > 0.55 && !this.reduced) {
      const target = c.dotted ? 3 : 4;
      c.emitAcc += dt;
      if (c.parts.length < target && c.emitAcc > 0.28) {
        c.emitAcc = 0;
        const reverse = (c.group === 'team' || c.group === 'agent') && Math.random() < 0.18;
        c.parts.push({ p: reverse ? 1 : 0, sp: rnd(0.28, 0.5), dir: reverse ? -1 : 1, sz: rnd(1.3, 2.4) });
      }
      for (let i = c.parts.length - 1; i >= 0; i--) {
        const pt = c.parts[i];
        pt.p += pt.sp * dt * pt.dir;
        if (pt.p > 1 || pt.p < 0) { c.parts.splice(i, 1); continue; }
        const pos = this.pathPoint(c, pt.p);
        const back = this.pathPoint(c, clamp(pt.p - 0.05 * pt.dir, 0, 1));
        const grd = ctx.createLinearGradient(back.x, back.y, pos.x, pos.y);
        grd.addColorStop(0, 'rgba(255,210,110,0)'); grd.addColorStop(1, pal[0]);
        ctx.strokeStyle = grd; ctx.lineWidth = pt.sz; ctx.globalAlpha = 0.85;
        ctx.beginPath(); ctx.moveTo(back.x, back.y); ctx.lineTo(pos.x, pos.y); ctx.stroke();
        const hr = pt.sz * 1.6 * 2.6;
        const hg = ctx.createRadialGradient(pos.x, pos.y, 0, pos.x, pos.y, hr);
        hg.addColorStop(0, '#fff3d6'); hg.addColorStop(0.4, pal[0]); hg.addColorStop(1, 'rgba(0,0,0,0)');
        ctx.fillStyle = hg; ctx.globalAlpha = 1;
        ctx.beginPath(); ctx.arc(pos.x, pos.y, hr, 0, 7); ctx.fill();
      }
      ctx.globalAlpha = 1;
    }
  }

  private drawBrainStream(t: number, dt: number) {
    if (t < 1.25) return;
    const ctx = this.ctx;
    const f = this.m(BRAIN_STREAM.from), to = this.m(BRAIN_STREAM.to);
    if (!this.reduced) {
      this.brainAcc += dt;
      const intens = clamp((t - 1.25) / 0.8, 0, 1);
      if (this.brainAcc > 0.045 && this.brainP.length < 60 * intens) {
        this.brainAcc = 0;
        this.brainP.push({ p: 0, sp: rnd(0.35, 0.7), off: rnd(-1, 1), sz: rnd(1, 2.3) });
      }
    }
    ctx.globalCompositeOperation = 'lighter';
    for (let i = this.brainP.length - 1; i >= 0; i--) {
      const pp = this.brainP[i];
      pp.p += pp.sp * dt;
      if (pp.p > 1) { this.brainP.splice(i, 1); continue; }
      const e = ease(pp.p);
      const spread = (1 - pp.p) * 26 * this.sx;
      const x = lerp(f.x, to.x, e) + pp.off * spread + Math.sin(pp.p * 6 + pp.off) * 4 * this.sx;
      const y = lerp(f.y, to.y, e);
      const a = Math.sin(pp.p * Math.PI) * 0.9;
      const r = pp.sz * 1.8 * 2.4;
      const hg = ctx.createRadialGradient(x, y, 0, x, y, r);
      hg.addColorStop(0, `rgba(255,235,180,${a})`); hg.addColorStop(0.5, `rgba(245,190,80,${a * 0.7})`); hg.addColorStop(1, 'rgba(0,0,0,0)');
      ctx.fillStyle = hg;
      ctx.beginPath(); ctx.arc(x, y, r, 0, 7); ctx.fill();
    }
  }

  private drawRings(t: number, dt: number, core: { x: number; y: number }) {
    const ctx = this.ctx;
    if (t - this.lastRing > 3.4 && t > 0.4) {
      this.lastRing = t;
      this.rings.push({ r0: 60 * this.sx, t: 0, max: 230 * this.sx });
    }
    for (let i = this.rings.length - 1; i >= 0; i--) {
      const rg = this.rings[i];
      rg.t += dt * 0.5;
      if (rg.t > 1) { this.rings.splice(i, 1); continue; }
      const e = ease(rg.t);
      const r = lerp(rg.r0, rg.max, e);
      ctx.strokeStyle = `rgba(245,196,73,${(1 - rg.t) * 0.5})`;
      ctx.lineWidth = lerp(2.4, 0.4, e);
      ctx.beginPath(); ctx.arc(core.x, core.y, r, 0, 7); ctx.stroke();
    }
  }
}
