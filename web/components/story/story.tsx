"use client";

import { useScroll } from "motion/react";
import dynamic from "next/dynamic";
import Link from "next/link";
import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { ContactCta } from "@/components/site/contact";
import { NameJoiner, PEOPLE } from "@/components/site/name-joiner";
import { cta, display, figure, h2, lead, link, small } from "@/components/site/styles";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cn } from "@/lib/utils";
import { BEATS, CAST, CAST_ORDER, TRACK_VH, beatAt, type BeatId, type CastKey, type StoryFigures } from "./beats";
import { Still } from "./stills";

const StageCanvas = dynamic(() => import("./stage-canvas"), { ssr: false });


/**
 * `stills` is the server render and the fallback (reduced motion, no WebGL2, little memory, or a slow
 * first second); `3d` is the scroll room. `?story=3d|stills` forces one, for tests and probes.
 */
type Mode = "stills" | "3d";

const word = "text-[14px] leading-[1.45] text-(--muted)";

function Published({ children }: { children: React.ReactNode }) {
  return (
    <HonestyLabel kind="published" className={cn("inline h-auto whitespace-normal bg-transparent px-0", word)}>
      {children}
    </HonestyLabel>
  );
}

function Simulated({ children }: { children: React.ReactNode }) {
  return (
    <HonestyLabel kind="simulated" className={cn("inline h-auto whitespace-normal bg-transparent px-0", word)}>
      {children}
    </HonestyLabel>
  );
}

const RECEIPT = [
  { label: "Follow-up check", value: "Complete" },
  { label: "Proof, in the hospital’s words", value: "“All surfaces were well seen”" },
  { label: "Read by", value: "Two readers, and both agree" },
  { label: "Matched by", value: "Her name and her birth date" },
  { label: "Looked after by", value: "Her clinic nurse" },
];

function Receipt() {
  return (
    <div
      role="img"
      aria-label="A Wasl receipt for Fatima Al Zaabi: follow-up check complete, with the hospital's own words as proof, read by two readers who agree, matched by name and birth date, looked after by her clinic nurse, done with proof."
      className="mt-8 max-w-[26rem] bg-(--card) px-6 pb-6 pt-7 shadow-[0_1px_0_var(--sand-deep)]"
    >
      <div className="flex items-center gap-3">
        <span className="text-[20px] font-bold leading-none tracking-[-0.02em]">Wasl</span>
        <span className="h-[18px] w-[3px] bg-(--teal)" />
        <span lang="ar" dir="rtl" className="font-arabic text-[18px] font-semibold leading-none text-(--teal)">
          وصل
        </span>
      </div>
      <div className="mt-5 text-[28px] font-bold leading-[1.05] tracking-[-0.02em]">Fatima Al Zaabi</div>
      <dl className="mt-4 flex flex-col gap-2.5">
        {RECEIPT.map((r) => (
          <div key={r.label}>
            <dt className={small}>{r.label}</dt>
            <dd className="m-0 text-[16px] font-semibold leading-[1.4]">{r.value}</dd>
          </div>
        ))}
      </dl>
      <div className="mt-5 bg-(--teal) px-4 py-3 text-[18px] font-bold text-(--paper)">Done, with proof</div>
    </div>
  );
}

const WHO = [
  { who: "The person waiting", what: "A message in her own language, and a named nurse to call." },
  { who: "The nurse", what: "One list of who still needs a check, so calls go to the right people." },
  { who: "The health system", what: "A follow-up rate it can trust, because every case marked done carries its proof." },
  { who: "The auditor", what: "Every decision written down, with who made it." },
];

/** The live join for the 3D room: the chips move the thread; the verdict is plain text. */
function Chips({ person, onPick }: { person: CastKey; onPick: (k: CastKey) => void }) {
  const p = PEOPLE[person];
  return (
    <div className="mt-7">
      <div role="group" aria-label="Choose a person" className="flex flex-wrap gap-2">
        {CAST_ORDER.map((k) => (
          <button
            key={k}
            type="button"
            aria-pressed={k === person}
            onClick={() => onPick(k)}
            className={cn(
              "min-h-12 cursor-pointer border-0 px-5 py-2.5 text-[18px] font-semibold",
              k === person ? "bg-(--ink) text-(--paper)" : "bg-(--card) text-(--ink) hover:bg-(--sand)",
            )}
          >
            {CAST[k].first}
          </button>
        ))}
      </div>
      <dl className="mt-5 grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-[16px] leading-[1.45]">
        <dt className={small}>Clinic</dt>
        <dd className="m-0">
          <span lang="ar" dir="rtl" className="font-arabic font-semibold" data-testid="clinic-name">
            {p.arabic}
          </span>
          . {p.clinicDetail}
        </dd>
        <dt className={small}>Hospital</dt>
        <dd className="m-0">
          <span className="font-semibold" data-testid="hospital-name">
            {p.latin}
          </span>
          . {p.hospitalDetail}
        </dd>
      </dl>
      <p aria-live="polite" className="m-0 mt-4 text-[22px] font-bold leading-[1.3]" data-testid="verdict" data-joined={p.joined}>
        {p.joined ? "Same person" : "Not joined. A person decides."}
      </p>
      <HonestyLabel kind="illustrative" className={cn("mt-2 block h-auto bg-transparent px-0", word)}>
        Made-up people, for illustration.
      </HonestyLabel>
    </div>
  );
}

