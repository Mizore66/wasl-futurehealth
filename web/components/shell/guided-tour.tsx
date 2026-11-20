"use client";

import { useCallback, useEffect, useLayoutEffect, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { CircleHelp, MousePointerClick, X } from "lucide-react";
import { TOUR_STEPS, type TourStep } from "./tour-steps";

/**
 * A skippable walkthrough that moves through the workspace page by page. "Click" steps leave only the highlighted
 * link live; the tour moves on when the user lands on that page. Modelled on the welcome-card + anchored-popover
 * tours on Mobbin (Quicken, beehiiv, Calendly, Front).
 */

const SEEN_KEY = "wasl.tour.seen";
export const TOUR_EVENT = "wasl:tour";
const FIND_TIMEOUT_MS = 6000;
const SKIP_AFTER_MS = 600;

/** Automated browsers (Playwright sets navigator.webdriver) never get the unprompted welcome card; the ? button still works. */
function readSeen(): boolean {
  if (navigator.webdriver) return true;
  try {
    return localStorage.getItem(SEEN_KEY) === "1";
  } catch {
    return true;
  }
}

function markSeen() {
  try {
    localStorage.setItem(SEEN_KEY, "1");
  } catch {}
}

/** The desktop sidebar and the phone menu can both match; take the one on screen. */
function findTarget(selector: string): HTMLElement | null {
  const all = [...document.querySelectorAll<HTMLElement>(selector)];
  return all.find((el) => el.getClientRects().length > 0) ?? null;
}

const onPage = (pathname: string, page?: string) => !page || pathname === page;

type Phase = "idle" | "welcome" | "touring";

export function GuidedTour() {
  const pathname = usePathname();
  const router = useRouter();
  const [phase, setPhase] = useState<Phase>("idle");
  const [index, setIndex] = useState(0);
  const [found, setFound] = useState<{ index: number; el: HTMLElement } | null>(null);
  const [rect, setRect] = useState<DOMRect | null>(null);

  const step: TourStep | undefined = phase === "touring" ? TOUR_STEPS[index] : undefined;
  const last = index === TOUR_STEPS.length - 1;

  const close = useCallback(() => {
    markSeen();
    setPhase("idle");
    setFound(null);
  }, []);

  const go = useCallback(
    (to: number) => {
      if (to >= TOUR_STEPS.length) return close();
      setFound(null);
      setIndex(Math.max(0, to));
    },
    [close],
  );

  /** Back steps over a click step: the user is already past it, and landing on its page would advance it again. */
  const back = useCallback(() => {
    const to = TOUR_STEPS[index - 1]?.goTo ? index - 2 : index - 1;
    if (to >= 0) go(to);
  }, [index, go]);

  const start = useCallback(() => {
    setFound(null);
    setIndex(0);
    setPhase("touring");
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      if (!readSeen()) setPhase("welcome");
    }, 600);
    const onReplay = () => start();
    window.addEventListener(TOUR_EVENT, onReplay);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener(TOUR_EVENT, onReplay);
    };
  }, [start]);

  // A step that lives on another page (after Back, or a replay started elsewhere) takes the user there.
  useEffect(() => {
    if (step && !onPage(pathname, step.page)) router.push(step.page!);
  }, [step, pathname, router]);

  // A click step is done once the user is where it pointed (adjusted during render, not in an effect).
  if (step?.goTo && pathname === step.goTo) {
    setFound(null);
    setIndex(index + 1);
  }

  // A page's first step waits for it to load; later steps on a loaded page skip a missing target (a phone layout, a case
  // with no open question) almost at once.
  useEffect(() => {
    if (!step || !onPage(pathname, step.page) || (step.goTo && pathname === step.goTo)) return;
    const loaded = index > 0 && TOUR_STEPS[index - 1].page === step.page;
    const began = Date.now();
    const timer = window.setInterval(() => {
      const el = findTarget(step.target);
      if (el) {
        window.clearInterval(timer);
        const r = el.getBoundingClientRect();
        if (r.top < 60 || r.bottom > window.innerHeight) el.scrollIntoView({ block: "center", behavior: "smooth" });
        setFound({ index, el });
      } else if (Date.now() - began > (loaded ? SKIP_AFTER_MS : FIND_TIMEOUT_MS)) {
        window.clearInterval(timer);
        if (step.goTo) router.push(step.goTo);
        else go(index + 1);
      }
    }, 100);
    return () => window.clearInterval(timer);
  }, [step, pathname, index, go, router]);

  const el = found?.index === index ? found.el : null;

  useLayoutEffect(() => {
    if (!el) return;
    const measure = () => setRect(el.getBoundingClientRect());
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    window.addEventListener("resize", measure);
    window.addEventListener("scroll", measure, true);
    return () => {
      ro.disconnect();
      window.removeEventListener("resize", measure);
      window.removeEventListener("scroll", measure, true);
    };
  }, [el]);

  useEffect(() => {
    if (phase === "idle") return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
      if (phase !== "touring" || !step) return;
      if (e.key === "ArrowRight" && !step.goTo) go(index + 1);
      if (e.key === "ArrowLeft") back();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [phase, step, index, close, go, back]);

  if (phase === "welcome") {
    return (
      <div
        role="dialog"
        aria-labelledby="tour-welcome-h"
        className="fixed bottom-5 right-5 z-50 w-[min(340px,calc(100vw-32px))] rounded-card border border-line bg-surface p-5 shadow-[0_12px_40px_rgb(11_36_39/0.18)] motion-safe:animate-in motion-safe:fade-in motion-safe:slide-in-from-bottom-2"
      >
        <button type="button" onClick={close} aria-label="Close" className="absolute right-3 top-3 rounded-control p-1 text-muted hover:bg-line/50 hover:text-ink">
          <X className="size-4" />
        </button>
        <h2 id="tour-welcome-h" className="m-0 text-section font-medium">
          Welcome to Wasl
        </h2>
        <p className="m-0 mt-1.5 text-body text-ink-soft">
          A two-minute walk through your worklist, patients, the review desk and the programme numbers. Replay it any time from the ? button.
        </p>
        <div className="mt-4 flex justify-end gap-2">
          <button type="button" onClick={close} className="h-9 rounded-control border border-line px-3 text-body hover:bg-line/40">
            Not now
          </button>
          <button type="button" onClick={start} autoFocus className="h-9 rounded-control bg-jade-700 px-3 text-body font-medium text-limestone-50 hover:bg-jade-800">
            Show me around
          </button>
        </div>
      </div>
    );
  }

  if (!step || !el || !rect) return null;

  const pad = 6;
  const hole = { top: rect.top - pad, left: rect.left - pad, right: rect.right + pad, bottom: rect.bottom + pad };
  const pop = placePopover(rect, pad);
  const click = Boolean(step.goTo);
  const section = TOUR_STEPS.filter((s) => s.section === step.section);
  const inSection = section.indexOf(step) + 1;

  return (
    <div className="pointer-events-none fixed inset-0 z-50" data-testid="guided-tour" data-step={index} data-goto={step.goTo}>
      {/* Four blockers around the spotlight keep the page inert; a click step leaves its hole open so the highlighted link works. */}
      {[
        { top: 0, left: 0, right: 0, height: Math.max(0, hole.top) },
        { top: hole.bottom, left: 0, right: 0, bottom: 0 },
        { top: hole.top, left: 0, width: Math.max(0, hole.left), height: hole.bottom - hole.top },
        { top: hole.top, left: hole.right, right: 0, height: hole.bottom - hole.top },
        ...(click ? [] : [{ top: hole.top, left: hole.left, width: hole.right - hole.left, height: hole.bottom - hole.top }]),
      ].map((style, i) => (
        <div key={i} className="pointer-events-auto absolute" style={style} />
      ))}
      <div
        aria-hidden
        className={`absolute rounded-card ring-2 motion-safe:transition-all motion-safe:duration-300 ${click ? "ring-ochre-ink motion-safe:animate-pulse" : "ring-jade-300"}`}
        style={{
          top: hole.top,
          left: hole.left,
          width: hole.right - hole.left,
          height: hole.bottom - hole.top,
          boxShadow: "0 0 0 9999px rgb(11 36 39 / 0.5)",
        }}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="tour-step-h"
        aria-describedby="tour-step-b"
        className="pointer-events-auto absolute w-[min(340px,calc(100vw-32px))] rounded-card border border-line bg-surface p-4 shadow-[0_12px_40px_rgb(11_36_39/0.25)] motion-safe:transition-all motion-safe:duration-300"
        style={pop}
      >
        <button type="button" onClick={close} aria-label="Skip tour" className="absolute right-2.5 top-2.5 rounded-control p-1 text-muted hover:bg-line/50 hover:text-ink">
          <X className="size-4" />
        </button>
        <p className="m-0 text-[11px] font-semibold uppercase tracking-[1.2px] text-jade-700">
          {step.section}
          {section.length > 1 && <span className="font-normal text-muted"> · {inSection} of {section.length}</span>}
        </p>
        <h2 id="tour-step-h" className="m-0 mt-1 pr-6 text-body font-semibold">
          {step.title}
        </h2>
        <p id="tour-step-b" className="m-0 mt-1 text-small text-ink-soft">
          {step.body}
        </p>
        {click && (
          <p className="m-0 mt-3 flex items-center gap-1.5 text-small font-medium text-ink">
            <MousePointerClick aria-hidden className="size-4 text-ochre-ink" strokeWidth={1.8} />
            Click the highlighted link to continue
          </p>
        )}
        <div className="mt-4 flex items-center justify-between gap-2">
          <Progress index={index} />
          <div className="flex items-center gap-2">
            {index === 0 ? (
              <button type="button" onClick={close} className="h-8 rounded-control px-2.5 text-small text-muted hover:text-ink">
                Skip tour
              </button>
            ) : (
              <button type="button" onClick={back} className="h-8 rounded-control border border-line px-2.5 text-small hover:bg-line/40">
                Back
              </button>
            )}
            {!click && (
              <button
                type="button"
                autoFocus
                key={index}
                onClick={() => go(index + 1)}
                className="h-8 rounded-control bg-jade-700 px-3 text-small font-medium text-limestone-50 hover:bg-jade-800"
              >
                {last ? "Done" : "Next"}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

/** One segment per section, filled up to the current one. */
function Progress({ index }: { index: number }) {
  const sections = [...new Set(TOUR_STEPS.map((s) => s.section))];
  const current = sections.indexOf(TOUR_STEPS[index].section);
  return (
    <div className="flex items-center gap-1" role="img" aria-label={`Step ${index + 1} of ${TOUR_STEPS.length}`}>
      {sections.map((s, i) => (
        <span key={s} className={`h-1.5 w-4 rounded-full ${i <= current ? "bg-jade-700" : "bg-line"}`} />
      ))}
    </div>
  );
}

/** Right of the target when it fits (sidebar), else below, else above; clamped inside the viewport. */
function placePopover(r: DOMRect, pad: number): React.CSSProperties {
  const W = Math.min(340, window.innerWidth - 32);
  const H = 210;
  const gap = 14;
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const clampX = (x: number) => Math.max(16, Math.min(x, vw - W - 16));
  const clampY = (y: number) => Math.max(16, Math.min(y, vh - H - 16));

  if (r.right + pad + gap + W < vw - 16 && r.width < vw / 2) return { left: r.right + pad + gap, top: clampY(r.top) };
  if (r.bottom + pad + gap + H < vh) return { left: clampX(r.left), top: r.bottom + pad + gap };
  if (r.top - pad - gap - H > 0) return { left: clampX(r.left), top: r.top - pad - gap - H };
  if (r.left - pad - gap - W > 16) return { left: r.left - pad - gap - W, top: clampY(r.top) };
  return { left: clampX(r.right - W), top: clampY(vh - H - 16) };
}

/** The replay control in the top bar. */
export function TourButton() {
  return (
    <button
      type="button"
      data-tour="help"
      onClick={() => window.dispatchEvent(new Event(TOUR_EVENT))}
      aria-label="Show the guided tour"
      title="Show the guided tour"
      className="grid size-8 flex-none place-items-center rounded-control text-muted hover:bg-line/50 hover:text-ink"
    >
      <CircleHelp className="size-[18px]" strokeWidth={1.6} />
    </button>
  );
}
