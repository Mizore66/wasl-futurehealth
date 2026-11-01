"use client";

import { AnimatePresence, motion } from "motion/react";
import { useState } from "react";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cn } from "@/lib/utils";
import { h3, small } from "./styles";

/** One person as two places wrote them down. Illustrative cast; Fatima follows the canonical record. */
type Pair = {
  first: string;
  arabic: string;
  latin: string;
  clinicDetail: string;
  hospitalDetail: string;
  joined: boolean;
  reason: string;
};

export const PEOPLE = {
  fatima: {
    first: "Fatima",
    arabic: "فاطمة الزعابي",
    latin: "F. Alzabi",
    clinicDetail: "Born 14 March 1972. ID number on file.",
    hospitalDetail: "Born 14 March 1972. No ID number.",
    joined: true,
    reason: "The spellings match, and so does her birth date. Her records are joined.",
  },
  khalid: {
    first: "Khalid",
    arabic: "خالد الكتبي",
    latin: "Khaled Alkitbi",
    clinicDetail: "Born 3 October 1959. ID number on file.",
    hospitalDetail: "Born 3 October 1959. No ID number.",
    joined: true,
    reason: "A different spelling, the same birth date. His records are joined.",
  },
  aisha: {
    first: "Aisha",
    arabic: "عائشة المزروعي",
    latin: "Aysha Al Mazroui",
    clinicDetail: "Born 2 June 1971. ID number on file.",
    hospitalDetail: "Born 9 November 1983. No ID number.",
    joined: false,
    reason: "The names are close, but the birth dates differ. A person decides.",
  },
} satisfies Record<string, Pair>;

export type Key = keyof typeof PEOPLE;
const ORDER: Key[] = ["fatima", "khalid", "aisha"];

/** The thread between the two records: one solid teal line when joined, two grey stubs with a gap when not. */
function Thread({ joined }: { joined: boolean }) {
  const draw = { initial: { scaleY: 0 }, animate: { scaleY: 1 }, exit: { opacity: 0 }, transition: { duration: 0.45, ease: "easeOut" as const } };
  return (
    <div aria-hidden className="relative w-2 flex-none self-stretch">
      <AnimatePresence initial={false} mode="popLayout">
        {joined ? (
          <motion.span key="joined" data-thread="joined" {...draw} className="absolute inset-0 block origin-top bg-(--teal)" />
        ) : (
          <motion.span key="broken" data-thread="broken" {...draw} className="absolute inset-0 flex origin-top flex-col justify-between">
            <span className="block h-8 bg-(--stub)" />
            <span className="block h-8 bg-(--stub)" />
          </motion.span>
        )}
      </AnimatePresence>
    </div>
  );
}

function Mark({ joined }: { joined: boolean }) {
  return (
    <span
      aria-hidden
      className={cn(
        "mt-[-2px] inline-flex size-9 flex-none items-center justify-center text-(--paper)",
        joined ? "rounded-full bg-(--teal)" : "bg-(--ink)",
      )}
    >
      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
        <path d={joined ? "M5 12l5 5 9-10" : "M9 6v12M15 6v12"} />
      </svg>
    </span>
  );
}

const bigName = "mt-2 text-[clamp(2rem,1rem_+_3vw,4rem)] font-semibold leading-[1.15]";

export function NameJoiner() {
  const [key, setKey] = useState<Key>("fatima");
  const p = PEOPLE[key];

  return (
    <section aria-labelledby="try-title" className="bg-(--sand) p-[clamp(1.25rem,3vw,2.5rem)]">
      <h2 id="try-title" className={h3}>
        Pick a name and watch Wasl decide
      </h2>
      <div role="group" aria-label="Choose a person" className="mt-5 flex flex-wrap gap-2">
        {ORDER.map((k) => (
          <button
            key={k}
            type="button"
            aria-pressed={k === key}
            onClick={() => setKey(k)}
            className={cn(
              "min-h-12 cursor-pointer border-0 px-[22px] py-3 text-[18px] font-semibold",
              k === key ? "bg-(--ink) text-(--paper)" : "bg-(--card) text-(--ink) hover:bg-(--paper)",
            )}
          >
            {PEOPLE[k].first}
          </button>
        ))}
      </div>

      <div className="mt-8 bg-(--card) px-7 py-6">
        <div className={small}>Written at her clinic in Al Mushrif</div>
        <div lang="ar" dir="rtl" className={cn("font-arabic", bigName, "text-left")} data-testid="clinic-name">
          {p.arabic}
        </div>
        <div className="mt-2 text-[18px] leading-normal">{p.clinicDetail}</div>
      </div>

      <div className="flex min-h-[120px] items-stretch gap-6 px-7">
        <Thread joined={p.joined} />
        <div aria-live="polite" className="flex items-start gap-3.5 py-5">
          <Mark joined={p.joined} />
          <div>
            <div className="text-[24px] font-bold leading-[1.3] tracking-[-0.01em]" data-testid="verdict">
              {p.joined ? "Same person" : "Not joined yet"}
            </div>
            <div className="mt-1 text-[18px] leading-normal">{p.reason}</div>
          </div>
        </div>
      </div>

      <div className="bg-(--card) px-7 py-6">
        <div className={small}>Written at the hospital across the city</div>
        <div className={cn(bigName, "tracking-[-0.02em]")} data-testid="hospital-name">
          {p.latin}
        </div>
        <div className="mt-2 text-[18px] leading-normal">{p.hospitalDetail}</div>
      </div>
      <HonestyLabel kind="illustrative" className="mt-5 block h-auto bg-transparent px-0 text-[14px] leading-[1.45] text-(--muted)">
        Made-up people, for illustration.
      </HonestyLabel>
    </section>
  );
}