/** Over the 3D room the panels must fit one screen, so display type steps down a size. */
const roomDisplay = "m-0 text-[clamp(2.5rem,1rem_+_3.6vw,4.75rem)] leading-[0.98] tracking-[-0.04em] font-bold text-balance";
const roomFigure = "text-[clamp(2.5rem,1rem_+_3vw,4.25rem)] leading-none tracking-[-0.04em] font-bold";

function Copy({ id, f, mode, person, onPick }: { id: BeatId; f: StoryFigures; mode: Mode; person: CastKey; onPick: (k: CastKey) => void }) {
  const room = mode === "3d";
  const big = room ? roomDisplay : display;
  const fig = room ? roomFigure : figure;
  switch (id) {
    case "lost":
      return (
        <>
          <h1 className={cn(big, "-ml-[0.04em]")}>
            Nobody lost
            <br /> between hospitals
          </h1>
          <p className={cn(lead, "mt-8 max-w-[34rem]")}>
            When a screening test asks for a closer look, Wasl makes sure that person gets the follow-up check, and marks it done only with
            proof.
          </p>
          <div className="mt-8 flex flex-wrap items-center gap-7">
            <a href="#pilot" className={cta}>
              Request a pilot
            </a>
            <Link href="/how-it-works" className={link}>
              See how it works
            </Link>
          </div>
        </>
      );
    case "unknown":
      return (
        <>
          <h2 className={h2}>Done, and nobody knew</h2>
          <p className={cn(lead, "mt-6")}>
            Fatima had her check across the city. The hospital wrote her name as “F.&nbsp;Alzabi” and left off her ID number. To her clinic,
            she never went.
          </p>
          <p className={cn(lead, "mt-4")}>So the nurse keeps calling her, while someone who truly missed their check waits further down the list.</p>
          <div className={cn(fig, "mt-6 text-(--teal)")} data-figure="missed">
            About {f.missedOfFive} in 5
          </div>
          <p className="m-0 mt-3 text-[18px] leading-[1.5]">
            people who had their check looked as if they had not, when records were matched by ID number alone.{" "}
            <Simulated>A rounded figure, from testing on simulated patients.</Simulated>
          </p>
        </>
      );
    case "wait":
      return (
        <>
          <div className={fig} data-figure="wait">
            {f.waitDays} days
          </div>
          <p className={cn(lead, "mt-5")}>
            The typical wait for the follow-up check in Abu Dhabi, against a {f.standardDays}-day standard.
          </p>
          <p className="m-0 mt-3">
            <Published>From a published study, 2021.</Published>
          </p>
        </>
      );
    case "join":
      return (
        <>
          <h2 className={h2}>One person, two spellings</h2>
          <p className={cn(lead, "mt-6")}>
            A similar name is never enough. Wasl also checks a second detail, like a birth date. If they disagree, a person decides.
          </p>
          {mode === "3d" ? <Chips person={person} onPick={onPick} /> : <div className="mt-8"><NameJoiner /></div>}
        </>
      );
    case "proof":
      return (
        <>
          <h2 className={h2}>Done only when it is proven</h2>
          <p className={cn(lead, "mt-6")}>
            Two separate readers, one following fixed medical rules and one AI, read the hospital’s report. Both must agree. If they do not, two
            people look before anything is closed.
          </p>
          <blockquote className="m-0 mt-6 border-l-[3px] border-(--teal) pl-4 text-[20px] font-semibold leading-[1.4]">
            “all surfaces were well seen”
          </blockquote>
          <div className={cn(fig, "mt-6 text-(--teal)")}>None wrong</div>
          <p className="m-0 mt-3 text-[18px] leading-[1.5]" data-figure="closures">
            of {f.closuresDone.toLocaleString("en-US")} cases Wasl marked done. <Simulated>In testing on simulated patients.</Simulated>
          </p>
        </>
      );
    case "receipt":
      return (
        <>
          <p className={cn(lead, "max-w-[30rem]")}>
            In Arabic,{" "}
            <span lang="ar" dir="rtl" className="font-arabic font-semibold text-(--teal)">
              وصل
            </span>{" "}
            means a connection. It is also a word for a receipt.
          </p>
          <Receipt />
        </>
      );
    case "who":
      return (
        <>
          <h2 className={h2}>Who notices the difference</h2>
          <ul className="m-0 mt-6 flex list-none flex-col gap-4 p-0">
            {WHO.map((w) => (
              <li key={w.who}>
                <span className="text-[20px] font-bold">{w.who}.</span> <span className="text-[18px] leading-[1.5]">{w.what}</span>
              </li>
            ))}
          </ul>
        </>
      );
    case "start":
      return (
        <>
          <h2 id="start-title" className={cn(big, "-ml-[0.04em]")}>
            Try it at one clinic first
          </h2>
          <p className={cn(lead, "mt-6 max-w-[34rem]")}>
            Three months, free, alongside your team. A clinician checks every case before Wasl may mark it done.
          </p>
          <div className="mt-8 flex flex-wrap items-center gap-7">
            <ContactCta />
            <Link href="/pilot" className={cn(link, "text-(--ink)")}>
              See how a pilot works
            </Link>
          </div>
        </>
      );
  }
}

