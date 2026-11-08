"use client";

import { useEffect, useId, useRef, useState, useTransition } from "react";
import Link from "next/link";
import { AnimatePresence, MotionConfig, motion } from "motion/react";
import { signReview, type SavedEvent } from "@/app/app/review/[id]/actions";
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
  type ReportAnswer,
} from "./decision";
import { IdentityBridge } from "./identity-bridge";
import { plain as plainRole } from "@/lib/domain/terms";
import { Pill } from "./marks";
import { ReadersTable } from "./readers-table";

export type NextCase = { href: string; label: string; last: boolean };

type Done = { outcome: OutcomeKey; signed: number; events: SavedEvent[]; next: NextCase };

const appear = { initial: { opacity: 0, y: 4 }, animate: { opacity: 1, y: 0 }, exit: { opacity: 0 }, transition: { duration: 0.16 } };

/** Native radios in a fieldset: arrow keys, Space and the group name come from the browser. */
function Chips<T extends string>({
  name,
  options,
  value,
  onPick,
  disabled,
}: {
  name: string;
  options: [T, string][];
  value: T | undefined;
  onPick: (v: T) => void;
  disabled: boolean;
}) {
  return (
    <div className="mt-2 flex flex-wrap gap-2">
      {options.map(([v, label]) => (
        <label
          key={v}
          className={cn(
            "relative flex min-h-11 min-w-[7rem] flex-1 cursor-pointer items-center justify-center rounded-chip px-3 py-1 text-center text-ink has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-hue",
            value === v ? "border-2 border-ink bg-active font-medium" : "border border-control bg-surface hover:bg-active",
          )}
        >
          <input
            type="radio"
            name={name}
            value={v}
            checked={value === v}
            disabled={disabled}
            onChange={() => onPick(v)}
            className="absolute -inset-0.5 m-0 size-[calc(100%+4px)] cursor-pointer opacity-0"
          />
          {label}
        </label>
      ))}
    </div>
  );
}

/**
 * Signed nodes appear as soon as the API holds them, so a partial live save shows its one square.
 * In fixtures mode nothing is signed: the answers show as a preview, never as a signature.
 */
function trailFor(view: ReviewView, draft: Draft, done: Done | undefined, saved: SavedEvent[], mode: ApiMode): ChainNode[] {
  const answers = signaturesFor(draft, view.items).filter((s) => done || mode === "fixtures" || saved.some((x) => x.item === s.item));
  const now = done ? OUTCOMES[done.outcome].next(view.unit) : { who: SIGNER.name, what: "your answers" };
  if (mode === "fixtures") {
    const preview: ChainNode[] = answers.map((s) => ({
      id: `preview-${s.item}`,
      kind: "current",
      who: SIGNER.name,
      what: `preview, not saved: ${SIGNS[s.decision]} (as ${roleFor(s.decision)})`,
      when: "not saved",
    }));
    return [...view.trail, ...preview, { id: "now", kind: "current", ...now, when: done ? "if saved" : "waiting" }];
  }
  const signed: ChainNode[] = answers.map((s) => {
    const ev = saved.find((x) => x.item === s.item);
    return {
      id: `signed-${s.item}`,
      kind: "human",
      who: ev?.actor || SIGNER.name,
      what: `${SIGNS[s.decision]} (as ${ev?.role || roleFor(s.decision)})`,
      when: "just now",
      hash: ev?.hash,
    };
  });
  return [...view.trail, ...signed, { id: "now", kind: "current", ...now, when: done ? "next" : "waiting" }];
}

