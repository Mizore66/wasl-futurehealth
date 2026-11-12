"use client";

import { useEffect, useId, useRef, useState } from "react";
import Link from "next/link";
import { AnimatePresence, MotionConfig, motion } from "motion/react";
import { claimReviewQuestion, submitReviewRead } from "@/app/app/review/[id]/actions";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { SealedChain, type ChainNode } from "@/components/wasl/sealed-chain";
import type { ApiMode } from "@/lib/api/source";
import type { ReviewState } from "@/lib/api/schema";
import { allQuestionsResolved, finalDecisions, READ_LABEL, resolutionProgress } from "@/lib/domain/stage";
import type { ReviewView } from "./case-view";
import {
  answerWords,
  canAnswerClaim,
  claimSummary,
  draftKey,
  orderedStates,
  outcomeOf,
  questionAnchor,
  OUTCOMES,
  readFailureSentence,
  readLine,
  resolvedCell,
  roleFor,
  SIGNER,
  signWords,
  signaturesFor,
  stateHeadline,
  submittedReads,
  SUBMIT_LABEL,
  type Draft,
  type IdentityAnswer,
  type OutcomeKey,
  type ReportAnswer,
} from "./decision";
import { Chips } from "./chips";
import { IfhasLabelComposer } from "./ifhas-label-composer";
import { ReviewerSignOut } from "./reviewer-login";
import { IdentityBridge } from "./identity-bridge";
import { Pill } from "./marks";
import { ReadersTable } from "./readers-table";

export type NextCase = { href: string; label: string; last: boolean };

type Done = { outcome: OutcomeKey; next: NextCase };

const appear = { initial: { opacity: 0, y: 4 }, animate: { opacity: 1, y: 0 }, exit: { opacity: 0 }, transition: { duration: 0.16 } };

function EvidenceChoices({
  name,
  programme,
  options,
  value,
  onPick,
  disabled,
}: {
  name: string;
  programme: string;
  options: [ReportAnswer, string][];
  value: ReportAnswer | undefined;
  onPick: (value: ReportAnswer) => void;
  disabled: boolean;
}) {
  if (programme === "ifhas") return <IfhasLabelComposer name={name} value={value} onPick={onPick} disabled={disabled} />;
  if (options.length <= 3) return <Chips name={name} options={options} value={value} onPick={onPick} disabled={disabled} />;
  return (
    <select
      name={name}
      aria-label="Evidence label"
      value={value ?? ""}
      disabled={disabled}
      onChange={(event) => onPick(event.target.value)}
      className="mt-2 min-h-11 w-full rounded-control border border-control bg-surface px-3 text-ink"
    >
      <option value="" disabled>
        Select an evidence label
      </option>
      {options.map(([value, label]) => (
        <option key={value} value={value}>
          {label}
        </option>
      ))}
    </select>
  );
}

/** In fixtures mode nothing is signed: the answers show as a preview, never as a signature. */
function previewTrail(view: ReviewView, draft: Draft, done: Done | undefined): ChainNode[] {
  const answers = signaturesFor(draft, view.items);
  const now = done ? OUTCOMES[done.outcome].next(view.unit, Boolean(view.items.link)) : { who: SIGNER.name, what: "your answers" };
  const preview: ChainNode[] = answers.map((s) => ({
    id: `preview-${s.item}`,
    kind: "current",
    who: SIGNER.name,
    what: `preview, not saved: ${signWords(s.decision)} (as ${roleFor(s.decision)})`,
    when: "not saved",
  }));
  return [...view.trail, ...preview, { id: "now", kind: "current", ...now, when: done ? "if saved" : "waiting" }];
}

type Drafts = Record<string, string>;

