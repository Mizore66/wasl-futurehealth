"use client";

import { useCallback, useEffect, useLayoutEffect, useState } from "react";
import { CircleHelp, X } from "lucide-react";

/** Proof of concept: a skippable walkthrough of the workspace, modelled on the welcome-card + anchored-popover tours on Mobbin (Quicken, beehiiv, Calendly). */

type Step = { target: string; title: string; body: string };

const STEPS: Step[] = [
  { target: "workspace", title: "Your daily work", body: "Today is your worklist. Patients, Referrals and Recovery hold every case you follow up." },
  { target: "oversight", title: "Oversight", body: "Programme shows how screening is going. Review desk is where cases wait for a second signature. Audit trail records every decision." },
  { target: "evidence", title: "The evidence", body: "Evidence, Pipeline and Results show how Wasl reached its numbers, so you can check them yourself." },
  { target: "breadcrumb", title: "Where you are", body: "The bar shows which screen you are on." },
  { target: "run", title: "Which data you are seeing", body: "Every number comes from one run. This notice says whether it is simulated and when it was taken. Open it for the details." },
  { target: "page", title: "This screen", body: "The page itself. Each panel's heading has a source note that says where its numbers come from." },
  { target: "help", title: "Come back anytime", body: "Lost? Click here to replay this tour." },
];

const SEEN_KEY = "wasl.tour.seen";
export const TOUR_EVENT = "wasl:tour";

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

/** The desktop sidebar and the phone menu both carry targets; take the one on screen. */
function findTarget(name: string): HTMLElement | null {
  const all = [...document.querySelectorAll<HTMLElement>(`[data-tour="${name}"]`)];
  return all.find((el) => el.getClientRects().length > 0 && el.offsetParent !== null) ?? null;
}

type Phase = "idle" | "welcome" | "touring";

export function GuidedTour() {
  const [phase, setPhase] = useState<Phase>("idle");
  const [steps, setSteps] = useState<Step[]>([]);
  const [index, setIndex] = useState(0);
  const [rect, setRect] = useState<DOMRect | null>(null);

  const start = useCallback(() => {
    const visible = STEPS.filter((s) => findTarget(s.target));
    setSteps(visible);
    setIndex(0);
    setPhase(visible.length ? "touring" : "idle");
  }, []);

  const close = useCallback(() => {
    markSeen();
    setPhase("idle");
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

  const step = phase === "touring" ? steps[index] : undefined;

  useLayoutEffect(() => {
    if (!step) return;
    const el = findTarget(step.target);
    if (!el) return;
    el.scrollIntoView({ block: "nearest", behavior: "smooth" });
    const measure = () => setRect(el.getBoundingClientRect());
    measure();
    window.addEventListener("resize", measure);
    window.addEventListener("scroll", measure, true);
    return () => {
      window.removeEventListener("resize", measure);
      window.removeEventListener("scroll", measure, true);
    };
  }, [step]);

  useEffect(() => {
    if (phase === "idle") return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
      if (phase !== "touring") return;
      if (e.key === "ArrowRight") setIndex((i) => Math.min(i + 1, steps.length - 1));
      if (e.key === "ArrowLeft") setIndex((i) => Math.max(i - 1, 0));
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [phase, steps.length, close]);

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
        <p className="m-0 mt-1.5 text-body text-ink-soft">Take a one-minute tour of where things are. You can replay it any time from the ? button.</p>
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

  if (!step || !rect) return null;

  const pad = 6;
  const last = index === steps.length - 1;
  const pop = placePopover(rect, pad);

  return (
    <div className="fixed inset-0 z-50" data-testid="guided-tour">
      {/* Clicks on the dimmed page are swallowed so the tour stays in charge; Skip or Esc leaves. */}
      <div className="absolute inset-0" onClick={(e) => e.stopPropagation()} />
      <div
        aria-hidden
        className="pointer-events-none absolute rounded-card ring-2 ring-jade-300 motion-safe:transition-all motion-safe:duration-300"
        style={{
          top: rect.top - pad,
          left: rect.left - pad,
          width: rect.width + pad * 2,
          height: rect.height + pad * 2,
          boxShadow: "0 0 0 9999px rgb(11 36 39 / 0.5)",
        }}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="tour-step-h"
        aria-describedby="tour-step-b"
        className="absolute w-[min(320px,calc(100vw-32px))] rounded-card border border-line bg-surface p-4 shadow-[0_12px_40px_rgb(11_36_39/0.25)] motion-safe:transition-all motion-safe:duration-300"
        style={pop}
      >
        <button type="button" onClick={close} aria-label="Skip tour" className="absolute right-2.5 top-2.5 rounded-control p-1 text-muted hover:bg-line/50 hover:text-ink">
          <X className="size-4" />
        </button>
        <h2 id="tour-step-h" className="m-0 pr-6 text-body font-semibold">
          {step.title}
        </h2>
        <p id="tour-step-b" className="m-0 mt-1 text-small text-ink-soft">
          {step.body}
        </p>
        <div className="mt-4 flex items-center justify-between gap-2">
          <span className="text-small tabular-nums text-muted" aria-live="polite">
            {index + 1} of {steps.length}
          </span>
          <div className="flex items-center gap-2">
            {index === 0 ? (
              <button type="button" onClick={close} className="h-8 rounded-control px-2.5 text-small text-muted hover:text-ink">
                Skip tour
              </button>
            ) : (
              <button type="button" onClick={() => setIndex(index - 1)} className="h-8 rounded-control border border-line px-2.5 text-small hover:bg-line/40">
                Back
              </button>
            )}
            <button
              type="button"
              autoFocus
              key={index}
              onClick={() => (last ? close() : setIndex(index + 1))}
              className="h-8 rounded-control bg-jade-700 px-3 text-small font-medium text-limestone-50 hover:bg-jade-800"
            >
              {last ? "Done" : "Next"}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

/** Right of the target when it fits (sidebar), else below, else above; clamped inside the viewport. */
function placePopover(r: DOMRect, pad: number): React.CSSProperties {
  const W = Math.min(320, window.innerWidth - 32);
  const H = 170;
  const gap = 14;
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const clampX = (x: number) => Math.max(16, Math.min(x, vw - W - 16));
  const clampY = (y: number) => Math.max(16, Math.min(y, vh - H - 16));

  if (r.right + pad + gap + W < vw - 16 && r.width < vw / 2) return { left: r.right + pad + gap, top: clampY(r.top) };
  if (r.bottom + pad + gap + H < vh) return { left: clampX(r.left), top: r.bottom + pad + gap };
  if (r.top - pad - gap - H > 0) return { left: clampX(r.left), top: r.top - pad - gap - H };
  return { left: clampX(r.left + r.width / 2 - W / 2), top: clampY(vh / 2 - H / 2) };
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
