// Page-transition latency for the staff app: clicks each sidebar destination from another page (a soft navigation,
// as a user does) and times click -> URL changed and the new page's h1 rendered. Background reads after that (the
// sidebar counts) are left out of the timed window.
// Usage: node nav-latency.mjs <base url> <samples per transition> <out.json>
import { chromium } from "@playwright/test";
import { writeFileSync } from "node:fs";
const [base, nArg, out] = process.argv.slice(2);
const N = Number(nArg ?? 20);
const TARGETS = [
  ["Programme", "/app"],
  ["Today", "/app/today"],
  ["To call", "/app/today?queue=call"],
  ["Waiting", "/app/today?queue=waiting"],
  ["No owner", "/app/today?queue=none"],
  ["Patients", "/app/patients"],
  ["Referrals", "/app/referrals"],
  ["Recovery", "/app/recovery"],
  ["Review desk", "/app/review"],
  ["Audit trail", "/app/audit"],
  ["Pipeline", "/app/pipeline"],
  ["Results", "/app/results"],
];
const pct = (xs, p) => { const s = [...xs].sort((a, b) => a - b); return s[Math.min(s.length - 1, Math.ceil((p / 100) * s.length) - 1)]; };
const b = await chromium.launch();
const ctx = await b.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();
const results = {};
const hard = {};
for (const [name, path] of TARGETS) {
  results[name] = [];
  hard[name] = [];
}
for (let i = 0; i < N; i++) {
  for (let t = 0; t < TARGETS.length; t++) {
    const [name, path] = TARGETS[t];
    // Start from a different page each time (the previous target), already loaded.
    const [, from] = TARGETS[(t + TARGETS.length - 1) % TARGETS.length];
    const fromUrl = from === "/app/audit" ? "/app/patients" : from;
    if (!page.url().startsWith(base) || (i === 0 && t === 0)) await page.goto(base + fromUrl, { waitUntil: "networkidle" });
    const link = page.locator("aside#app-sidebar").getByRole("link", { name: new RegExp("^" + name.replace(" ", "\\s") + "(\\s|$|\\d)") }).first();
    const before = page.url();
    const t0 = performance.now();
    try {
      await link.click({ timeout: 10000 });
      await page.waitForURL((u) => u.toString() !== before || path === "/app/today?queue=call", { timeout: 30000 });
      await page.waitForFunction(() => {
        const h1 = document.querySelector("main h1");
        return h1 && !document.querySelector("[data-loading], [aria-busy='true']");
      }, null, { timeout: 30000 });
      results[name].push(Math.round(performance.now() - t0));
      // Let background reads (the sidebar counts) settle before the next click, outside the timed window.
      await page.waitForLoadState("networkidle", { timeout: 30000 }).catch(() => {});
    } catch (e) {
      results[name].push(30000);
      console.error(name, "timeout", e.message.slice(0, 80));
      await page.goto(base + "/app", { waitUntil: "networkidle" });
    }
  }
  // One full page load (hard navigation) per destination per round: the server render a cold visit pays.
  for (const [name, path] of TARGETS) {
    if (path === "/app/audit") continue;
    const t0 = performance.now();
    await page.goto(base + path, { waitUntil: "domcontentloaded" });
    await page.waitForSelector("main h1", { timeout: 30000 }).catch(() => {});
    hard[name].push(Math.round(performance.now() - t0));
  }
  console.error(`round ${i + 1}/${N}`);
}
await b.close();
const summary = (m) => Object.fromEntries(Object.entries(m).filter(([, xs]) => xs.length).map(([k, xs]) => [k, { n: xs.length, p50: pct(xs, 50), p95: pct(xs, 95), p99: pct(xs, 99), max: Math.max(...xs) }]));
const res = { base, samples: N, at: new Date().toISOString(), click: summary(results), fullLoad: summary(hard), raw: { click: results, fullLoad: hard } };
writeFileSync(out, JSON.stringify(res, null, 1));
const row = (k, v) => `${k.padEnd(12)} n=${String(v.n).padStart(3)}  p50 ${String(v.p50).padStart(6)}  p95 ${String(v.p95).padStart(6)}  p99 ${String(v.p99).padStart(6)}  max ${String(v.max).padStart(6)}`;
console.log("Sidebar click -> page rendered (ms)");
for (const [k, v] of Object.entries(res.click)) console.log(row(k, v));
console.log("Full page load -> h1 (ms)");
for (const [k, v] of Object.entries(res.fullLoad)) console.log(row(k, v));
