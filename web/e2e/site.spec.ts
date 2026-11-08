import { readFileSync } from "node:fs";
import path from "node:path";
import AxeBuilder from "@axe-core/playwright";
import { expect, test, type Page } from "@playwright/test";
import { BEATS, CUES, TRACK_VH, WALK, pageAt } from "../components/story/script";
import { ROUTES as APP_ROUTES } from "./routes";

const ROUTES = [
  { path: "/", name: "home" },
  { path: "/how-it-works", name: "how-it-works" },
  { path: "/experts", name: "experts" },
] as const;

const WIDTHS = [
  { w: 1440, h: 900 },
  { w: 390, h: 844 },
] as const;

/** Every file the site may quote a number from. */
const CORPUS_FILES = [
  "web/data/results.json",
  "web/data/api/dashboard.json",
  "docs/BRIEF.md",
  "docs/PITCH.md",
  "docs/WRITEUP.md",
  "pipeline/results/final/SUMMARY.md",
  "pipeline/results/ltt/LTT.md",
];

/**
 * Birth years of the illustrative name-picker cast, the only numbers that are not measurements.
 * Codes glued to letters (v2.5.1, ORU^R01, e4c7553) and the HL7 sample are skipped by the scan.
 * Anything else on a page must appear in the corpus.
 */
const NOT_MEASUREMENTS = new Set(["1959", "1971", "1972", "1983"]);

const repo = path.resolve(__dirname, "../..");
const corpus = CORPUS_FILES.map((f) => readFileSync(path.join(repo, f), "utf8")).join("\n");
const corpusNumbers = new Set([...corpus.matchAll(/\d[\d,]*(?:\.\d+)?/g)].map((m) => m[0].replaceAll(",", "")));
const corpusValues = [...corpusNumbers].map(Number);

/** A shown number is sourced when the corpus has it, or (for decimals) a value that rounds to it. */
function sourced(token: string): boolean {
  const t = token.replaceAll(",", "");
  if (NOT_MEASUREMENTS.has(t) || corpusNumbers.has(t)) return true;
  const decimals = t.split(".")[1]?.length ?? 0;
  if (decimals === 0) return false;
  return corpusValues.some((v) => v.toFixed(decimals) === t || (v * 100).toFixed(decimals) === t);
}

async function visitAll(page: Page) {
  const height = await page.evaluate(() => document.body.scrollHeight);
  for (let y = 0; y < height; y += 600) await page.evaluate((top) => window.scrollTo(0, top), y);
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.waitForTimeout(600);
}

for (const vp of WIDTHS) {
  for (const route of ROUTES) {
    test(`${route.path} at ${vp.w}px: renders, fits, labels its honesty, passes axe`, async ({ page }) => {
      await page.setViewportSize({ width: vp.w, height: vp.h });
      const res = await page.goto(route.path);
      expect(res?.status()).toBe(200);
      await expect(page.locator("h1")).toHaveCount(1);
      await expect(page.locator("[data-honesty]").first()).toBeVisible();
      await expect(page.locator('a[href^="mailto:"]')).toHaveCount(0);

      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      expect(overflow, "no horizontal scroll").toBeLessThanOrEqual(0);

      await visitAll(page);
      await page.screenshot({ path: `test-results/screens/site-${route.name}-${vp.w}.png`, fullPage: true });

      const axe = await new AxeBuilder({ page }).analyze();
      const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
      expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
    });
  }
}

for (const route of ROUTES) {
  test(`${route.path}: every number on the page is in a source file`, async ({ page }) => {
    await page.goto(route.path);
    const text = await page.locator("main").evaluate((main) => {
      const copy = main.cloneNode(true) as HTMLElement;
      copy.querySelectorAll("pre").forEach((pre) => pre.remove());
      document.body.append(copy);
      const t = copy.innerText;
      copy.remove();
      return t;
    });
    const tokens = [...new Set([...text.matchAll(/(?<![\w^])\d[\d,]*(?:\.\d+)?/g)].map((m) => m[0].replace(/[.,]$/, "")))];
    expect(tokens.filter((t) => !sourced(t))).toEqual([]);
  });
}

/** The palette before sand and thread: blue #1d5fbf, near-black #0a0a0a, cool grey #f2f4f7, and pure black grounds. */
const OLD_PALETTE = ["rgb(29, 95, 191)", "rgb(10, 10, 10)", "rgb(242, 244, 247)"];