/** One card per (item, question): the server decides what is shown, what may be submitted and by whom. */
function QuestionCard({
  state,
  view,
  position,
  total,
  draft,
  onDraft,
  busy,
  error,
  onSubmit,
  onClaim,
  claimBusy,
  headingRef,
  locked,
}: {
  state: ReviewState;
  view: ReviewView;
  position: number;
  total: number;
  draft: string | undefined;
  onDraft: (value: string) => void;
  busy: boolean;
  error: string | undefined;
  onSubmit: () => void;
  onClaim: (operation: "claim" | "renew" | "release" | "takeover") => void;
  claimBusy: boolean;
  headingRef: (el: HTMLHeadingElement | null) => void;
  /** The server committed a read here but its fresh state could not be read: no form until a reload. */
  locked: boolean;
}) {
  const id = useId();
  const link = state.question === "link";
  const headline = stateHeadline(state);
  const reads = submittedReads(state);
  const terminal = state.state === "resolved_agreed" || state.state === "resolved_senior";
  const next = state.next_read;
  const canAnswer = canAnswerClaim(state) && next !== null && !terminal && !locked;
  const actions = state.claim_actions;
  const evidenceOptions: [ReportAnswer, string][] = view.evidenceChoices.map((label) => {
    const display = view.programme === "crc" ? ({ adequate: "Adequate", inadequate: "Not adequate" } as Record<string, string>)[label] ?? label : label;
    return [label, display];
  });
  const title = link ? "Is it the same patient?" : view.evidenceQuestion;
  return (
    <section
      aria-labelledby={`${id}-h`}
      id={questionAnchor(state.item, state.question)}
      data-item={state.item}
      data-question={state.question}
      data-state={state.state}
      className={position > 0 ? "border-t border-rule pt-3" : undefined}
    >
      <h3 id={`${id}-h`} ref={headingRef} tabIndex={-1} className="m-0 text-body font-medium outline-offset-2">
        {position + 1}. {title}
      </h3>
      <p className="m-0 mt-1 flex flex-wrap items-center gap-2" data-testid={`status-${state.question}`}>
        <Pill tone={headline.tone}>{headline.text}</Pill>
        <span className="text-small text-muted">
          {link ? "Identity (reviewer discipline)" : view.programme === "crc" ? "Colonoscopy report (clinician discipline)" : "Programme evidence (clinician discipline)"}
          {total > 1 ? ` · question ${position + 1} of ${total}` : ""}
        </span>
      </p>

      {!terminal && (state.claim === undefined || actions === undefined) && (
        <p className="m-0 mt-2 text-small text-muted" data-testid={`claim-unavailable-${state.question}`}>
          This API does not provide review claims, so this question is not actionable.
        </p>
      )}
      {!terminal && state.claim !== undefined && actions !== undefined && (
        <div className="m-0 mt-2 flex flex-wrap items-center gap-2 text-small" data-testid={`claim-status-${state.question}`}>
          <span className={state.claim?.status === "expired" ? "text-warning" : "text-muted"}>{claimSummary(state)}</span>
          {state.claim?.status === "active" && actions.renew && <span className="text-muted">Heartbeat every 60 seconds</span>}
          {actions.claim && next && (
            <button type="button" disabled={claimBusy} onClick={() => onClaim("claim")} className="min-h-11 rounded-control bg-cta px-3 font-medium text-white hover:bg-ink disabled:opacity-60">
              {claimBusy ? "Claiming…" : `Claim ${READ_LABEL[next].toLowerCase()}`}
            </button>
          )}
          {actions.takeover && (
            <button type="button" disabled={claimBusy} onClick={() => onClaim("takeover")} className="min-h-11 rounded-control bg-cta px-3 font-medium text-white hover:bg-ink disabled:opacity-60">
              {claimBusy ? "Taking over…" : "Take over claim"}
            </button>
          )}
          {actions.renew && state.claim?.status === "active" && (
            <button type="button" disabled={claimBusy} onClick={() => onClaim("renew")} className="min-h-11 rounded-control border border-control bg-surface px-3 text-ink hover:bg-active disabled:opacity-60">
              {claimBusy ? "Renewing…" : "Renew claim"}
            </button>
          )}
          {actions.release && state.claim?.status === "active" && (
            <button type="button" disabled={claimBusy} onClick={() => onClaim("release")} className="min-h-11 rounded-control border border-control bg-surface px-3 text-ink hover:bg-active disabled:opacity-60">
              {claimBusy ? "Releasing…" : "Release claim"}
            </button>
          )}
        </div>
      )}

      {reads.length > 0 && (
        <ol aria-label={`${link ? "Identity" : "Evidence"} reads`} className="m-0 mt-2 list-none p-0 text-small">
          {reads.map((r) => (
            <li key={r.hash ?? r.read} className="py-0.5" data-read={r.read}>
              {readLine(r, state.question)}
              {r.hash && (
                <>
                  {" "}
                  <span className="font-mono text-muted" title={r.hash}>
                    {r.hash.slice(0, 12)}
                  </span>
                </>
              )}
            </li>
          ))}
        </ol>
      )}

      {state.state === "resolved_agreed" && state.final_decision && (
        <p className="m-0 mt-2" data-testid={`final-${state.question}`}>
          Both reads agree: {answerWords(state.final_decision, state.question)}. No senior form is needed.
        </p>
      )}
      {state.state === "resolved_senior" && state.final_decision && (
        <p className="m-0 mt-2" data-testid={`final-${state.question}`}>
          Senior resolution: {answerWords(state.final_decision, state.question)}.
        </p>
      )}
      {terminal && <p className="m-0 mt-1 text-small text-muted">Case data is unchanged: this is a demo overlay, not a canonical decision.</p>}
      {locked && (
        <p className="m-0 mt-2 text-small text-muted" data-testid="committed-lock">
          Your read was recorded. This form is closed until the page is reloaded so the current state can be shown.
        </p>
      )}
      {state.state === "awaiting_r3" && !canAnswer && !locked && (
        <p className="m-0 mt-2 text-small text-muted">The two reads differ. Only an authorised senior can resolve it; a third read from you is not accepted.</p>
      )}
      {state.state === "awaiting_r2" && !canAnswer && !locked && (
        <p className="m-0 mt-2 text-small text-muted">The first answer stays hidden from everyone but its author until the second read is in.</p>
      )}
      {state.state === "awaiting_r1" && !canAnswer && !locked && !actions?.claim && !actions?.takeover && !state.claim && (
        <p className="m-0 mt-2 text-small text-muted">Your credential cannot give the first read for this question.</p>
      )}

      {canAnswer && (
        <div className="mt-3">
          <fieldset className="m-0 min-w-0 border-0 p-0" aria-describedby={`${id}-help`}>
            <legend className="p-0 text-small font-medium">
              {READ_LABEL[next]}: {link ? "your answer" : "your evidence label"}
            </legend>
            <p id={`${id}-help`} className="m-0 mt-0.5 text-small text-muted">
              {next === "r3"
                ? "You are resolving a disagreement between two independent reads. Your answer is recorded as the senior resolution."
                : next === "r2"
                  ? "Answer independently. You cannot see the first answer until you submit."
                  : "Answer first. The second reader will not see your answer until they have submitted theirs."}
              {!link && view.disputed.length > 0 ? ` The readers could not settle: ${view.disputed.map((d) => d.says).join("; ")}.` : ""}
            </p>
            {link ? (
              <Chips
                name={`${id}-a`}
                options={[
                  ["same", "Same patient"],
                  ["different", "Not the same patient"],
                ]}
                value={draft as IdentityAnswer | undefined}
                onPick={onDraft}
                disabled={busy}
              />
            ) : (
              <EvidenceChoices name={`${id}-a`} programme={view.programme} options={evidenceOptions} value={draft} onPick={onDraft} disabled={busy} />
            )}
          </fieldset>
          <button
            type="button"
            onClick={onSubmit}
            aria-disabled={busy || !draft}
            className="mt-3 min-h-11 rounded-control bg-cta px-4 font-medium text-white hover:bg-ink aria-disabled:opacity-60"
          >
            {busy ? "Saving…" : SUBMIT_LABEL[next]}
          </button>
          {!draft && <p className="m-0 mt-1 text-small text-muted">Choose an answer to submit.</p>}
        </div>
      )}
      {error && (
        <p role="alert" className="m-0 mt-2 rounded-control bg-pill-negative-bg px-3 py-2 text-pill-negative-ink">
          {error}
        </p>
      )}
    </section>
  );
}

