import Link from "next/link";
import { PEOPLE } from "@/components/site/name-joiner";
import { Wordmark } from "@/components/site/wordmark";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { HonestyKind } from "@/lib/honesty";
import { cn } from "@/lib/utils";
import { CAST, CAST_ORDER, type CastKey, type StoryFigures, type TextId } from "./script";

/**
 * The words of the story, one body per cue. `W` marks text that reveals word by word as the visitor
 * scrolls; everything else in a cue arrives with the cue as a whole.
 */

export function W({ children }: { children: string }) {
  return (
    <>
      {children.split(/(\s+)/).map((part, i) => (/\s/.test(part) || !part ? part : <span key={i} data-w className="story-w">{part}</span>))}
    </>
  );
}

const mute = "text-(--muted) italic font-normal";

/** The honesty word, inline beside its number: italic, muted, no pill. */
function Honest({ kind, children }: { kind: HonestyKind; children: string }) {
  return (
    <HonestyLabel kind={kind} className={cn("inline h-auto whitespace-normal bg-transparent px-0 text-[0.8em] leading-[inherit]", mute)}>
      {children}
    </HonestyLabel>
  );
}

export const type = {
  display: "text-[clamp(2.5rem,1rem_+_3.6vw,5.25rem)] m-0 font-bold leading-[0.98] tracking-[-0.04em] text-balance",
  figure: "text-[clamp(3.25rem,1rem_+_4.4vw,6.5rem)] m-0 font-bold leading-none tracking-[-0.045em]",
  caption: "text-[clamp(1.0625rem,0.9rem_+_0.4vw,1.375rem)] m-0 font-semibold leading-[1.3] text-pretty",
  line: "text-[clamp(1.125rem,0.85rem_+_0.75vw,1.75rem)] m-0 font-medium leading-[1.3] text-pretty",
  body: "text-[clamp(1rem,0.9rem_+_0.3vw,1.25rem)] m-0 leading-[1.45] text-pretty",
};

export const cta =
  "inline-flex min-h-12 items-center bg-(--ink) [text-shadow:none] px-6 py-3 text-[18px] font-semibold leading-tight text-(--paper) no-underline hover:bg-(--teal) hover:text-(--paper)";
export const ctaLink = "inline-flex min-h-12 items-center text-[18px] font-semibold text-(--teal) underline decoration-[0.07em] underline-offset-[0.2em] hover:text-(--ink)";

function Ctas() {
  return (
    <div className="mt-7 flex flex-wrap items-center gap-x-7 gap-y-3 max-md:flex-col max-md:items-stretch">
      <Link href="/app" className={cn(cta, "max-md:justify-center")}>
        Open the dashboard
      </Link>
      <Link href="/app/pipeline" className={cn(ctaLink, "max-md:justify-center")}>
        See the pipeline
      </Link>
    </div>
  );
}

/** What the join line says while the thread works: the name first, then the outcome for the chip picked. */
export const JOIN_SAYS = {
  name: "Similar name. Not enough on its own.",
  joined: "Same birth date. Same person.",
  apart: "Different birth date. Two different people.",
  review: "Birth dates almost match. A person decides.",
} as const;

export type JoinStep = "name" | "outcome";

function Chips({ person, onPick, step }: { person: CastKey; onPick: (k: CastKey) => void; step: JoinStep }) {
  const p = PEOPLE[person];
  const outcome = CAST[person].outcome;
  return (
    <div className="flex flex-col items-center text-center">
      <p className={cn(type.caption, "text-[clamp(1.125rem,0.9rem_+_0.5vw,1.5rem)] leading-[1.3]")} aria-live="polite" data-testid="verdict" data-outcome={step === "name" ? "pending" : outcome}>
        {step === "name" ? JOIN_SAYS.name : JOIN_SAYS[outcome]}
      </p>
      <p className="m-0 mt-2 text-[16px] leading-[1.4] text-(--muted)">
        <span lang="ar" dir="rtl" className="font-arabic font-semibold text-(--ink)" data-testid="clinic-name">
          {p.arabic}
        </span>{" "}
        at the clinic,{" "}
        <span className="font-semibold text-(--ink)" data-testid="hospital-name">
          {p.latin}
        </span>{" "}
        at the hospital
      </p>
      <div role="group" aria-label="Choose a person" className="mt-4 flex w-full justify-center gap-2 max-md:grid max-md:grid-cols-3">
        {CAST_ORDER.map((k) => (
          <button
            key={k}
            type="button"
            aria-pressed={k === person}
            onClick={() => onPick(k)}
            className={cn(
              "min-h-12 cursor-pointer border-0 px-5 py-2.5 text-[18px] font-semibold",
              k === person ? "bg-(--ink) text-(--paper)" : "bg-(--card) text-(--ink) shadow-[inset_0_0_0_1px_var(--sand-deep)] hover:bg-(--sand)",
            )}
          >
            {CAST[k].first}
          </button>
        ))}
      </div>
      <p className="m-0 mt-2 text-[15px] text-(--muted)">Try another name</p>
    </div>
  );
}