for (const route of ROUTES) {
  test(`${route.path}: nothing is painted in the old palette`, async ({ page }) => {
    await page.goto(route.path);
    await visitAll(page);
    const hits = await page.evaluate((old) => {
      const props = ["color", "background-color", "border-top-color", "border-bottom-color", "border-left-color", "border-right-color", "fill", "stroke", "outline-color"];
      const out: string[] = [];
      for (const el of document.querySelectorAll("body *")) {
        const cs = getComputedStyle(el);
        const name = `${el.tagName.toLowerCase()}${el.id ? `#${el.id}` : ""}.${String(el.getAttribute("class") ?? "").split(" ")[0]}`;
        for (const prop of props) {
          const v = cs.getPropertyValue(prop);
          if (old.includes(v)) out.push(`${name} ${prop} ${v}`);
        }
        if (cs.backgroundColor === "rgb(0, 0, 0)") out.push(`${name} background-color pure black`);
      }
      return out;
    }, OLD_PALETTE);
    expect(hits).toEqual([]);
  });
}

test("the header's primary action opens the dashboard from every site page", async ({ page }) => {
  for (const route of ROUTES) {
    await page.goto(route.path);
    const nav = page.getByRole("navigation", { name: "Primary" });
    await nav.getByRole("link", { name: "Open the dashboard" }).click();
    await expect(page).toHaveURL(/\/app$/);
    await expect(page.locator("h1")).toHaveCount(1);
  }
});

test("the pilot page is gone and no page links to it", async ({ page }) => {
  test.setTimeout(120_000);
  const res = await page.goto("/pilot");
  expect(res?.status()).toBe(404);
  for (const route of APP_ROUTES) {
    await page.goto(route);
    await expect(page.locator('a[href="/pilot"], a[href^="/pilot#"], a[href^="/pilot?"]'), route).toHaveCount(0);
  }
});

const results = JSON.parse(readFileSync(path.join(repo, "web/data/results.json"), "utf8"));
const closures = results.entries["v2.safety.crc_false_closures"];
const waitDays = results.entries["published.crc_followup_median_wait_days"].value;
const standardDays = results.entries["published.crc_followup_standard_days"].value;

/** Scrolls the 3D story to scroll progress `p` and waits for the damped camera and the copy to settle. */
async function scrollStory(page: Page, p: number) {
  await page.evaluate((q) => {
    const el = document.querySelector<HTMLElement>('[data-story="3d"]')!;
    const top = el.getBoundingClientRect().top + window.scrollY;
    window.scrollTo(0, top + q * (el.offsetHeight - window.innerHeight));
  }, pageAt(p));
  await page.waitForTimeout(700);
}

const onCues = (page: Page) => page.locator("[data-cue][data-on]");

/** `stillAt` stops the passers-by at that many seconds, for anything that compares pixels. */
async function openStory(page: Page, stillAt?: number) {
  await page.goto(stillAt === undefined ? "/?story=3d" : `/?story=3d&still=${stillAt}`);
  await expect(page.locator('[data-story="3d"] canvas')).toHaveCount(1);
  await page.waitForFunction(() => typeof window.__wasl3d === "function");
}

for (const vp of WIDTHS) {
  test(`landing at ${vp.w}px: every line of the story arrives and leaves, one at a time`, async ({ page }) => {
    test.setTimeout(150_000);
    await page.setViewportSize({ width: vp.w, height: vp.h });
    await openStory(page);
    for (const c of CUES) {
      await scrollStory(page, c.from + (c.to - c.from) * 0.55);
      await expect(onCues(page), c.id).toHaveCount(1);
      await expect(onCues(page)).toHaveAttribute("data-cue", c.id);
      await page.screenshot({ path: `test-results/screens/story-${c.id}-${vp.w}.png` });
      if (c.exit !== false) {
        await scrollStory(page, Math.min(1, c.to + 0.002));
        await expect(page.locator(`[data-cue="${c.id}"]`), `${c.id} leaves`).not.toHaveAttribute("data-on");
      }
    }
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, "no horizontal scroll").toBeLessThanOrEqual(0);
    const axe = await new AxeBuilder({ page }).analyze();
    const serious = axe.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
    expect(serious.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
  });
}

const ON_CUE = "[data-cue][data-on]";
const COUNTER = "[data-counter]";
const MARK = '[data-story="3d"] a[href="#notes"]';

/** Every line box of text inside the elements a selector finds, with the colour it is drawn in. */
async function textRuns(page: Page, selector: string) {
  return page.evaluate((selector) => {
    const runs: { text: string; color: string; x: number; y: number; w: number; h: number }[] = [];
    for (const el of document.querySelectorAll(selector)) {
      const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
      for (let n = walker.nextNode(); n; n = walker.nextNode()) {
        const text = n.textContent!.trim();
        if (!text) continue;
        const range = document.createRange();
        range.selectNodeContents(n);
        const color = getComputedStyle(n.parentElement!).color;
        for (const r of range.getClientRects()) {
          if (r.width < 2 || r.bottom < 0 || r.top > innerHeight) continue;
          runs.push({ text, color, x: r.left, y: r.top, w: r.width, h: r.height });
        }
      }
    }
    return runs;
  }, selector);
}

