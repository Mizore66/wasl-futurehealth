"use client";

import { useState, useTransition } from "react";
import Link from "next/link";
import { AnimatePresence, motion } from "motion/react";
import { signReview } from "@/app/app/review/[id]/actions";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { SealedChain, type ChainNode } from "@/components/wasl/sealed-chain";
import type { ApiMode } from "@/lib/api/source";
import { cn } from "@/lib/utils";
import type { ReviewView } from "./case-view";
import {
  failureSentence,
  outcomeOf,
  OUTCOMES,
  roleFor,
  SIGNER,
  SIGNS,
  signaturesFor,
  type Draft,
  type IdentityAnswer,
  type OutcomeKey,
  type PrepAnswer,
} from "./decision";
import { IdentityBridge } from "./identity-bridge";
import { Pill } from "./marks";
import { ReadersTable } from "./readers-table";

export type NextCase = { href: string; label: string; last: boolean };

type Done = { outcome: OutcomeKey; signed: number; hashes: string[] };

const appear = { initial: { opacity: 0, y: 4 }, animate: { opacity: 1, y: 0 }, exit: { opacity: 0 }, transition: { duration: 0.16 } };

function Chips<T extends string>({
  options,
  value,
  onPick,
  disabled,
}: {
  options: [T, string][];
  value: T | undefined;
  onPick: (v: T) => void;
  disabled: boolean;
}) {
  return (
    <div className="mt-2 flex gap-2">
      {options.map(([v, label]) => (
        <button
          key={v}
          type="button"
          role="radio"
          aria-checked={value === v}
          disabled={disabled}
          onClick={() => onPick(v)}
          className={cn(
            "h-9 flex-1 whitespace-nowrap rounded-chip px-2 text-ink",
            value === v ? "border-2 border-ink bg-active font-medium" : "border border-control bg-surface hover:bg-active",
          )}
        >
          {label}
        </button>
      ))}
    </div>
  );
}

/** Signed nodes appear as soon as the API holds them, so a partial live save shows its one square. */
function trailFor(view: ReviewView, draft: Draft, done: Done | undefined, saved: { item: string }[]): ChainNode[] {
  const signed: ChainNode[] = signaturesFor(draft, view.items)
    .filter((s) => done || saved.some((x) => x.item === s.item))
    .map((s) => ({ id: `signed-${s.item}`, kind: "human", who: SIGNER.name, what: `${SIGNS[s.decision]} (as ${roleFor(s.decision)})`, when: "just now" }));
  const now = done ? OUTCOMES[done.outcome].next(view.unit) : { who: SIGNER.name, what: "your signature" };
  return [...view.trail, ...signed, { id: "now", kind: "current", ...now, when: done ? "next" : "waiting" }];
}