const RECEIPT = [
  { label: "Follow-up check", value: "Complete" },
  { label: "Proof, in the hospital’s words", value: "“All surfaces were well seen”" },
  { label: "Read by", value: "Two readers, and both agree" },
  { label: "Matched by", value: "Her name and her birth date" },
];

function Receipt() {
  return (
    <div
      role="img"
      aria-label="A Wasl receipt for Fatima Al Zaabi: follow-up check complete, with the hospital's own words as proof, read by two readers who agree, matched by her name and birth date. Done, with proof."
      className="mt-4 w-full max-w-[24rem] bg-(--card) px-6 pb-6 pt-6 shadow-[0_1px_0_var(--sand-deep),0_18px_40px_-24px_rgb(40_30_20/0.35)]"
    >
      <div className="text-[14px] font-semibold uppercase tracking-[0.08em] text-(--muted)">Receipt</div>
      <div className="mt-2 text-[26px] font-bold leading-[1.05] tracking-[-0.02em]">Fatima Al Zaabi</div>
      <dl className="mt-4 flex flex-col gap-2">
        {RECEIPT.map((r) => (
          <div key={r.label}>
            <dt className="text-[14px] leading-[1.4] text-(--muted)">{r.label}</dt>
            <dd className="m-0 text-[16px] font-semibold leading-[1.35]">{r.value}</dd>
          </div>
        ))}
      </dl>
      <div className="mt-4 bg-(--teal) px-4 py-2.5 text-[17px] font-bold text-(--paper) [text-shadow:none]">Done, with proof</div>
    </div>
  );
}

