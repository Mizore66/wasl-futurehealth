"use client";

import { useScroll, useTransform } from "motion/react";
import dynamic from "next/dynamic";
import { useCallback, useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore } from "react";
import { NameJoiner } from "@/components/site/name-joiner";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cn } from "@/lib/utils";
import { CueBody, type JoinStep } from "./copy";
import { BEATS, CUES, JOIN, TRACK_VH, WALK, beatAt, cueAt, dayAt, span, storyAt, type BeatId, type CastKey, type StoryFigures, type TextCue } from "./script";
import type { Anchors } from "./stage-canvas";
import { Still } from "./stills";

const StageCanvas = dynamic(() => import("./stage-canvas"), { ssr: false });

/**
 * `stills` is the server render and the fallback (reduced motion, no WebGL2, little memory, or a slow
 * first second); `3d` is the scroll story. `?story=3d|stills` forces one, for tests and probes, and
 * `&still=<seconds>` stops the passers-by at that time, for anything that compares pixels.
 */
type Mode = "stills" | "3d";
let detected: Mode | null = null;

/** What this device gets, decided once per page load on the client; the server always renders stills. */
function detectMode(): Mode {
  if (detected) return detected;
  const forced = new URLSearchParams(location.search).get("story");
  if (forced === "3d" || forced === "stills") return (detected = forced);
  const probe = document.createElement("canvas").getContext("webgl2");
  probe?.getExtension("WEBGL_lose_context")?.loseContext();
  const memory = (navigator as Navigator & { deviceMemory?: number }).deviceMemory;
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const room = !reduce && probe && !(memory !== undefined && memory <= 2);
  return (detected = room ? "3d" : "stills");
}
const never = () => () => {};

/** The phone layout follows the viewport for as long as the page is open. */
const PHONE = "(max-width: 767px)";
function watchPhone(onChange: () => void) {
  const query = matchMedia(PHONE);
  query.addEventListener("change", onChange);
  return () => query.removeEventListener("change", onChange);
}

const sectionId = (id: BeatId) => `beat-${id}`;

export function Story({ figures }: { figures: StoryFigures }) {
  const detectedMode = useSyncExternalStore<Mode>(never, detectMode, () => "stills");
  const mobile = useSyncExternalStore(watchPhone, () => matchMedia(PHONE).matches, () => false);
  const [slowAt, setSlowAt] = useState<BeatId | null>(null);
  const mode: Mode = slowAt ? "stills" : detectedMode;

  useEffect(() => {
    if (slowAt) document.getElementById(sectionId(slowAt))?.scrollIntoView();
  }, [slowAt]);

  const onSlow = useCallback((beat: BeatId) => {
    if (new URLSearchParams(location.search).get("story") !== "3d") setSlowAt(beat);
  }, []);

  if (mode === "3d") return <Journey figures={figures} mobile={mobile} onSlow={onSlow} />;
  return <Stills figures={figures} />;
}

const noPick = () => {};

/** One drawn frame per beat with that beat's lines beside it, in normal flow. */
function Stills({ figures }: { figures: StoryFigures }) {
  return (
    <div data-story="stills">
      {BEATS.map((b, i) => (
        <section key={b.id} id={sectionId(b.id)} data-beat={b.id} className={cn("scroll-mt-4 py-14 md:py-20", i % 2 ? "bg-(--sand)" : "bg-(--paper)")}>
          <div className="mx-auto grid w-full max-w-[1760px] grid-cols-[repeat(auto-fit,minmax(min(100%,420px),1fr))] items-center gap-[clamp(2rem,5vw,6rem)] px-[clamp(1rem,2.2vw,2rem)]">
            <div className="flex max-w-[40rem] flex-col gap-8">
              {CUES.filter((c) => beatAt(c.from) === b.id).map((c) => (
                <div key={c.id} data-cue={c.id}>
                  {c.id === "join" ? <NameJoiner /> : <CueBody id={c.id} f={figures} person="fatima" onPick={noPick} step="outcome" still />}
                </div>
              ))}
            </div>
            <Still id={b.id} figures={figures} />
          </div>
        </section>
      ))}
    </div>
  );
}

/** Desktop placement of each zone. Anchored cues are placed from the stage's projection. */
const ZONE: Record<string, string> = {
  tl: "left-[clamp(1rem,4vw,4.5rem)] top-[clamp(5rem,12vh,9rem)] max-w-[min(46rem,52vw)]",
  tc: "left-1/2 top-[clamp(4rem,9vh,7rem)] w-[min(60rem,86vw)] -translate-x-1/2 text-center",
  c: "left-1/2 top-1/2 w-[min(64rem,88vw)] -translate-x-1/2 -translate-y-1/2 text-center",
  bl: "bottom-[clamp(3.5rem,10vh,7rem)] left-[clamp(1rem,4vw,4.5rem)] max-w-[34rem]",
  br: "bottom-[clamp(3.5rem,10vh,7rem)] right-[clamp(1rem,4vw,4.5rem)] max-w-[30rem]",
  bc: "bottom-[clamp(2.5rem,6vh,4.5rem)] left-1/2 w-[min(40rem,92vw)] -translate-x-1/2",
  r: "right-[clamp(1rem,5vw,6rem)] top-1/2 w-[min(26rem,40vw)] -translate-y-1/2",
};