export function ReviewDesk({ view, mode, next }: { view: ReviewView; mode: ApiMode; next: NextCase }) {
  const [draft, setDraft] = useState<Draft>({});
  const [done, setDone] = useState<Done>();
  const [error, setError] = useState<string>();
  /** Live saves are append-only: a retry after a partial save skips what the API already holds. */
  const [saved, setSaved] = useState<{ item: string; hash: string }[]>([]);
  const [saving, startSaving] = useTransition();

  const pending = outcomeOf(draft);
  const outcome = done && OUTCOMES[done.outcome];

  function pickIdentity(identity: IdentityAnswer) {
    setDraft({ identity });
    setError(undefined);
  }
  function pickPrep(prep: PrepAnswer) {
    setDraft((d) => ({ ...d, prep }));
    setError(undefined);
  }

  function sign() {
    if (!pending) return;
    const signatures = signaturesFor(draft, view.items);
    if (mode === "fixtures") {
      setDone({ outcome: pending, signed: signatures.length, hashes: [] });
      return;
    }
    startSaving(async () => {
      const res = await signReview(
        view.caseId,
        signatures.filter((s) => !saved.some((x) => x.item === s.item)),
      );
      const all = [...saved, ...res.saved];
      setSaved(all);
      if (res.failed) {
        const identitySaved = all.some((x) => x.item === view.items.link) ? "The identity answer was saved. " : "";
        setError(identitySaved + failureSentence(res.failed.signature, res.failed.failure));
        return;
      }
      setDone({ outcome: pending, signed: signatures.length, hashes: all.map((x) => x.hash) });
    });
  }

  return (
    <div className="mt-6 flex flex-wrap items-start gap-3">
      <div className="flex min-w-0 flex-[999_1_560px] flex-col gap-3">
        <IdentityBridge view={view} answer={draft.identity} />
        <ReadersTable view={view} signed={outcome?.prepCell} />
      </div>

      <aside aria-labelledby="decision-h" className="min-w-0 flex-[1_1_360px] rounded-card border border-line bg-surface p-6">
        <h2 id="decision-h" className="m-0 text-section font-medium">
          Your signature
        </h2>
        <p className="m-0 mt-0.5 text-small text-muted">Two answers, two signatures. Nothing closes on one.</p>

        {!done && (
          <div className="mt-4 flex flex-col gap-3">
            <div role="radiogroup" aria-labelledby="q1-h">
              <h3 id="q1-h" className="m-0 text-body font-medium">
                1. Is it the same patient?
              </h3>
              <Chips
                options={[
                  ["same", "Same patient"],
                  ["different", "Not the same patient"],
                ]}
                value={draft.identity}
                onPick={pickIdentity}
                disabled={saving}
              />
            </div>
            <AnimatePresence initial={false}>
              {draft.identity === "same" && (
                <motion.div key="q2" {...appear} role="radiogroup" aria-labelledby="q2-h" className="border-t border-rule pt-3">
                  <h3 id="q2-h" className="m-0 text-body font-medium">
                    2. Bowel prep: is it adequate?
                  </h3>
                  <p className="m-0 mt-0.5 text-small text-muted">Neither reader found a grade in the report. You sign this as clinician.</p>
                  <Chips
                    options={[
                      ["adequate", "Adequate"],
                      ["inadequate", "Not adequate"],
                      ["ask", "Ask the unit"],
                    ]}
                    value={draft.prep}
                    onPick={pickPrep}
                    disabled={saving}
                  />
                </motion.div>
              )}
            </AnimatePresence>
            {pending ? (
              <button
                type="button"
                onClick={sign}
                disabled={saving}
                className="h-9 rounded-control bg-cta px-4 font-medium text-white hover:bg-ink disabled:opacity-60"
              >
                {saving ? "Signing…" : OUTCOMES[pending].commit}
              </button>
            ) : (
              <p className="m-0 text-small text-muted">{draft.identity === "same" ? "Answer the bowel prep question to sign." : "Answer both questions to sign."}</p>
            )}
            {error && (
              <p role="alert" className="m-0 rounded-control bg-pill-negative-bg px-3 py-2 text-pill-negative-ink">
                {error}
              </p>
            )}
          </div>
        )}

        <AnimatePresence>
          {done && outcome && (
            <motion.div key="result" {...appear} role="status" className="mt-4 rounded-control bg-ground px-4 py-3">
              <div className="flex items-center justify-between gap-2">
                <Pill tone={outcome.tone}>{outcome.pill}</Pill>
                <span role="img" aria-label={done.signed === 1 ? "One signature" : `${done.signed} signatures`} className="inline-flex gap-1" data-testid="signature-squares">
                  {Array.from({ length: done.signed }, (_, i) => (
                    <span key={i} className="size-2.5 bg-ink" />
                  ))}
                </span>
              </div>
              <p className="m-0 mt-2">{outcome.text(view.name.split(" ")[0], view.unit)}</p>
              <div className="mt-2">
                {mode === "fixtures" ? (
                  <HonestyLabel kind="designed">Demo: not saved</HonestyLabel>
                ) : (
                  <p className="m-0 text-small text-muted">
                    Saved: {done.signed} {done.signed === 1 ? "event" : "events"} appended to the review log, last hash{" "}
                    <span className="font-mono">{done.hashes.at(-1)?.slice(0, 12)}</span>.
                    {done.outcome === "ask" && " Asking the unit is designed, not built."}
                  </p>
                )}
              </div>
              <div className="mt-3 flex items-center justify-between">
                <Link href={next.href} className="inline-flex h-9 items-center rounded-control bg-cta px-4 font-medium text-white no-underline hover:bg-ink hover:text-white">
                  {next.label}
                </Link>
                {mode === "fixtures" && (
                  <button
                    type="button"
                    onClick={() => {
                      setDone(undefined);
                      setDraft({});
                    }}
                    className="h-8 rounded-control px-2 underline hover:bg-active"
                  >
                    Undo
                  </button>
                )}
              </div>
              {next.last && <p className="m-0 mt-2 text-small text-muted">That was the last case waiting.</p>}
            </motion.div>
          )}
        </AnimatePresence>

        <h3 className="m-0 mt-5 text-body font-medium">Her case so far</h3>
        <div aria-label="Key" className="mt-1 flex flex-wrap gap-3 text-small text-muted">
          <span className="inline-flex items-center gap-1.5">
            <span aria-hidden className="size-2.5 rounded-full border-2 border-ink" />
            Automatic
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span aria-hidden className="size-2.5 bg-ink" />
            Signed
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span aria-hidden className="size-2.5 rounded-full bg-hue" />
            Now
          </span>
        </div>
        <SealedChain nodes={trailFor(view, draft, done, saved)} className="mt-3" />
      </aside>
    </div>
  );
}