export function ReviewDesk({
  view,
  mode,
  next,
  waiting = true,
  settled,
  sharedEvidence,
  states,
}: {
  view: ReviewView;
  mode: ApiMode;
  next: NextCase;
  /** Shared programme finding and reader projections from the case page. */
  sharedEvidence?: React.ReactNode;
  /** False once the API no longer lists this case as open. The desk stays if the confirmation is still on screen. */
  waiting?: boolean;
  /** What to show instead of the desk when the case is not waiting. */
  settled?: React.ReactNode;
  /** Live only: the server's independent-review projection for the signed-in credential. */
  states?: ReviewState[];
}) {
  const live = mode === "live";
  const [draft, setDraft] = useState<Draft>({});
  const [done, setDone] = useState<Done>();
  const [drafts, setDrafts] = useState<Drafts>({});
  /* Failures are keyed by question, not by draft key: a 409 changes next_read and the draft key with it. */
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [locked, setLocked] = useState<Record<string, true>>({});
  const lockedAt = useRef<Record<string, string>>({});
  const [announcement, setAnnouncement] = useState("");
  const [busy, setBusy] = useState<string>();
  const busyRef = useRef<string | undefined>(undefined);
  const latestStates = useRef<ReviewState[]>([]);
  const draftHeads = useRef<Record<string, string | undefined>>({});
  const haltedLeases = useRef<Record<string, string>>({});
  const [notice, setNotice] = useState<string>();
  const [seen, setSeen] = useState(states);
  const [override, setOverride] = useState<ReviewState[]>();
  const resultHeading = useRef<HTMLHeadingElement>(null);
  const formRef = useRef<HTMLDivElement>(null);
  const headings = useRef<Record<string, HTMLHeadingElement | null>>({});
  const undone = useRef(false);
  const q1 = useId();
  const q2 = useId();

  /* A fresh server render replaces any optimistic copy of the states. */
  if (seen !== states) {
    setSeen(states);
    setOverride(undefined);
    /* A lock lifts only when the server's head for that question has moved past the one we wrote against. */
    setLocked((l) =>
      Object.fromEntries(Object.entries(l).filter(([qk]) => (states ?? []).some((x) => `${x.item}:${x.question}` === qk && x.head === lockedAt.current[qk]))),
    );
  }
  const current = live ? orderedStates(override ?? states ?? []) : [];
  latestStates.current = current;
  const resolved = live && allQuestionsResolved({ api_review_states: current });
  const evidenceFinal = finalDecisions({ api_review_states: current }, "evidence")[0];

  const hasLink = Boolean(view.items.link);
  const hasEvidence = Boolean(view.items.evidence);
  const evidenceOptions: [ReportAnswer, string][] = [
    ...view.evidenceChoices.map((label) => {
      const display = view.programme === "crc" ? ({ adequate: "Adequate", inadequate: "Not adequate" } as Record<string, string>)[label] ?? label : label;
      return [label, display] as [ReportAnswer, string];
    }),
    ...(view.programme === "crc" && hasLink && hasEvidence ? [["ask", "Ask the unit"] as [ReportAnswer, string]] : []),
  ];
  const pending = outcomeOf(draft, view.items);
  const outcome = done && OUTCOMES[done.outcome];

  /*
   * Closing a fixture preview removes the questions and the button, so focus would fall to <body>.
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
  }
  function pickReport(report: ReportAnswer) {
    setDraft((d) => ({ ...d, report }));
  }

  function preview() {
    if (!pending) return;
    setDone({ outcome: pending, next });
  }

  /** Applies one server-projected lease operation. No response is retried, and fresh state replaces every card. */
  async function changeClaim(state: ReviewState, operation: "claim" | "renew" | "release" | "takeover"): Promise<boolean> {
    const qk = `${state.item}:${state.question}`;
    const busyKey = `claim:${qk}`;
    if (busyRef.current) return false;
    busyRef.current = busyKey;
    setBusy(busyKey);
    setNotice(undefined);
    setErrors((e) => ({ ...e, [qk]: "" }));
    try {
      const res = await claimReviewQuestion(view.caseId, { item: state.item, question: state.question, operation, expectedClaimHead: state.claim_head ?? "" });
      if (res.states) setOverride(res.states);
      if (res.ok) {
        delete haltedLeases.current[qk];
        setDrafts((d) => Object.fromEntries(Object.entries(d).filter(([key]) => !key.startsWith(`${qk}:`))));
        if (res.committed && !res.states) setNotice("The server recorded the claim operation, but its confirmation could not be read. Reload before continuing.");
        setAnnouncement(`${state.question === "link" ? "Identity" : "Evidence"} claim ${operation} recorded. The current server state is shown.`);
        return true;
      }
      if (res.failure.status === 401 || res.failure.status === 403 || res.failure.status === 409) haltedLeases.current[qk] = state.claim_head ?? "";
      setErrors((e) => ({ ...e, [qk]: `Not saved: ${res.failure.error}` }));
      return false;
    } catch {
      haltedLeases.current[qk] = state.claim_head ?? "";
      setErrors((e) => ({ ...e, [qk]: "Not saved: the claim request failed. Check the current state before trying again." }));
      return false;
    } finally {
      busyRef.current = undefined;
      setBusy(undefined);
    }
  }

  /** Submits exactly one read. Only that question's draft is cleared; the server's answer is what the card then shows. */
  async function submitRead(state: ReviewState) {
    if (busyRef.current || !state.next_read || !canAnswerClaim(state)) return;
    const key = draftKey(state.item, state.question, state.next_read);
    const decision = drafts[key];
    if (!decision) return;
    busyRef.current = key;
    setBusy(key);
    setNotice(undefined);
    const qk = `${state.item}:${state.question}`;
    setErrors((e) => ({ ...e, [qk]: "" }));
    try {
      const res = await submitReviewRead(view.caseId, { item: state.item, question: state.question, decision, expectedHead: state.head, claimHead: state.claim_head ?? "" });
      if (res.states) setOverride(res.states);
      if (res.ok) {
        setDrafts((d) => Object.fromEntries(Object.entries(d).filter(([k]) => k !== key)));
        if (res.committed) {
          if (!res.states) {
            lockedAt.current[qk] = state.head;
            setLocked((l) => ({ ...l, [qk]: true }));
          }
          setNotice("The server recorded your read, but its confirmation could not be read. Do not submit again; reload to see the current state.");
        }
        const label = state.question === "link" ? "identity" : "evidence";
        setAnnouncement(`Your ${READ_LABEL[res.read as "r1" | "r2" | "r3"]?.toLowerCase() ?? "read"} for the ${label} question was recorded. The question status has been updated.`);
        requestAnimationFrame(() => headings.current[`${state.item}:${state.question}`]?.focus());
      } else {
        setErrors((e) => ({ ...e, [qk]: readFailureSentence(res.failure) }));
        if (res.failure.status === 409) setAnnouncement(`The ${state.question === "link" ? "identity" : "evidence"} question changed on the server. The current state is shown.`);
        if (res.failure.status === 409) setDrafts((d) => Object.fromEntries(Object.entries(d).filter(([k]) => k !== key)));
      }
    } catch {
      setErrors((e) => ({ ...e, [qk]: "Not saved: the request failed. Check the current state before trying again." }));
    } finally {
      busyRef.current = undefined;
      setBusy(undefined);
    }
  }

  const leaseSignature = current.map((s) => `${s.item}:${s.question}:${s.claim_head ?? ""}:${s.claim?.status ?? "none"}`).join("|");

  /* Clear a draft if its lease owner/head changed while the reviewer was answering. */
  useEffect(() => {
    setDrafts((existing) => Object.fromEntries(Object.entries(existing).filter(([key]) => {
      const [item, question, slot] = key.split(":");
      const state = current.find((s) => s.item === item && s.question === question && s.next_read === slot);
      return state && draftHeads.current[key] === state.claim_head;
    })));
    // The signature changes exactly when a lease head or status changes.
  }, [leaseSignature]);

  /* One non-overlapping heartbeat per active owned lease. The latest head is read at each tick. */
  useEffect(() => {
    if (!live) return;
    const timers: ReturnType<typeof setInterval>[] = [];
    for (const initial of current) {
      const qk = `${initial.item}:${initial.question}`;
      if (!initial.may_submit || initial.claim?.status !== "active" || !initial.claim_actions?.renew) continue;
      const timer = setInterval(() => {
        const latest = latestStates.current.find((s) => s.item === initial.item && s.question === initial.question);
        if (!latest || haltedLeases.current[qk] === (latest.claim_head ?? "") || !canAnswerClaim(latest) || !latest.claim_actions?.renew) {
          clearInterval(timer);
          return;
        }
        void changeClaim(latest, "renew").then((ok) => {
          if (!ok) clearInterval(timer);
        });
      }, 60_000);
      timers.push(timer);
    }
    return () => timers.forEach(clearInterval);
  }, [live, leaseSignature]);

  if (live && current.length === 0) {
    return (
      <div className="mt-6 max-w-[72ch]" role="status">
        <p className="m-0 text-muted">
          The API did not report an independent review for this case, so nothing can be submitted here. No answer is recorded from this page.
        </p>
        <ReviewerSignOut caseId={view.caseId} />
        {settled}
      </div>
    );
  }
  if (!live && !waiting && !done) return <>{settled}</>;

  const asks = hasEvidence && (!hasLink || draft.identity === "same");
  const questionCount = hasLink && hasEvidence ? 2 : 1;
  const evidenceNumber = hasLink ? 2 : 1;
  const announce = asks
    ? `Question ${evidenceNumber} of ${questionCount} is now showing: ${view.evidenceQuestion}`
    : draft.identity === "different"
      ? "Not the same patient. There is no second question."
      : "";
  const progress = resolutionProgress({ api_review_states: current });
  const answerCell = !live
    ? outcome && { ...outcome.answerCell, word: `${outcome.answerCell.word.replace(", signed", "")} (preview, not saved)` }
    : evidenceFinal
      ? resolvedCell(evidenceFinal)
      : undefined;

  return (
    <MotionConfig reducedMotion="user">
      <div className="mt-4 grid items-start gap-3 md:mt-6 xl:grid-cols-[minmax(0,1fr)_380px]">
        {/* The decision comes first in reading order and on a phone; the evidence it rests on follows. On a wide screen it sits to the right. */}
        <aside aria-labelledby="decision-h" className="min-w-0 rounded-card border border-line bg-surface p-4 md:p-6 xl:col-start-2 xl:row-start-1">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <h2 id="decision-h" className="m-0 text-section font-medium">
              Your answers
            </h2>
            {live && <ReviewerSignOut caseId={view.caseId} />}
          </div>
          <p className="m-0 mt-0.5 text-small text-muted">
            {live
              ? "Independent double review: two different credentials answer each question separately, and a disagreement goes to an authorised senior. Tokens show configured demo identities, not that separate people took part."
              : "A reconstruction: nothing here is appended or saved. Signing needs the live API."}
          </p>

          {live && (
            <div className="mt-4 flex flex-col gap-3" data-testid="question-cards">
              {current.map((state, i) => {
                const key = state.next_read ? draftKey(state.item, state.question, state.next_read) : "";
                return (
                  <QuestionCard
                    key={`${state.item}:${state.question}`}
                    state={state}
                    view={view}
                    position={i}
                    total={current.length}
                    draft={drafts[key]}
                    onDraft={(value) => {
                      draftHeads.current[key] = state.claim_head;
                      setDrafts((d) => ({ ...d, [key]: value }));
                      setErrors((e) => ({ ...e, [`${state.item}:${state.question}`]: "" }));
                    }}
                    busy={busy === key || busy === `claim:${state.item}:${state.question}`}
                    claimBusy={busy === `claim:${state.item}:${state.question}`}
                    error={errors[`${state.item}:${state.question}`] || undefined}
                    locked={Boolean(locked[`${state.item}:${state.question}`])}
                    onSubmit={() => submitRead(state)}
                    onClaim={(operation) => void changeClaim(state, operation)}
                    headingRef={(el) => {
                      headings.current[`${state.item}:${state.question}`] = el;
                    }}
                  />
                );
              })}
              <p role="status" aria-live="polite" className="sr-only" data-testid="read-announcement">
                {announcement}
              </p>
              {notice && (
                <p role="status" className="m-0 rounded-control bg-ground px-3 py-2">
                  {notice}
                </p>
              )}
              {progress && (
                <p className="m-0 text-small text-muted" data-testid="progress">
                  {progress}. {resolved ? "Every question is resolved." : "The item stays open until every question is resolved."}
                </p>
              )}
              {resolved && (
                <Link
                  href={next.href}
                  className="inline-flex min-h-11 items-center self-start rounded-control bg-cta px-4 font-medium text-white no-underline hover:bg-ink hover:text-white"
                >
                  {next.label}
                </Link>
              )}
            </div>
          )}

          {!live && !done && (
            <div ref={formRef} className="mt-4 flex flex-col gap-3">
              <div aria-live="polite" className="sr-only">
                {announce}
              </div>
              {hasLink && (
                <fieldset className="m-0 min-w-0 border-0 p-0" aria-describedby={`${q1}-who`}>
                  <legend className="p-0 text-body font-medium">1. Is it the same patient?</legend>
                  <p id={`${q1}-who`} className="m-0 mt-0.5 text-small text-muted">
                    Preview as reviewer: {SIGNER.name}.
                  </p>
                  <Chips
                    name={`${q1}-identity`}
                    options={[
                      ["same", "Same patient"],
                      ["different", "Not the same patient"],
                    ]}
                    value={draft.identity}
                    onPick={pickIdentity}
                    disabled={false}
                  />
                </fieldset>
              )}
              <AnimatePresence initial={false}>
                {asks && (
                  <motion.div key="q2" {...appear} className={hasLink ? "border-t border-rule pt-3" : undefined}>
                    <fieldset
                      className="m-0 min-w-0 border-0 p-0"
                      aria-describedby={`${q2}-who${view.programme === "crc" && hasLink ? ` ${q2}-ask` : ""}`}
                    >
                      <legend className="p-0 text-body font-medium">
                        {evidenceNumber}. {view.evidenceQuestion}
                      </legend>
                      <p id={`${q2}-who`} className="m-0 mt-0.5 text-small text-muted">
                        {view.disputed.length > 0
                          ? `The readers could not settle: ${view.disputed.map((d) => d.says).join("; ")}. One answer covers ${view.disputed.length > 1 ? "all of them" : "it"}. `
                          : "Use the exact programme evidence label. "}
                        Preview as clinician: {SIGNER.name}
                        {hasLink ? ", the same person as question 1" : ""}.
                      </p>
                      {view.programme === "crc" && hasLink && (
                        <p id={`${q2}-ask`} className="m-0 mt-0.5 text-small text-muted">
                          &ldquo;Ask the unit&rdquo; is designed, not built: it records the identity answer only and sends nothing.
                        </p>
                      )}
                      <EvidenceChoices name={`${q2}-report`} programme={view.programme} options={evidenceOptions} value={draft.report} onPick={pickReport} disabled={false} />
                    </fieldset>
                  </motion.div>
                )}
              </AnimatePresence>
              {pending ? (
                <button
                  type="button"
                  onClick={preview}
                  className="min-h-11 rounded-control bg-cta px-4 font-medium text-white hover:bg-ink"
                >
                  {`Preview: ${OUTCOMES[pending].commit}`}
                </button>
              ) : (
                <p className="m-0 text-small text-muted">
                  {draft.identity === "same" && hasEvidence
                    ? view.programme === "crc" ? `Answer the colonoscopy report question to sign.` : `Answer the evidence question to sign.`
                    : `Answer ${hasLink && hasEvidence ? "both questions" : hasLink ? "the identity question" : "the evidence question"} to sign.`}
                </p>
              )}
            </div>
          )}

          <AnimatePresence>
            {!live && done && outcome && (
              <motion.div key="result" {...appear} role="status" className="mt-4 rounded-control bg-ground px-4 py-3">
                <div className="flex items-center justify-between gap-2">
                  <h3 ref={resultHeading} tabIndex={-1} className="m-0 text-body font-normal">
                    <Pill tone={outcome.tone}>{outcome.preview}</Pill>
                  </h3>
                </div>
                <div className="mt-2">
                  <HonestyLabel kind="designed">Reconstruction: nothing was saved</HonestyLabel>
                </div>
                <p className="m-0 mt-2">
                  No event was appended and no hash exists. If this were saved against the live API: {outcome.text(view.name.split(" ")[0], view.unit, hasLink)}
                </p>
                <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
                  <Link
                    href={done.next.href}
                    className="inline-flex min-h-11 items-center rounded-control bg-cta px-4 font-medium text-white no-underline hover:bg-ink hover:text-white"
                  >
                    {done.next.label}
                  </Link>
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
          <SealedChain
            nodes={
              live
                ? [...view.trail, { id: "now", kind: "current", who: resolved ? "Case data" : "Two independent reads", what: resolved ? "no downstream effect inferred" : "still being answered", when: resolved ? "resolved" : "waiting" }]
                : previewTrail(view, draft, done)
            }
            className="mt-3"
          />
        </aside>

        <div className="flex min-w-0 flex-col gap-3 xl:col-start-1 xl:row-start-1">
          {hasLink && <IdentityBridge view={view} answer={live ? (finalDecisions({ api_review_states: current }, "link")[0] as IdentityAnswer | undefined) : draft.identity} />}
          {view.programme === "crc" ? <ReadersTable view={view} signed={answerCell} /> : sharedEvidence}
        </div>
      </div>
    </MotionConfig>
  );
}