/** On a phone, every cue sits in one of two full-width bands. */
const BAND = {
  top: "inset-x-4 top-[5dvh]",
  bottom: "inset-x-4 bottom-[8dvh]",
};

/** What keeps text readable over any part of the scene: a soft pool of paper behind it, not a card. */
const PAPER_GLOW =
  "[text-shadow:0_0_18px_var(--paper),0_0_4px_var(--paper)] before:absolute before:-inset-x-10 before:-inset-y-6 before:-z-10 before:rounded-[3rem] before:bg-(--paper)/70 before:blur-2xl before:content-['']";
/** The same pool for chrome that has dark glass and towers behind it, where a cue would have open ground. */
const CHROME_GLOW =
  "[text-shadow:0_0_12px_var(--paper),0_0_3px_var(--paper)] before:absolute before:-inset-x-16 before:-inset-y-14 before:-z-10 before:rounded-[3rem] before:bg-(--paper)/90 before:blur-2xl before:content-['']";

/** Cues the visitor can act on; hidden ones leave the tab order. */
const ACTIONS = new Set(["hero", "join", "close"]);

function cueClass(c: TextCue, mobile: boolean): string {
  if (mobile) return BAND[c.band];
  if ("zone" in c.place) return ZONE[c.place.zone];
  return "max-w-[22rem]";
}

/** The scroll story: a sticky full-bleed stage, and over it one line of copy at a time. */
function Journey({ figures, mobile, onSlow }: { figures: StoryFigures; mobile: boolean; onSlow: (b: BeatId) => void }) {
  const track = useRef<HTMLDivElement>(null);
  const overlay = useRef<HTMLDivElement>(null);
  const day = useRef<HTMLDivElement>(null);
  const { scrollYProgress } = useScroll({ target: track, offset: ["start start", "end end"] });
  const story = useTransform(scrollYProgress, storyAt);
  const [person, setPerson] = useState<CastKey>("fatima");
  const [step, setStep] = useState<JoinStep>("name");
  const [anchors, setAnchors] = useState<Anchors>({});
  const slow = useCallback(() => onSlow(beatAt(story.get())), [onSlow, story]);

  // The track's height changes with the layout, so the scroll offset has to follow to stay on the same line.
  const layout = useRef(mobile);
  useLayoutEffect(() => {
    if (layout.current === mobile) return;
    layout.current = mobile;
    const el = track.current!;
    window.scrollTo({ top: el.getBoundingClientRect().top + window.scrollY + scrollYProgress.get() * (el.offsetHeight - window.innerHeight), behavior: "instant" });
  }, [mobile, scrollYProgress]);

  useEffect(() => {
    const root = overlay.current!;
    const els = CUES.map((c) => {
      const el = root.querySelector<HTMLElement>(`[data-cue="${c.id}"]`)!;
      return { c, el, words: [...el.querySelectorAll<HTMLElement>("[data-w]")], last: { reveal: -1, fade: -1 } };
    });
    const counter = day.current!;
    const dayText = counter.querySelector<HTMLElement>("[data-day-value]")!;
    const bar = counter.querySelector<HTMLElement>("[data-day-bar]")!;
    let lastDay = -1;
    let lastStep: JoinStep = "name";

    function apply(p: number) {
      for (const e of els) {
        const { reveal, fade } = cueAt(e.c, p);
        if (reveal === e.last.reveal && fade === e.last.fade) continue;
        e.last = { reveal, fade };
        const on = fade > 0;
        e.el.style.opacity = String(fade);
        e.el.style.visibility = on || !ACTIONS.has(e.c.id) ? "visible" : "hidden";
        e.el.style.pointerEvents = on ? "auto" : "none";
        e.el.style.setProperty("--rise", `${(1 - Math.min(1, reveal * 2.5)) * 16 + (1 - fade) * -8}px`);
        if (on) e.el.dataset.on = "";
        else delete e.el.dataset.on;
        const n = e.words.length;
        const k = reveal * (n + 1.5);
        e.words.forEach((w, i) => (w.style.opacity = String(0.12 + 0.88 * Math.min(1, Math.max(0, k - i)))));
      }
      const walking = p > WALK.from - 0.006 && p < WALK.to + 0.006;
      const d = dayAt(p, figures.waitDays);
      counter.style.opacity = walking ? "1" : "0";
      if (d !== lastDay) {
        lastDay = d;
        dayText.textContent = String(d);
        counter.dataset.day = String(d);
        bar.style.transform = `scaleX(${d / figures.waitDays})`;
      }
      const s: JoinStep = span(p, JOIN.from, JOIN.to) < 0.45 ? "name" : "outcome";
      if (s !== lastStep) setStep((lastStep = s));
    }
    apply(story.get());
    return story.on("change", apply);
  }, [story, figures.waitDays]);

  return (
    <div ref={track} data-story="3d" className="relative" style={{ height: `${mobile ? TRACK_VH.mobile : TRACK_VH.desktop}dvh` }}>
      <div className="sticky top-0 h-dvh overflow-hidden" data-dots={figures.closuresDone}>
        <StageCanvas progress={story} person={person} figures={figures} mobile={mobile} onSlow={slow} onAnchors={setAnchors} />
        <div ref={overlay} className="story-copy pointer-events-none absolute inset-0">
          {CUES.map((c) => {
            const a = "anchor" in c.place && !mobile ? anchors[c.place.anchor] : undefined;
            const flip = a && a.x > 0.62;
            return (
              <div
                key={c.id}
                data-cue={c.id}
                className={cn("absolute [transform:translateY(var(--rise,0px))]", PAPER_GLOW, cueClass(c, mobile), "anchor" in c.place && !mobile && !a && "invisible")}
                style={
                  a
                    ? {
                        left: `${(a.x * 100).toFixed(2)}%`,
                        top: `${(a.y * 100).toFixed(2)}%`,
                        translate: flip ? "calc(-100% - 1.25rem) -50%" : "1.25rem -50%",
                        textAlign: flip ? "right" : "left",
                      }
                    : undefined
                }
              >
                <CueBody id={c.id} f={figures} person={person} onPick={setPerson} step={step} />
              </div>
            );
          })}
        </div>
        <DayCounter ref={day} figures={figures} mobile={mobile} />
        <a
          href="#notes"
          className={cn(
            "absolute bottom-3 z-10 text-[12px] leading-none text-(--ink) no-underline underline-offset-2 hover:underline [text-shadow:0_0_6px_var(--paper),0_0_2px_var(--paper)] before:absolute before:-inset-x-5 before:-inset-y-3 before:-z-10 before:rounded-full before:bg-(--paper)/90 before:blur-md before:content-['']",
            mobile ? "left-1/2 -translate-x-1/2" : "right-[clamp(1rem,2vw,2rem)]",
          )}
        >
          <HonestyLabel kind="simulated" className="h-auto bg-transparent px-0 text-[12px] font-normal text-inherit [&>[data-dot]]:hidden">
            Synthetic people · simulated results
          </HonestyLabel>
        </a>
      </div>
    </div>
  );
}