export function ReviewDesk({
  view,
  mode,
  next,
  waiting = true,
  settled,
}: {
  view: ReviewView;
  mode: ApiMode;
  next: NextCase;
  /** False once the API no longer lists this case as open. The desk stays if the confirmation is still on screen. */
  waiting?: boolean;
  /** What to show instead of the desk when the case is not waiting. */
  settled?: React.ReactNode;
}) {
  const [draft, setDraft] = useState<Draft>({});
  const [done, setDone] = useState<Done>();
  const [error, setError] = useState<string>();
  /** Live saves are append-only: a retry after a partial save skips what the API already holds. */
  const [saved, setSaved] = useState<SavedEvent[]>([]);
  const [saving, startSaving] = useTransition();
  const resultHeading = useRef<HTMLHeadingElement>(null);
  const formRef = useRef<HTMLDivElement>(null);
  const undone = useRef(false);
  const q1 = useId();
  const q2 = useId();

  const pending = outcomeOf(draft);
  const outcome = done && OUTCOMES[done.outcome];
  const live = mode === "live";

  /*
   * Closing removes the questions and the button, so focus would fall to <body>.
   * It moves to the confirmation heading; the next Tab reaches the link on to the next case.
   * Undoing brings the questions back, and focus goes to the first of them.
   */
  useEffect(() => {
    if (done) resultHeading.current?.focus();
    else if (undone.current) {
      undone.current = false;
      formRef.current?.querySelector<HTMLInputElement>("input[type=radio]")?.focus();
    }
  }, [done]);

  function pickIdentity(identity: IdentityAnswer) {
    setDraft({ identity });
    setError(undefined);
  }
  function pickReport(report: ReportAnswer) {
    setDraft((d) => ({ ...d, report }));
    setError(undefined);
  }

  function sign() {
    if (!pending || saving) return;
    const signatures = signaturesFor(draft, view.items);
    if (!live) {
      setDone({ outcome: pending, signed: 0, events: [], next });
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
      setDone({ outcome: pending, signed: signatures.length, events: all, next });
    });
  }

  if (!waiting && !done) return <>{settled}</>;

  const asks = draft.identity === "same";
  const announce = asks
    ? "Question 2 of 2 is now showing: is the colonoscopy report adequate?"
    : draft.identity === "different"
      ? "Not the same patient. There is no second question."
      : "";

  return (
    <MotionConfig reducedMotion="user">
      <div className="mt-4 grid items-start gap-3 md:mt-6 xl:grid-cols-[minmax(0,1fr)_380px]">
        {/* The decision comes first in reading order and on a phone; the evidence it rests on follows. On a wide screen it sits to the right. */}
        <aside aria-labelledby="decision-h" className="min-w-0 rounded-card border border-line bg-surface p-4 md:p-6 xl:col-start-2 xl:row-start-1">
          <h2 id="decision-h" className="m-0 text-section font-medium">
            Your answers
          </h2>
          <p className="m-0 mt-0.5 text-small text-muted">
            {live
              ? `One signer, two answers: ${SIGNER.name} (${SIGNER.title}) gives both, each recorded under its own role. That is not independent double review.`
              : "A reconstruction: nothing here is appended or saved. Signing needs the live API."}
          </p>

          {!done && (
            <div ref={formRef} className="mt-4 flex flex-col gap-3">
              <div aria-live="polite" className="sr-only">
                {announce}
              </div>
              <fieldset className="m-0 min-w-0 border-0 p-0" aria-describedby={`${q1}-who`}>
                <legend className="p-0 text-body font-medium">1. Is it the same patient?</legend>
                <p id={`${q1}-who`} className="m-0 mt-0.5 text-small text-muted">
                  Recorded as reviewer: {SIGNER.name}.
                </p>
                <Chips
                  name={`${q1}-identity`}
                  options={[
                    ["same", "Same patient"],
                    ["different", "Not the same patient"],
                  ]}
                  value={draft.identity}
                  onPick={pickIdentity}
                  disabled={saving}
                />
              </fieldset>
              <AnimatePresence initial={false}>
                {asks && (
                  <motion.div key="q2" {...appear} className="border-t border-rule pt-3">
                    <fieldset className="m-0 min-w-0 border-0 p-0" aria-describedby={`${q2}-who ${q2}-ask`}>
                      <legend className="p-0 text-body font-medium">2. Is the colonoscopy report adequate?</legend>
                      <p id={`${q2}-who`} className="m-0 mt-0.5 text-small text-muted">
                        {view.disputed.length > 0 ? `The readers could not settle: ${view.disputed.map((d) => d.says).join("; ")}. ` : ""}
                        One answer covers {view.disputed.length > 1 ? "all of them" : "it"}. Recorded as clinician: {SIGNER.name}, the same person as question 1.
                      </p>
                      <p id={`${q2}-ask`} className="m-0 mt-0.5 text-small text-muted">
                        &ldquo;Ask the unit&rdquo; is designed, not built: it records the identity answer only and sends nothing.
                      </p>
                      <Chips
                        name={`${q2}-report`}
                        options={[
                          ["adequate", "Adequate"],
                          ["inadequate", "Not adequate"],
                          ["ask", "Ask the unit"],
                        ]}
                        value={draft.report}
                        onPick={pickReport}
                        disabled={saving}
                      />
                    </fieldset>
                  </motion.div>
                )}
              </AnimatePresence>
              {pending ? (
                <button
                  type="button"
                  onClick={sign}
                  aria-disabled={saving}
                  className="min-h-11 rounded-control bg-cta px-4 font-medium text-white hover:bg-ink aria-disabled:opacity-60"
                >
                  {saving ? "Signing…" : live ? OUTCOMES[pending].commit : `Preview: ${OUTCOMES[pending].commit}`}
                </button>
              ) : (
                <p className="m-0 text-small text-muted">
                  {draft.identity === "same" ? "Answer the colonoscopy report question to sign." : "Answer both questions to sign."}
                </p>
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
                  <h3 ref={resultHeading} tabIndex={-1} className="m-0 text-body font-normal">
                    <Pill tone={outcome.tone}>{live ? outcome.pill : outcome.preview}</Pill>
                  </h3>
                  {live && (
                    <span
                      role="img"
                      aria-label={done.signed === 1 ? "One answer recorded" : `${done.signed} answers recorded`}
                      className="inline-flex gap-1"
                      data-testid="signature-squares"
                    >
                      {Array.from({ length: done.signed }, (_, i) => (
                        <span key={i} className="size-2.5 bg-ink" />
                      ))}
                    </span>
                  )}
                </div>
                {live ? (
                  <>
                    <p className="m-0 mt-2">{outcome.text(view.name.split(" ")[0], view.unit)}</p>
                    <p className="m-0 mt-2 text-small text-muted">
                      Saved: {done.events.length} {done.events.length === 1 ? "event" : "events"} appended to the review log, last hash{" "}
                      <span className="font-mono">{done.events.at(-1)?.hash.slice(0, 12)}</span>.
                      {done.outcome === "ask" && " Asking the unit is designed, not built."}
                    </p>
                    <ul aria-label="Appended events" className="m-0 mt-1 list-none p-0 text-small text-muted">
                      {done.events.map((e) => (
                        <li key={e.hash}>
                          {plainRole(e.role)} · {e.actor} · <span className="font-mono">{e.hash.slice(0, 12)}</span>
                        </li>
                      ))}
                    </ul>
                    <p className="m-0 mt-1 text-small text-muted">
                      {done.events.length > 1 && new Set(done.events.map((e) => e.actor)).size === 1
                        ? `Both answers came from ${done.events[0].actor}. They are separate entries under two roles, not two independent reviewers. `
                        : ""}
                      The review history below was read again from the log.
                    </p>
                  </>
                ) : (
                  <>
                    <div className="mt-2">
                      <HonestyLabel kind="designed">Reconstruction: nothing was saved</HonestyLabel>
                    </div>
                    <p className="m-0 mt-2">
                      No event was appended and no hash exists. If this were saved against the live API: {outcome.text(view.name.split(" ")[0], view.unit)}
                    </p>
                  </>
                )}
                <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
                  <Link
                    href={done.next.href}
                    className="inline-flex min-h-11 items-center rounded-control bg-cta px-4 font-medium text-white no-underline hover:bg-ink hover:text-white"
                  >
                    {done.next.label}
                  </Link>
                  {!live && (
                    <button
                      type="button"
                      onClick={() => {
                        undone.current = true;
                        setDone(undefined);
                        setDraft({});
                      }}
                      className="min-h-11 rounded-control px-3 underline hover:bg-active"
                    >
                      Undo
                    </button>
                  )}
                </div>
                {done.next.last && <p className="m-0 mt-2 text-small text-muted">That was the last case waiting.</p>}
              </motion.div>
            )}
          </AnimatePresence>

          <h3 className="m-0 mt-5 text-body font-medium">The case so far</h3>
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
          <SealedChain nodes={trailFor(view, draft, done, saved, mode)} className="mt-3" />
        </aside>

        <div className="flex min-w-0 flex-col gap-3 xl:col-start-1 xl:row-start-1">
          <IdentityBridge view={view} answer={draft.identity} />
          <ReadersTable view={view} signed={outcome && { ...outcome.answerCell, word: live ? outcome.answerCell.word : `${outcome.answerCell.word.replace(", signed", "")} (preview, not saved)` }} />
        </div>
      </div>
    </MotionConfig>
  );
}