export function CueBody({
  id,
  f,
  person,
  onPick,
  step,
  still = false,
}: {
  id: TextId;
  f: StoryFigures;
  person: CastKey;
  onPick: (k: CastKey) => void;
  step: JoinStep;
  /** In the stills the join is the 2D picker, placed by the caller. */
  still?: boolean;
}) {
  switch (id) {
    case "hero":
      return (
        <>
          <h1 className={cn(type.display, "text-[clamp(2.75rem,1rem_+_4.6vw,6.25rem)] leading-[0.96]")}>Nobody lost between hospitals</h1>
          <p className={cn(type.body, "mt-5 max-w-[32rem]")}>
            When a screening test asks for a closer look, Wasl makes sure that person gets the follow-up check, and marks it done only with
            proof.
          </p>
          <Ctas />
        </>
      );
    case "her-test":
      return (
        <p className={type.caption}>
          <W>Fatima. Her screening test asked for a closer look.</W>
        </p>
      );
    case "her-clinic":
      return (
        <p className={type.caption}>
          <W>Her clinic, Al Mushrif.</W>
          <br />
          <span lang="ar" dir="rtl" className="font-arabic">
            فاطمة الزعابي
          </span>{" "}
          <span className="font-normal">· ID number on file</span>
        </p>
      );
    case "booked":
      return (
        <p className={type.line}>
          <W>Her follow-up check was booked at a hospital across the city.</W>
        </p>
      );
    case "wait":
      return (
        <>
          <div className={cn(type.figure, "text-(--teal)")} data-figure="wait">
            {f.waitDays} days
          </div>
          <p className={cn(type.body, "mt-3 max-w-[24rem]")}>
            <W>{`The typical wait for this check in Abu Dhabi, against a ${f.standardDays}-day standard.`}</W>{" "}
            <Honest kind="published">published study, 2021</Honest>
          </p>
        </>
      );
    case "written":
      return (
        <p className={type.caption}>
          <span className="font-normal">
            <W>Written here as</W>
          </span>
          <br />
          F. Alzabi <span className="font-normal">· no ID number</span>
        </p>
      );
    case "nobody-knew":
      return (
        <h2 className={type.display}>
          <W>Done, and nobody knew.</W>
        </h2>
      );
    case "calling":
      return (
        <p className={type.caption}>
          <W>So the nurse keeps calling.</W>
        </p>
      );
    case "never-hears":
      return (
        <p className={cn(type.line, "font-semibold")}>
          <W>Her clinic never hears back.</W>
        </p>
      );
    case "missed":
      return (
        <>
          <div className={cn(type.figure, "text-(--teal)")} data-figure="missed">
            About {f.missedOfFive} in 5
          </div>
          <p className={cn(type.body, "mt-3 max-w-[27rem]")}>
            <W>people who had their check looked as if they had not, when records were matched by ID number alone.</W>{" "}
            <Honest kind="simulated">simulated, rounded</Honest>
          </p>
        </>
      );
    case "wasl":
      return (
        <>
          <h2 className={type.display}>
            <W>Wasl follows her.</W>
          </h2>
          <p className={cn(type.line, "mt-3")}>
            In Arabic,{" "}
            <span lang="ar" dir="rtl" className="font-arabic font-semibold text-(--teal)">
              وصل
            </span>{" "}
            <W>means a connection.</W>
          </p>
        </>
      );
    case "follows":
      return (
        <p className={type.caption}>
          <W>It goes the way she went.</W>
        </p>
      );
    case "spellings":
      return (
        <h2 className={type.display}>
          <W>One person, two spellings.</W>
        </h2>
      );
    case "join":
      return still ? null : <Chips person={person} onPick={onPick} step={step} />;
    case "quote":
      return (
        <>
          <p className={cn(type.line, "font-semibold")}>
            <W>“All surfaces were well seen.”</W>
          </p>
          <p className={cn(type.body, "mt-2")}>
            <W>Two readers, fixed rules and an AI. Both agree.</W>
          </p>
        </>
      );
    case "none-wrong":
      return (
        <>
          <div className={cn(type.figure, "text-(--teal)")}>{f.closuresWrong === 0 ? "None wrong" : `${f.closuresWrong} wrong`}</div>
          <p className={cn(type.body, "mt-3 max-w-[24rem]")} data-figure="closures">
            <W>{`of ${f.closuresDone.toLocaleString("en-US")} cases Wasl marked done.`}</W> <Honest kind="simulated">simulated</Honest>
          </p>
        </>
      );
    case "receipt-word":
      return (
        <p className={type.caption}>
          In Arabic,{" "}
          <span lang="ar" dir="rtl" className="font-arabic text-(--teal)">
            وصل
          </span>{" "}
          <W>is also a word for a receipt.</W>
        </p>
      );
    case "receipt":
      return (
        <>
          <p className={type.caption}>
            <W>It reaches her clinic. The nurse stops calling.</W>
          </p>
          <Receipt />
        </>
      );
    case "who":
      return (
        <ul className={cn(type.body, "m-0 flex max-w-[30rem] list-none flex-col gap-2 p-0")}>
          {WHO.map((w) => (
            <li key={w.who}>
              <span className="font-bold">
                <W>{w.who}</W>
              </span>{" "}
              <W>{w.what}</W>
            </li>
          ))}
        </ul>
      );
    case "close":
      return (
        <div className="flex flex-col items-center text-center">
          <Wordmark />
          <h2 className={cn(type.display, "mt-6")}>
            <W>Nobody lost between hospitals.</W>
          </h2>
          <div className="flex justify-center">
            <Ctas />
          </div>
        </div>
      );
  }
}

const WHO = [
  { who: "The person waiting.", what: "A message in her own language." },
  { who: "The nurse.", what: "Calls go to the people who still need them." },
  { who: "The health system.", what: "Every case marked done carries its proof." },
  { who: "The auditor.", what: "Every decision written down, with who made it." },
];