/**
 * Chrome, not story text: the published median wait counts up while Fatima walks, against a tick at
 * the standard. Both numbers come from results.json.
 */
function DayCounter({ ref, figures, mobile }: { ref: React.Ref<HTMLDivElement>; figures: StoryFigures; mobile: boolean }) {
  return (
    <div
      ref={ref}
      data-counter
      data-max={figures.waitDays}
      data-standard={figures.standardDays}
      className={cn("absolute z-10 w-[min(15rem,42vw)] text-(--ink) opacity-0", CHROME_GLOW, mobile ? "right-4 top-[3dvh] w-[11rem]" : "right-[clamp(1rem,4vw,4.5rem)] top-[clamp(1.5rem,5vh,3rem)]")}
    >
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-[14px] font-semibold uppercase tracking-[0.08em]">Day</span>
        <span data-day-value className="text-[clamp(1.75rem,1rem_+_1.6vw,2.75rem)] font-bold leading-none tabular-nums tracking-[-0.03em]">
          0
        </span>
      </div>
      <div className="relative mt-2 h-[3px] bg-(--sand-deep)">
        <div data-day-bar className="absolute inset-0 origin-left scale-x-0 bg-(--teal)" />
        <div className="absolute -top-[5px] h-[13px] w-[2px] bg-(--ink)" style={{ left: `${(figures.standardDays / figures.waitDays) * 100}%` }} />
      </div>
      <div className="relative mt-1.5 h-[18px] text-[12px] leading-none">
        <span className="absolute -translate-x-1/2 whitespace-nowrap" style={{ left: `${(figures.standardDays / figures.waitDays) * 100}%` }}>
          {figures.standardDays} · standard
        </span>
        <span className="absolute right-0">{figures.waitDays}</span>
      </div>
      <div className="mt-1 text-[12px] leading-[1.3]">
        Typical wait in Abu Dhabi, <span className="italic">published</span>
      </div>
    </div>
  );
}