/**
 * For each line of the text a selector finds: the 2nd-percentile contrast between the text colour and
 * the rendered pixels behind that line, with the glyphs made transparent but their paper halo kept.
 */
async function lineContrasts(page: Page, selector: string) {
  const runs = await textRuns(page, selector);
  expect(runs.length, `${selector} has text on screen`).toBeGreaterThan(0);
  const hide = await page.addStyleTag({
    content: `[data-cue] *, ${COUNTER} *, ${MARK} * { color: transparent !important; -webkit-text-fill-color: transparent !important; text-decoration-color: transparent !important; }`,
  });
  await page.waitForTimeout(100);
  const shot = (await page.screenshot({ scale: "css" })).toString("base64");
  await hide.evaluate((tag) => (tag as Element).remove());
  return page.evaluate(
    async ({ shot, runs }) => {
      const img = await createImageBitmap(await (await fetch(`data:image/png;base64,${shot}`)).blob());
      const c = new OffscreenCanvas(img.width, img.height).getContext("2d")!;
      c.drawImage(img, 0, 0);
      const lin = (v: number) => ((v /= 255) <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
      const lum = (r: number, g: number, b: number) => 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
      return runs.map((run) => {
        const [r, g, b] = run.color.match(/\d+(\.\d+)?/g)!.map(Number);
        const fg = lum(r, g, b);
        const x = Math.max(0, Math.floor(run.x));
        const y = Math.max(0, Math.floor(run.y));
        const w = Math.min(img.width - x, Math.ceil(run.w));
        const h = Math.min(img.height - y, Math.ceil(run.h));
        const px = c.getImageData(x, y, w, h).data;
        const ratios: number[] = [];
        for (let i = 0; i < px.length; i += 4) {
          const bg = lum(px[i], px[i + 1], px[i + 2]);
          ratios.push((Math.max(fg, bg) + 0.05) / (Math.min(fg, bg) + 0.05));
        }
        ratios.sort((a, b) => a - b);
        return { text: run.text, ratio: ratios[Math.floor(ratios.length * 0.02)] };
      });
    },
    { shot, runs },
  );
}

const CONTRAST_WHY =
  "Each line box is screenshotted with its glyphs transparent (their paper halo and backdrop stay), and the text colour must reach 4.5:1 against the 2nd-percentile pixel behind it. " +
  "A median, or even the 10th percentile, passes a 2px thread arc drawn straight through a headline; the 2nd percentile fails it, and with the glyphs hidden there are no anti-aliased glyph edges to trip it.";

for (const vp of WIDTHS) {
  test(`landing at ${vp.w}px: every line of every cue and the honesty mark read at 4.5:1 against what is drawn behind them`, async ({ page }) => {
    test.setTimeout(180_000);
    await page.setViewportSize({ width: vp.w, height: vp.h });
    await openStory(page, 0);
    const low: string[] = [];
    const faint: string[] = [];
    for (const c of CUES) {
      await scrollStory(page, c.from + (c.to - c.from) * 0.55);
      await expect(onCues(page)).toHaveAttribute("data-cue", c.id);
      const held = await onCues(page).evaluate((el) => [el, ...el.querySelectorAll("[data-w]")].map((e) => Number(getComputedStyle(e).opacity)));
      if (held.some((o) => o < 1)) faint.push(c.id);
      for (const l of await lineContrasts(page, `${ON_CUE}, ${MARK}`)) if (l.ratio < 4.5) low.push(`${c.id} "${l.text}" ${l.ratio.toFixed(2)}:1`);
    }
    expect(faint, "at its hold a cue and every word in it are at full opacity").toEqual([]);
    expect(low, CONTRAST_WHY).toEqual([]);
  });

  test(`landing at ${vp.w}px: the Day counter reads at 4.5:1 all through the walk`, async ({ page }) => {
    test.setTimeout(120_000);
    await page.setViewportSize({ width: vp.w, height: vp.h });
    await openStory(page, 0);
    const counter = page.locator(COUNTER);
    const low: string[] = [];
    for (const p of [WALK.from, 0.165, 0.18, 0.2, 0.22, 0.24, 0.26, 0.28, 0.3, 0.315, 0.325, WALK.to, WALK.to + 0.005]) {
      await scrollStory(page, p);
      await expect(counter).toHaveCSS("opacity", "1");
      for (const l of await lineContrasts(page, COUNTER)) if (l.ratio < 4.5) low.push(`p=${p} "${l.text}" ${l.ratio.toFixed(2)}:1`);
    }
    expect(low, CONTRAST_WHY).toEqual([]);
  });
}

test("landing at 1440x900: wheel notches never skip a line, and every cue stays fully readable for 4 notches", async ({ page }) => {
  test.setTimeout(150_000);
  await page.setViewportSize({ width: 1440, height: 900 });
  await openStory(page);
  await page.mouse.move(1000, 500);
  const readable = new Map<string, number>();
  let run = { cue: "", notches: 0 };
  let end = false;
  for (let notch = 0; notch < 400 && !end; notch++) {
    const now = await page.evaluate(() => {
      const track = document.querySelector<HTMLElement>('[data-story="3d"]')!;
      const cue = document.querySelector<HTMLElement>("[data-cue][data-on]");
      const full = !!cue && [cue, ...cue.querySelectorAll("[data-w]")].every((el) => Number(getComputedStyle(el).opacity) === 1);
      return { id: full ? cue.dataset.cue! : "", done: track.getBoundingClientRect().bottom <= innerHeight + 1 };
    });
    run = now.id === run.cue ? { cue: run.cue, notches: run.notches + 1 } : { cue: now.id, notches: 1 };
    if (run.cue) readable.set(run.cue, Math.max(readable.get(run.cue) ?? 0, run.notches));
    end = now.done;
    if (!end) {
      await page.mouse.wheel(0, 100);
      await page.waitForTimeout(170);
    }
  }
  expect(end, "the wheel reaches the end of the story").toBe(true);
  const short = CUES.filter((c) => (readable.get(c.id) ?? 0) < 4).map((c) => `${c.id}: ${readable.get(c.id) ?? 0}`);
  expect(short, "fully readable notches for each cue").toEqual([]);
});

/** Waits for scroll and damping to end, then counts the frames drawn over `ms`. */
async function idleFrames(page: Page, ms: number) {
  await expect.poll(() => page.evaluate(() => window.__wasl3d!().clock), { timeout: 15_000 }).not.toBe("active");
  const a = await page.evaluate(() => window.__wasl3d!().frames);
  await page.waitForTimeout(ms);
  return (await page.evaluate(() => window.__wasl3d!().frames)) - a;
}

const STAGE_SMALL = { width: 640, height: 400 };
const WIDE_SHOT = 0.5;
const CLOSE_UP = 0.7707;
/** The fastest lane, in world units a second. */
const TOP_SPEED = 1.2;

test("landing: passers-by walk with no scrolling on a wide shot, and Fatima stands still", async ({ page }) => {
  test.setTimeout(60_000);
  await page.setViewportSize(STAGE_SMALL);
  await openStory(page);
  await scrollStory(page, WIDE_SHOT);
  await idleFrames(page, 0);
  const read = () => page.evaluate(() => ({ ...window.__wasl3d!(), at: performance.now() }));
  const before = await read();
  const after = await page.evaluate(
    (initial) =>
      new Promise<typeof before & { traveled: number[] }>((resolve) => {
        const started = performance.now();
        let previous = initial;
        const traveled = initial.map(() => 0);
        const tick = () => {
          const current = window.__wasl3d!();
          current.walkers.forEach(([x, , z], i) => {
            traveled[i] += Math.hypot(x - previous[i][0], z - previous[i][2]);
          });
          previous = current.walkers;
          if (performance.now() - started >= 1000) resolve({ ...current, at: performance.now(), traveled });
          else requestAnimationFrame(tick);
        };
        requestAnimationFrame(tick);
      }),
    before.walkers,
  );
  const seconds = (after.at - before.at) / 1000;
  expect(after.clock).toBe("ambient");
  expect(after.frames, "frames were drawn").toBeGreaterThan(before.frames);
  before.walkers.forEach((_, i) => {
    // A U-turn can end near where it began, so measure the walked path rather than net displacement.
    const moved = after.traveled[i];
    // A turn at a lane's end can eat some of the second, never all of it.
    expect(moved, `walker ${i} moved`).toBeGreaterThan(0.2);
    expect(moved, `walker ${i} walked, not ran`).toBeLessThan(TOP_SPEED * (seconds + 1));
  });
  expect(after.fatima, "Fatima waits for the scroll").toEqual(before.fatima);
  for (const [, y] of after.walkers) expect(y, "a small hop, feet near the ground").toBeLessThan(0.12);
});

test("landing: a scroll carries passers-by on at walking pace, never to a new place", async ({ page }) => {
  test.setTimeout(60_000);
  await page.setViewportSize(STAGE_SMALL);
  await openStory(page);
  await scrollStory(page, 0.45);
  await page.mouse.move(320, 200);
  await page.evaluate(() => {
    const w = window as unknown as { __walk: { t: number; at: number[][] }[]; __walking: boolean };
    w.__walk = [];
    w.__walking = true;
    const tick = (t: number) => {
      w.__walk.push({ t, at: window.__wasl3d!().walkers });
      if (w.__walking) requestAnimationFrame(tick);
    };
    requestAnimationFrame(tick);
  });
  const start = await page.evaluate(() => scrollY);
  const distance = await page.evaluate(() => {
    const el = document.querySelector<HTMLElement>('[data-story="3d"]')!;
    return el.offsetHeight - innerHeight;
  });
  for (let i = 0; i < 40; i++) {
    await page.mouse.wheel(0, (distance * (pageAt(0.53) - pageAt(0.45))) / 40);
    await page.waitForTimeout(30);
  }
  expect(await page.evaluate(() => scrollY), "the wheel scrolled").toBeGreaterThan(start + 100);
  const walk = await page.evaluate(() => {
    const w = window as unknown as { __walk: { t: number; at: number[][] }[]; __walking: boolean };
    w.__walking = false;
    return w.__walk;
  });
  expect(walk.length).toBeGreaterThan(10);
  for (let k = 1; k < walk.length; k++) {
    const dt = (walk[k].t - walk[k - 1].t) / 1000;
    walk[k].at.forEach(([x, , z], i) => {
      const step = Math.hypot(x - walk[k - 1].at[i][0], z - walk[k - 1].at[i][2]);
      // The sampler can read a frame late, so allow it one tenth of a second.
      expect(step, `walker ${i} at sample ${k}`).toBeLessThan(TOP_SPEED * (dt + 0.1) + 0.05);
    });
  }
});

test("landing: the stage draws at the ambient rate only when someone is in frame and on screen", async ({ page }) => {
  test.setTimeout(90_000);
  await page.setViewportSize(STAGE_SMALL);
  await openStory(page);
  await scrollStory(page, WIDE_SHOT);
  await idleFrames(page, 0);
  const clockAndFrames = () => page.evaluate(() => ({ at: performance.now(), frames: window.__wasl3d!().frames }));
  const start = await clockAndFrames();
  await page.waitForTimeout(2000);
  const end = await clockAndFrames();
  const rate = ((end.frames - start.frames) * 1000) / (end.at - start.at);
  expect(rate, "a wide shot keeps drawing").toBeGreaterThan(5);
  expect(rate, "no faster than the cap").toBeLessThan(33);

  await scrollStory(page, CLOSE_UP);
  await expect.poll(() => page.evaluate(() => window.__wasl3d!().clock), { message: "the camera settles on the close-up", timeout: 15_000 }).toBe("asleep");
  expect(await idleFrames(page, 2000), "a close-up with nobody in it").toBe(0);

  await scrollStory(page, WIDE_SHOT);
  await idleFrames(page, 0);
  await page.evaluate(() => {
    Object.defineProperty(document, "hidden", { value: true, configurable: true });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  expect(await idleFrames(page, 2000), "a hidden tab").toBe(0);
  await page.evaluate(() => {
    Object.defineProperty(document, "hidden", { value: false, configurable: true });
    document.dispatchEvent(new Event("visibilitychange"));
  });
  await expect.poll(() => page.evaluate(() => window.__wasl3d!().clock), "back in view").toBe("ambient");

  await page.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
  await page.waitForTimeout(300);
  expect(await idleFrames(page, 2000), "past the end of the track").toBe(0);
});

test("landing: a stopped clock stands passers-by on the ground and draws nothing when settled", async ({ page }) => {
  test.setTimeout(60_000);
  await page.setViewportSize(STAGE_SMALL);
  await openStory(page, 7);
  await scrollStory(page, WIDE_SHOT);
  expect(await idleFrames(page, 1500), "no frames at rest").toBe(0);
  const first = await page.evaluate(() => window.__wasl3d!().walkers);
  for (const [, y] of first) expect(y).toBe(0);
  await scrollStory(page, 0.46);
  expect(await page.evaluate(() => window.__wasl3d!().walkers), "scrolling moves nobody").toEqual(first);
  await page.goto("/?story=3d&still=7");
  await page.waitForFunction(() => typeof window.__wasl3d === "function");
  expect(await page.evaluate(() => window.__wasl3d!().walkers), "the same time is the same picture").toEqual(first);
});

test("landing: crossing the phone breakpoint rebuilds the layout and keeps the story where it is", async ({ page }) => {
  test.setTimeout(90_000);
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(String(e)));
  const layout = () =>
    page.evaluate(() => {
      const track = document.querySelector<HTMLElement>('[data-story="3d"]')!;
      const mark = document.querySelector('[data-story="3d"] a[href="#notes"]')!.getBoundingClientRect();
      const top = track.getBoundingClientRect().top + scrollY;
      return {
        trackVh: (track.offsetHeight / innerHeight) * 100,
        markRight: innerWidth - mark.right,
        markCentre: mark.left + mark.width / 2,
        page: (scrollY - top) / (track.offsetHeight - innerHeight),
        canvases: document.querySelectorAll('[data-story="3d"] canvas').length,
      };
    });
  await page.setViewportSize({ width: 700, height: 900 });
  await openStory(page);
  await scrollStory(page, 0.49);
  const narrow = await layout();
  expect(narrow.trackVh).toBeCloseTo(TRACK_VH.mobile, 0);
  expect(narrow.markCentre).toBeCloseTo(350, -1);

  await page.setViewportSize({ width: 1328, height: 781 });
  await page.waitForTimeout(900);
  const wide = await layout();
  expect(wide.trackVh, "the desktop track").toBeCloseTo(TRACK_VH.desktop, 0);
  expect(wide.markRight, "the mark sits at the right").toBeLessThan(40);
  expect(wide.canvases, "the old stage is gone").toBe(1);
  expect(Math.abs(wide.page - narrow.page), "same place in the story").toBeLessThan(0.002);
  await expect(onCues(page)).toHaveAttribute("data-cue", "never-hears");
  await expect.poll(() => page.evaluate(() => window.__wasl3d?.().frames ?? 0), "the new stage draws").toBeGreaterThan(0);

  await scrollStory(page, 1);
  const close = await page.locator('[data-cue="close"]').boundingBox();
  expect(close!.y, "the closing text sits above the city, not in the phone's bottom band").toBeLessThan(781 * 0.3);

  await scrollStory(page, 0.72);
  const before = await layout();
  await page.setViewportSize({ width: 700, height: 900 });
  await page.waitForTimeout(900);
  const back = await layout();
  expect(back.trackVh).toBeCloseTo(TRACK_VH.mobile, 0);
  expect(back.markCentre).toBeCloseTo(350, -1);
  expect(Math.abs(back.page - before.page), "same place in the story").toBeLessThan(0.002);
  await expect(onCues(page)).toHaveAttribute("data-cue", "join");
  expect(errors).toEqual([]);
});

test("landing: the Day counter runs to the published wait, with its tick at the standard", async ({ page }) => {
  await openStory(page);
  const counter = page.locator("[data-counter]");
  await expect(counter).toHaveAttribute("data-standard", String(standardDays));
  await expect(counter).toHaveAttribute("data-max", String(waitDays));
  await expect(counter).toContainText(`${standardDays} · standard`);
  await scrollStory(page, 0.05);
  await expect(counter).toHaveCSS("opacity", "0");
  await scrollStory(page, 0.24);
  const mid = Number(await counter.getAttribute("data-day"));
  expect(mid).toBeGreaterThan(0);
  expect(mid).toBeLessThan(waitDays);
  await scrollStory(page, 0.336);
  await expect(counter).toHaveAttribute("data-day", String(waitDays));
  await expect(counter.locator("[data-day-value]")).toHaveText(String(waitDays));
  await expect(counter).toContainText("published");
});

test("landing: the closures figure and the other numbers come from results.json", async ({ page }) => {
  await openStory(page);
  expect(closures.value).toBe(0);
  await scrollStory(page, 0.81);
  const cue = page.locator('[data-cue="none-wrong"]');
  await expect(cue).toHaveAttribute("data-on", "");
  await expect(cue).toContainText("None wrong");
  await expect(cue.locator('[data-figure="closures"]')).toContainText(`of ${closures.denominator.toLocaleString("en-US")} cases Wasl marked done.`);
  await expect(cue.locator('[data-honesty="simulated"]')).toHaveText("simulated");
  await expect(page.locator('[data-figure="wait"]')).toHaveText(`${waitDays} days`);
  await expect(page.locator('[data-cue="wait"] [data-honesty="published"]')).toHaveText("published study, 2021");
  await expect(page.locator('[data-figure="missed"]')).toHaveText("About 3 in 5");
  await expect(page.locator("main")).not.toContainText("2,090");
});

test("landing: the chips change the join, only in the join beat", async ({ page }) => {
  await openStory(page);
  const join = page.locator('[data-cue="join"]');
  await expect(join).toHaveCSS("visibility", "hidden");
  await scrollStory(page, 0.72);
  await expect(join).toHaveAttribute("data-on", "");
  const group = page.getByRole("group", { name: "Choose a person" });
  const verdict = page.getByTestId("verdict");
  await expect(group.getByRole("button", { name: "Fatima" })).toHaveAttribute("aria-pressed", "true");
  await expect(verdict).toHaveAttribute("data-outcome", "joined");
  await expect(verdict).toHaveText("Same birth date. Same person.");
  await expect(page.getByTestId("hospital-name")).toHaveText("F. Alzabi");

  await group.getByRole("button", { name: "Khalid" }).click();
  await expect(group.getByRole("button", { name: "Fatima" })).toHaveAttribute("aria-pressed", "false");
  await expect(verdict).toHaveAttribute("data-outcome", "apart");
  await expect(page.getByTestId("hospital-name")).toHaveText("Khaled Alkitbi");

  await group.getByRole("button", { name: "Aisha" }).click();
  await expect(verdict).toHaveAttribute("data-outcome", "review");
  await expect(verdict).toHaveText("Birth dates almost match. A person decides.");
  await page.waitForTimeout(1400);
  await page.screenshot({ path: "test-results/screens/story-join-aisha-1440.png" });

  await scrollStory(page, 0.8);
  await expect(join).toHaveCSS("visibility", "hidden");
});

test("landing: the hero and the close open the dashboard and the pipeline", async ({ page }) => {
  await openStory(page);
  for (const [id, p] of [
    ["hero", 0],
    ["close", 1],
  ] as const) {
    await scrollStory(page, p);
    const cue = page.locator(`[data-cue="${id}"]`);
    await expect(cue.getByRole("link", { name: "Open the dashboard" })).toHaveAttribute("href", "/app");
    await expect(cue.getByRole("link", { name: "How it works" })).toHaveAttribute("href", "/how-it-works");
  }
  const close = page.locator('[data-cue="close"]');
  await expect(close).toHaveAttribute("data-on", "");
  await expect(close.getByRole("link", { name: "Wasl, home" })).toBeVisible();
  await close.getByRole("link", { name: "Open the dashboard" }).click();
  await expect(page).toHaveURL(/\/app$/);
});

test("landing with reduced motion: stills with every line and the 2D name picker, no canvas", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/");
  await expect(page.locator('[data-story="stills"]')).toHaveCount(1);
  await expect(page.locator("canvas")).toHaveCount(0);
  for (const b of BEATS) await expect(page.locator(`[data-beat="${b.id}"] svg[role="img"]`)).toHaveCount(1);
  for (const c of CUES) await expect(page.locator(`[data-cue="${c.id}"]`)).toBeVisible();
  await expect(page.locator('[data-cue="hero"]').getByRole("link", { name: "How it works" })).toHaveAttribute("href", "/how-it-works");
  const group = page.getByRole("group", { name: "Choose a person" });
  await group.getByRole("button", { name: "Aisha" }).click();
  await expect(page.getByTestId("verdict")).toHaveText("A person decides");
  await group.getByRole("button", { name: "Khalid" }).click();
  await expect(page.getByTestId("verdict")).toHaveText("Two different people");
  await page.screenshot({ path: "test-results/screens/story-stills-1440.png", fullPage: true });
});

test("how it works: six moments, and the tracker's tick against Wasl's open case", async ({ page }) => {
  await page.goto("/how-it-works");
  const nav = page.getByRole("navigation", { name: "Primary" });
  await expect(nav.getByRole("link", { name: "How it works" })).toHaveAttribute("aria-current", "page");
  await expect(page.locator("#path-title + ol > li")).toHaveCount(6);
  await expect(page.locator('[data-side="tracker"]')).toContainText("Ticked done");
  await expect(page.locator('[data-side="wasl"]')).toContainText("Kept open");
  const faces = page.locator("#path-title + ol > li");
  expect(await faces.evaluateAll((lis) => lis.map((li) => li.getAttribute("data-face")))).toEqual(["fatima", "fatima", "record", "fatima", "fatima", "fatima"]);
  await expect(faces.nth(2).locator(".wav-root")).toHaveAttribute("data-state", "unknown");
  await expect(faces.nth(3).locator(".wav-root")).toHaveAttribute("data-state", "known");
  await expect(page.locator('#trap-title ~ div .wav-root[data-state="known"]')).toHaveCount(1);
  await expect(nav.getByRole("link", { name: /pilot/i })).toHaveCount(0);
  await expect(nav.getByRole("link", { name: "Open the dashboard" })).toHaveAttribute("href", "/app");
});

test("experts: KPI bars and outcome table use current sourced run figures", async ({ page }) => {
  await page.goto("/experts");
  const kpi = page.locator("figure");
  await expect(kpi).toContainText("Simulated, n = 1,331");
  for (const v of ["11.3%", "30.7%", "31.9%"]) await expect(kpi).toContainText(v);
  await expect(kpi).toContainText("65% of real completions look lost");
  await expect(kpi).toContainText("v7strict-cached-fhir-lung-dr-breast-ldct-ifhas-drnext-lungsurv-xreflink");

  const outcomes = page.getByTestId("outcomes");
  await expect(outcomes.locator("tbody tr")).toHaveCount(6);
  const ownership = outcomes.locator("tbody tr").nth(1);
  await expect(ownership).toContainText("simulated, 2,100 colorectal and cervical cases");
  await expect(ownership).toContainText("v7strict-cached-fhir-lung-dr-breast-ldct-ifhas-drnext-lungsurv-xreflink");
  await expect(ownership).toContainText("370 no acknowledgement, 241 not booked, 196 never notified");
  await expect(ownership).toContainText("referral unanswered 46 and referral rejected 10 are smaller");
  await expect(ownership.getByRole("link", { name: "the dashboard" })).toHaveAttribute("href", "/app");
  await expect(page.locator('[data-figure="false-closures"]')).toHaveText("0 / 3,025");
  await expect(page.locator('[data-figure="exactly-once"]')).toContainText("0 / 198,810");

  const tiers = page.getByRole("group", { name: "Closure tier" });
  const panel = page.getByTestId("tau-panel");
  const cell = (label: string) => panel.locator(`[data-cell="${label}"] [data-value]`);
  await expect(tiers.getByRole("button", { name: "τ 0.99", exact: true })).toHaveAttribute("aria-pressed", "true");
  await expect(panel).toContainText("τ 0.99. The operating tier");
  await expect(cell("Recall, with review")).toHaveText("0.816");
  await expect(cell("Review items per 100 loops")).toHaveText("58.8");

  await tiers.getByRole("button", { name: "τ 0.7", exact: true }).click();
  await expect(panel).toContainText("τ 0.7. Not certified");
  await expect(cell("False closures, with review")).toHaveText("8");
  await expect(cell("Recall, with review")).toHaveText("0.829");
  await expect(cell("Recall, no review")).toHaveText("0.565");

  await tiers.getByRole("button", { name: "τ 0.999", exact: true }).click();
  await expect(cell("Review items per 100 loops")).toHaveText("83.8");
  await expect(cell("False closures, with review")).toHaveText("0");
  await expect(tiers.locator('[aria-pressed="true"]')).toHaveCount(1);
});

test("figure lab: a figure wears its seed's look, and at known 0 it is the grey question mark", async ({ page }) => {
  await page.goto("/lab/figures");
  await expect(page.getByText("Lab · not part of the product")).toBeVisible();
  await page.waitForFunction(() => typeof window.__figureProbe === "function");
  const seen = await page.evaluate(() => ({
    known: window.__figureProbe!("p-fatima-zaabi", "female", 1),
    again: window.__figureProbe!("p-fatima-zaabi", "female", 1),
    other: window.__figureProbe!("p-khalid-kitbi", "male", 1),
    unknown: window.__figureProbe!("p-fatima-zaabi", "female", 0),
  }));
  expect(seen.known.body).toBe(seen.known.look.cloth);
  expect(seen.known.head).toBe(seen.known.look.skin);
  expect(seen.again).toEqual(seen.known);
  expect(seen.other.look).not.toEqual(seen.known.look);
  expect(seen.known).toMatchObject({ faceVisible: true, askVisible: false });
  expect(seen.known.hairVisible).toBe(seen.known.look.top !== "bald");
  expect(seen.unknown).toMatchObject({ hairVisible: false, faceVisible: false, askVisible: true });
});

test("experts: the clinical-labelling limit is current, and the obsolete claims are gone", async ({ page }) => {
  await page.goto("/experts");
  const limits = page.locator("#limits");
  await expect(limits).toContainText("clinically verified model-assisted labels");
  await expect(limits).toContainText("two model passes");
  await expect(limits).toContainText("clinician adjudication");
  await expect(limits).toContainText("κ = 1.000 on all five scored fields");
  await expect(limits).toContainText("not independent human double-labelling or clinical ground truth");
  await expect(limits).toContainText("small sample");
  await expect(limits).toContainText("Gate: 0 false closures, recall ≥ 0.95");
  const body = await page.locator("body").innerText();
  for (const gone of [
    "There are no clinician labels",
    "LLM-produced answer key",
    "Human double-labelling by both team members",
    "Before 21 Nov; not started",
  ]) {
    expect(body).not.toContain(gone);
  }
});