/** The last beat doubles as the page's #pilot anchor, which the header's request link targets. */
const sectionId = (id: BeatId) => (id === "start" ? "pilot" : `beat-${id}`);

const panel = "bg-(--paper)/94 p-[clamp(1.25rem,2.4vw,2.25rem)]";

type Room = { mode: Mode; mobile: boolean };
let detected: Room | null = null;

/** What this device gets, decided once per page load on the client; the server always renders stills. */
function detect(): Room {
  if (detected) return detected;
  const forced = new URLSearchParams(location.search).get("story");
  const mobile = matchMedia("(max-width: 767px)").matches;
  if (forced === "3d" || forced === "stills") return (detected = { mode: forced, mobile });
  const probe = document.createElement("canvas").getContext("webgl2");
  probe?.getExtension("WEBGL_lose_context")?.loseContext();
  const memory = (navigator as Navigator & { deviceMemory?: number }).deviceMemory;
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const room = !reduce && probe && !(memory !== undefined && memory <= 2);
  return (detected = { mode: room ? "3d" : "stills", mobile });
}
const SERVER: Room = { mode: "stills", mobile: false };
const never = () => () => {};

export function Story({ figures }: { figures: StoryFigures }) {
  const device = useSyncExternalStore(never, detect, () => SERVER);
  const [slowAt, setSlowAt] = useState<BeatId | null>(null);
  const [person, setPerson] = useState<CastKey>("fatima");
  const mode: Mode = slowAt ? "stills" : device.mode;
  const mobile = device.mobile;

  useEffect(() => {
    if (slowAt) document.getElementById(sectionId(slowAt))?.scrollIntoView();
  }, [slowAt]);

  const onSlow = useCallback((beat: BeatId) => {
    if (new URLSearchParams(location.search).get("story") !== "3d") setSlowAt(beat);
  }, []);

  if (mode === "3d") return <Room figures={figures} mobile={mobile} person={person} onPick={setPerson} onSlow={onSlow} />;

  return (
    <div data-story="stills">
      {BEATS.map((b, i) => (
        <section
          key={b.id}
          id={sectionId(b.id)}
          data-beat={b.id}
          className={cn("scroll-mt-4 py-14 md:py-20", i % 2 ? "bg-(--sand)" : "bg-(--paper)")}
        >
          <div className="mx-auto grid w-full max-w-[1760px] grid-cols-[repeat(auto-fit,minmax(min(100%,420px),1fr))] items-center gap-[clamp(2rem,5vw,6rem)] px-[clamp(1rem,2.2vw,2rem)]">
            <div className="max-w-[40rem]">
              <Copy id={b.id} f={figures} mode={mode} person={person} onPick={setPerson} />
            </div>
            <Still id={b.id} figures={figures} />
          </div>
        </section>
      ))}
    </div>
  );
}

/** The scroll room: a sticky canvas behind sticky copy panels, one section per beat. */
function Room({
  figures,
  mobile,
  person,
  onPick,
  onSlow,
}: {
  figures: StoryFigures;
  mobile: boolean;
  person: CastKey;
  onPick: (k: CastKey) => void;
  onSlow: (beat: BeatId) => void;
}) {
  const track = useRef<HTMLDivElement>(null);
  const { scrollYProgress } = useScroll({ target: track, offset: ["start start", "end end"] });
  const slow = useCallback(() => onSlow(beatAt(scrollYProgress.get())), [onSlow, scrollYProgress]);

  return (
    <div ref={track} data-story="3d" className="relative" style={{ ["--track" as string]: `${mobile ? TRACK_VH.mobile : TRACK_VH.desktop}dvh` }}>
      <div className="sticky top-0 -mb-[100dvh] h-dvh overflow-hidden" data-dots={figures.closuresDone}>
        <StageCanvas progress={scrollYProgress} person={person} figures={figures} mobile={mobile} onSlow={slow} />
      </div>
      {BEATS.map((b) => (
        <section
          key={b.id}
          id={sectionId(b.id)}
          data-beat={b.id}
          className="relative"
          style={{ height: `calc(var(--track) * ${(b.to - b.from).toFixed(3)})` }}
        >
          <div className="sticky top-[max(1rem,8dvh)] mx-auto w-full max-w-[1760px] px-[clamp(1rem,2.2vw,2rem)] max-md:top-[44dvh]">
            <div className={cn(panel, "max-w-[36rem] md:max-w-[clamp(26rem,33vw,38rem)]")}>
              <Copy id={b.id} f={figures} mode="3d" person={person} onPick={onPick} />
            </div>
          </div>
        </section>
      ))}
    </div>
  );
}
