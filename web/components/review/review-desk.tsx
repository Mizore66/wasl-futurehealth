"use client";

import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { AnimatePresence, MotionConfig, motion } from "motion/react";
import { Check, Lock } from "lucide-react";
import { cx } from "@/lib/cx";
import { claimReviewQuestion, submitReviewRead } from "@/app/app/review/[id]/actions";
import { HONESTY_PLAIN, HonestyLabel } from "@/components/wasl/honesty-label";
import { SealedChain, type ChainNode } from "@/components/wasl/sealed-chain";
import type { ApiMode, ReviewSourceResult } from "@/lib/api/source";
import type { ReviewRead, ReviewState } from "@/lib/api/schema";
import { allQuestionsResolved, finalDecisions, READ_LABEL, resolutionProgress } from "@/lib/domain/stage";
import type { ReviewView } from "./case-view";
import {
  answerWords,
  canAnswerClaim,
  claimSummary,
  CRC_CHOICE_LABELS,
  draftKey,
  orderedStates,
  outcomeOf,
  questionAnchor,
  OUTCOMES,
  outcomeFor,
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
import { imageModelRevealed, ReviewPhotos } from "./review-photos";
import { SourceReport } from "./source-report";

export type NextCase = { href: string; label: string; last: boolean };

const CLAIM_DONE: Record<"claim" | "renew" | "release" | "takeover", string> = { claim: "claimed", renew: "renewed", release: "released", takeover: "taken over" };

/** What a screen reader hears after a lease operation: the question and the operation in the past tense. */
export function claimAnnouncement(question: ReviewState["question"], operation: keyof typeof CLAIM_DONE): string {
  return `${question === "link" ? "Identity" : "Evidence"} question ${CLAIM_DONE[operation]}. The current server state is shown.`;
}

/**
 * The note under a disagreement before the viewer can answer. An eligible senior who has not claimed yet is told to
 * claim; only a viewer the API does not let claim or take over the senior read is told a third read is not accepted.
 */
export function awaitingSeniorWords(actions: ReviewState["claim_actions"]): string {
  if (actions?.claim || actions?.takeover) return "The two reads differ. Claim the senior resolution to give the deciding read.";
  return "The two reads differ. Only an authorised senior can resolve it. A third read from you is not accepted.";
}

/** A second read reveals the first, so the page's withheld-history notice must be re-rendered from the server. */
export function readEndsConcealment(read: unknown): boolean {
  return read === "r2";
}

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
    what: `preview, not saved: ${signWords(s.decision, view.items.scoped)} (as ${roleFor(s.decision)})`,
    when: "not saved",
  }));
  return [...view.trail, ...preview, { id: "now", kind: "current", ...now, when: done ? "if saved" : "waiting" }];
}

type Drafts = Record<string, { decision?: string; note?: string }>;
type ClaimOperationResult = "success" | "busy" | "terminal" | "transient";

/** A first read stays hidden until the second read is in, from its own author too: the desk then shows only that it was given. */
export function shownReads(state: ReviewState) {
  const reads = submittedReads(state);
  return state.state === "awaiting_r2" ? reads.map((r): ReviewRead => ({ read: r.read, submitted: r.submitted })) : reads;
}

/** The locked state's title: which read is due, never the answers. */
export function dueTitle(state: ReviewState): string {
  if (state.state === "awaiting_r1") return "First read due";
  if (state.state === "awaiting_r2") return "Second read due";
  return stateHeadline(state).text;
}

const BTN_PRIMARY = "inline-flex min-h-11 items-center gap-1.5 rounded-control bg-cta px-4 font-semibold text-white hover:bg-jade-800 disabled:opacity-60 aria-disabled:opacity-60";
const BTN_QUIET = "inline-flex min-h-11 items-center gap-1.5 rounded-control border border-control bg-surface px-3 text-ink hover:bg-active disabled:opacity-60";

/** One card per (item, question): the server decides what is shown, what may be submitted and by whom. */
function QuestionCard({
  state,
  view,
  position,
  total,
  draft,
  onDraft,
  note,
  onNote,
  busy,
  error,
  onSubmit,
  onClaim,
  claimBusy,
  headingRef,
  locked,
  bound,
  context,
}: {
  state: ReviewState;
  view: ReviewView;
  /** False when this item has no context on screen: the question is shown but cannot be answered. */
  bound: boolean;
  position: number;
  total: number;
  draft: string | undefined;
  onDraft: (value: string) => void;
  note: string;
  onNote: (value: string) => void;
  busy: boolean;
  error: string | undefined;
  onSubmit: () => void;
  onClaim: (operation: "claim" | "renew" | "release" | "takeover") => void;
  claimBusy: boolean;
  headingRef: (el: HTMLHeadingElement | null) => void;
  /** The server committed a read here but its fresh state could not be read: no form until a reload. */
  locked: boolean;
  /** The item's context: the record pair (link) wraps the form; evidence context sits below it. */
  context: (form: React.ReactNode, reveal: boolean) => React.ReactNode;
}) {
  const id = useId();
  const link = state.question === "link";
  const headline = stateHeadline(state);
  const reads = shownReads(state);
  const terminal = state.state === "resolved_agreed" || state.state === "resolved_senior";
  const next = state.next_read;
  const canAnswer = bound && canAnswerClaim(state) && next !== null && !terminal && !locked;
  const subject = link
    ? `Comparing ${view.referralRegistration.split("|")[1] ?? "the referral record"} with ${view.reportRegistration.split("|")[1] ?? "the report record"}`
    : `Judging ${view.programme === "crc" ? "report" : "evidence from report"} ${view.report.id}${view.report.version ? `, version ${view.report.version}` : ""}`;
  const actions = state.claim_actions;
  const crcItem = view.crcItems?.[state.item];
  const evidenceOptions: [ReportAnswer, string][] = (crcItem?.choices ?? view.evidenceChoices).map((label) => {
    const display = view.programme === "crc" ? CRC_CHOICE_LABELS[label] ?? label : label;
    return [label, display];
  });
  const title = link ? "Is it the same patient?" : crcItem?.title ?? view.evidenceQuestion;
  const needsNote = next === "r3" && !note.trim();
  const [sourceOk, setSourceOk] = useState(false);
  const [blocked, setBlocked] = useState<string>();
  const choices = useRef<HTMLFieldSetElement>(null);
  const withheld = !link && !sourceOk;
  const scoped = !link && Boolean(view.items.scoped);
  const claims = !terminal && state.claim !== undefined && actions !== undefined;
  const open = canAnswer && (!crcItem || crcItem.choices.length > 0);
  /* Identifiers are on screen only while this viewer holds the claim, or once the question is resolved. */
  const reveal = open || terminal;

  function submit() {
    if (withheld || busy) return;
    if (!draft) {
      setBlocked("Choose an answer before you submit.");
      choices.current?.querySelector<HTMLInputElement>("input, select")?.focus();
      return;
    }
    if (needsNote) {
      setBlocked("Explain why to submit the senior resolution.");
      document.getElementById(`${id}-note`)?.focus();
      return;
    }
    setBlocked(undefined);
    onSubmit();
  }

  const lease = claims && (
    <p className="m-0 flex flex-wrap items-center gap-x-3 text-small" data-testid={`claim-status-${state.question}`}>
      <span className={state.claim?.status === "expired" ? "text-warning" : "text-muted"}>{claimSummary(state)}</span>
      {state.claim?.status === "active" && actions.renew && <span className="text-muted">Heartbeat every 60 seconds</span>}
    </p>
  );

  const form = open && next && (
    <div className="grid gap-3 p-[var(--rv-pad,16px)]">
      {!link && <SourceReport caseId={view.caseId} item={state.item} onAvailable={setSourceOk} />}
      <fieldset ref={choices} className="m-0 min-w-0 border-0 p-0" aria-describedby={`${id}-help`}>
        <legend className="p-0 font-semibold">
          {READ_LABEL[next]}: {link ? "same patient?" : "your evidence label"}
        </legend>
        <p id={`${id}-help`} className="m-0 mt-0.5 text-small text-muted">
          {next === "r3"
            ? "You are resolving a disagreement between two independent reads. Your answer is recorded as the senior resolution."
            : next === "r2"
              ? "Answer independently. You cannot see the first answer until you submit."
              : "Answer first. The second reader will not see your answer until they have submitted theirs."}
          {!link && view.disputed.length > 0 ? ` The readers could not settle: ${view.disputed.map((d) => d.says).join("; ")}.` : ""}
          {scoped && view.disputed.length > 0 ? " Your answer settles only this question. The other checks stay open." : ""}
        </p>
        {link ? (
          <Chips
            name={`${id}-a`}
            options={[
              ["same", "Same patient"],
              ["different", "Not the same patient"],
            ]}
            value={draft as IdentityAnswer | undefined}
            onPick={(v) => {
              setBlocked(undefined);
              onDraft(v);
            }}
            disabled={busy}
          />
        ) : (
          <EvidenceChoices
            name={`${id}-a`}
            programme={view.programme}
            options={evidenceOptions}
            value={draft}
            onPick={(v) => {
              setBlocked(undefined);
              onDraft(v);
            }}
            disabled={busy}
          />
        )}
      </fieldset>
      <div>
        <label htmlFor={`${id}-note`} className="block text-small font-semibold">
          {next === "r3" ? "Rationale for the senior resolution (required)" : "Rationale (optional)"}
        </label>
        <textarea
          id={`${id}-note`}
          value={note}
          onChange={(e) => {
            if (e.target.value.trim()) setBlocked(undefined);
            onNote(e.target.value);
          }}
          disabled={busy}
          rows={2}
          className="mt-1 block w-full rounded-control border border-limestone-600 bg-surface px-3 py-2 text-ink"
        />
      </div>
      {blocked && (
        <p role="alert" className="m-0 rounded-control bg-pill-warning-bg px-3 py-2 text-small font-semibold text-pill-warning-ink" data-testid={`blocked-${state.question}`}>
          {blocked}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-2">
        <button type="button" onClick={submit} aria-disabled={busy || withheld || undefined} className={BTN_PRIMARY}>
          <Check aria-hidden className="size-4" strokeWidth={2} />
          {busy ? "Saving…" : SUBMIT_LABEL[next]}
        </button>
        {actions?.release && state.claim?.status === "active" && (
          <button type="button" disabled={claimBusy} onClick={() => onClaim("release")} className={BTN_QUIET}>
            {claimBusy ? "Releasing…" : "Release claim"}
          </button>
        )}
        {actions?.renew && state.claim?.status === "active" && (
          <button type="button" disabled={claimBusy} onClick={() => onClaim("renew")} className={BTN_QUIET}>
            {claimBusy ? "Renewing…" : "Renew claim"}
          </button>
        )}
      </div>
      {lease}
    </div>
  );

  const readList = reads.length > 0 && (
    <ol aria-label={`${link ? "Identity" : "Evidence"} reads`} className="m-0 list-none p-0 text-small">
      {reads.map((r) => (
        <li key={r.read} className="py-0.5" data-read={r.read}>
          {readLine(r, state.question, scoped)}
          {r.note && <span className="block text-muted">Rationale: &ldquo;{r.note}&rdquo;</span>}
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
  );

  const ownFirst = state.state === "awaiting_r2" && submittedReads(state).some((r) => r.read === "r1" && r.decision !== undefined);
  const mayClaim = Boolean(next && (actions?.claim || actions?.takeover));

  return (
    <section
      aria-labelledby={`${id}-h`}
      id={questionAnchor(state.item, state.question)}
      data-item={state.item}
      data-question={state.question}
      data-state={state.state}
      className="min-w-0 overflow-hidden rounded-card border border-line bg-surface [--rv-pad:14px] md:[--rv-pad:clamp(17px,2vw,32px)]"
    >
      <header className="flex flex-wrap items-start justify-between gap-x-3 gap-y-2 border-b border-rule px-[var(--rv-pad)] py-3">
        <div className="min-w-0">
          <h2 id={`${id}-h`} ref={headingRef} tabIndex={-1} className="staff-display m-0 text-[clamp(22px,2.1vw,30px)] leading-tight text-jade-900 outline-offset-2">
            {total > 1 ? `${position + 1}. ` : ""}
            {title}
          </h2>
          <p className="m-0 mt-1 text-small text-muted" data-testid={`subject-${state.question}`}>
            {bound ? subject : "The case context for this item is not available, so it cannot be answered here."}
          </p>
        </div>
        <p className="m-0 flex flex-wrap items-center gap-2" data-testid={`status-${state.question}`}>
          <Pill tone={headline.tone} wrap>{headline.text}</Pill>
          <span className="text-small text-muted">
            {link ? "Identity (reviewer discipline)" : view.programme === "crc" ? `${view.pathology ? "Pathology" : "Colonoscopy"} report (clinician discipline)` : "Programme evidence (clinician discipline)"}
            {total > 1 ? ` · question ${position + 1} of ${total}` : ""}
          </span>
        </p>
      </header>

      {!terminal && (state.claim === undefined || actions === undefined) && (
        <p className="m-0 px-[var(--rv-pad)] pt-3 text-small text-muted" data-testid={`claim-unavailable-${state.question}`}>
          This API does not provide review claims, so this question is not actionable.
        </p>
      )}

      {link ? reveal && bound && context(open ? form : null, true) : null}

      {!open && (
        <div className={cx("grid justify-items-start gap-3 p-[var(--rv-pad)]", !terminal && "min-h-[180px] content-center")} data-testid={terminal ? undefined : `locked-${state.question}`}>
          {!terminal && (
            <>
              <span aria-hidden className="grid size-10 place-items-center rounded-control bg-jade-100 text-jade-800">
                {ownFirst ? <Check className="size-4" strokeWidth={2} /> : <Lock className="size-4" strokeWidth={2} />}
              </span>
              <h3 className="staff-display m-0 text-[clamp(22px,2.2vw,32px)] leading-tight text-jade-900">{dueTitle(state)}</h3>
              <p className="m-0 text-muted">
                {ownFirst
                  ? "Your first read is in. A different reviewer is required. Your answer stays hidden until then."
                  : mayClaim
                    ? link
                      ? "Claim to view the identifiers."
                      : "Claim to answer this question."
                    : headline.text}
              </p>
              {lease}
              {claims && (actions.claim || actions.takeover) && next && (
                <div className="flex flex-wrap gap-2">
                  {actions.claim && (
                    <button type="button" disabled={claimBusy} onClick={() => onClaim("claim")} className={BTN_PRIMARY}>
                      <Lock aria-hidden className="size-4" strokeWidth={2} />
                      {claimBusy ? "Claiming…" : `Claim ${READ_LABEL[next].toLowerCase()}`}
                    </button>
                  )}
                  {actions.takeover && (
                    <button type="button" disabled={claimBusy} onClick={() => onClaim("takeover")} className={BTN_PRIMARY}>
                      {claimBusy ? "Taking over…" : "Take over claim"}
                    </button>
                  )}
                </div>
              )}
              {claims && canAnswerClaim(state) && !open && (actions.renew || actions.release) && (
                <div className="flex flex-wrap gap-2">
                  {actions.renew && (
                    <button type="button" disabled={claimBusy} onClick={() => onClaim("renew")} className={BTN_QUIET}>
                      {claimBusy ? "Renewing…" : "Renew claim"}
                    </button>
                  )}
                  {actions.release && (
                    <button type="button" disabled={claimBusy} onClick={() => onClaim("release")} className={BTN_QUIET}>
                      {claimBusy ? "Releasing…" : "Release claim"}
                    </button>
                  )}
                </div>
              )}
            </>
          )}
          {readList}
          {state.state === "resolved_agreed" && state.final_decision && (
            <p className="m-0" data-testid={`final-${state.question}`}>
              Both reads agree: {answerWords(state.final_decision, state.question, scoped)}. No senior form is needed.
            </p>
          )}
          {state.state === "resolved_senior" && state.final_decision && (
            <p className="m-0" data-testid={`final-${state.question}`}>
              Senior resolution: {answerWords(state.final_decision, state.question, scoped)}.
            </p>
          )}
          {terminal && <p className="m-0 text-small text-muted">Case data is unchanged. This is a demo overlay, not a canonical decision.</p>}
          {locked && (
            <p className="m-0 text-small text-muted" data-testid="committed-lock">
              Your read was recorded. This form is closed until the page is reloaded so the current state can be shown.
            </p>
          )}
          {state.state === "awaiting_r3" && !locked && (
            <p className="m-0 text-small text-muted" data-testid={`r3-note-${state.question}`}>{awaitingSeniorWords(actions)}</p>
          )}
          {state.state === "awaiting_r2" && !locked && !ownFirst && (
            <p className="m-0 text-small text-muted">The first answer stays hidden from everyone but its author until the second read is in.</p>
          )}
          {state.state === "awaiting_r1" && !locked && !actions?.claim && !actions?.takeover && !state.claim && (
            <p className="m-0 text-small text-muted">Your credential cannot give the first read for this question.</p>
          )}
          {crcItem && crcItem.choices.length === 0 && <p className="m-0">This obligation is blocked, already signed or superseded. No generic adequacy answer can resolve it.</p>}
        </div>
      )}

      {open && reads.length > 0 && <div className="border-t border-rule px-[var(--rv-pad)] py-3">{readList}</div>}
      {!link && open && form}
      {error && (
        <p role="alert" className="m-0 mx-[var(--rv-pad)] mb-3 rounded-control bg-pill-negative-bg px-3 py-2 text-pill-negative-ink">
          {error}
        </p>
      )}
      {!link && context(null, reveal)}
    </section>
  );
}

/** Each owned lease keeps its own interval across unrelated question-state updates. */
function ClaimHeartbeat({
  state,
  latestStates,
  haltedLeases,
  onRenew,
}: {
  state: ReviewState;
  latestStates: { current: ReviewState[] };
  haltedLeases: { current: Record<string, true> };
  onRenew: (state: ReviewState, operation: "renew") => Promise<ClaimOperationResult>;
}) {
  const qk = `${state.item}:${state.question}`;
  const active = state.may_submit && state.claim?.status === "active" && Boolean(state.claim_actions?.renew);
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(() => {
      const latest = latestStates.current.find((candidate) => `${candidate.item}:${candidate.question}` === qk);
      if (!latest || haltedLeases.current[qk] || !canAnswerClaim(latest) || !latest.claim_actions?.renew) {
        clearInterval(timer);
        return;
      }
      void onRenew(latest, "renew").then((result) => {
        if (result === "terminal") clearInterval(timer);
      });
    }, 60_000);
    return () => clearInterval(timer);
  }, [active, qk, state.claim_head, latestStates, haltedLeases, onRenew]);
  return null;
}

/* Re-read active leases at expiry, including another reviewer's claim, and retry until the server state changes. */
function ClaimExpiryRefresher({ states, refreshRevision }: { states: ReviewState[]; refreshRevision: number }) {
  const router = useRouter();
  useEffect(() => {
    if (refreshRevision > 0) router.refresh();
  }, [refreshRevision, router]);
  useEffect(() => {
    const expiries = states.flatMap((state) => {
      if (state.claim?.status !== "active") return [];
      const expiresAt = Date.parse(state.claim.expires_at);
      return Number.isFinite(expiresAt) ? [expiresAt] : [];
    });
    if (expiries.length === 0) return;
    const remaining = Math.min(...expiries) - Date.now();
    let interval: ReturnType<typeof setInterval> | undefined;
    /* An active lease already past its local expiry means clock skew: retry later, never in a zero-delay refresh loop. */
    const timeout = setTimeout(() => {
      interval = setInterval(() => router.refresh(), 60_000);
      router.refresh();
    }, remaining > 0 ? remaining : 60_000);
    return () => {
      clearTimeout(timeout);
      if (interval) clearInterval(interval);
    };
  }, [states, router]);
  return null;
}

export function ReviewDesk({
  view,
  itemViews,
  mode,
  next,
  waiting = true,
  settled,
  sharedEvidence,
  states,
  fixtureSource,
}: {
  /** Fixtures only: the evidence item's source report, read on the server by the page. Omitted, the desk reads it itself. */
  fixtureSource?: ReviewSourceResult;
  view: ReviewView;
  /** One view per raised item. When given, a question is answerable only if its own item is in here; otherwise `view` serves every item. */
  itemViews?: Record<string, ReviewView>;
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
  const [busy, setBusy] = useState<Record<string, true>>({});
  const busyRef = useRef<Record<string, true>>({});
  const latestStates = useRef<ReviewState[]>([]);
  const draftHeads = useRef<Record<string, string | undefined>>({});
  const haltedLeases = useRef<Record<string, true>>({});
  const reconcileAfterOperations = useRef(false);
  const [refreshRevision, setRefreshRevision] = useState(0);
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
  const current = useMemo(() => (live ? orderedStates(override ?? states ?? []) : []), [live, override, states]);
  const resolved = live && allQuestionsResolved({ api_review_states: current });
  const viewFor = (item: string): ReviewView | undefined => (itemViews ? itemViews[item] : view);
  /** A question is bound only when its own item has a view and that view established the exact context this question needs. */
  const boundFor = (s: ReviewState): boolean => {
    const v = viewFor(s.item);
    return Boolean(v) && (!itemViews || (s.question === "link" ? v!.context.link : v!.context.evidence));
  };

  const hasLink = Boolean(view.items.link);
  const hasEvidence = Boolean(view.items.evidence);
  const evidenceOptions: [ReportAnswer, string][] = [
    ...view.evidenceChoices.map((label) => {
      const display = view.programme === "crc" ? CRC_CHOICE_LABELS[label] ?? label : label;
      return [label, display] as [ReportAnswer, string];
    }),
    ...(view.programme === "crc" && hasLink && hasEvidence ? [["ask", "Ask the unit"] as [ReportAnswer, string]] : []),
  ];
  const pending = outcomeOf(draft, view.items);
  /* An evidence label is previewed only once the report it judges is on screen; identity-only outcomes need no source. */
  const [fixtureSourceOk, setFixtureSourceOk] = useState(false);
  const signsEvidence = Boolean(view.items.evidence && draft.report && draft.report !== "ask" && pending && pending !== "notSame");
  const sourceWithheld = signsEvidence && !fixtureSourceOk;
  const outcome = done && outcomeFor(done.outcome, view.purpose);

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

  /** Merge only the question acted on so a slower whole-case response cannot regress another question's concurrent operation. */
  const applyQuestionStates = useCallback((incoming: ReviewState[], item: string, question: ReviewState["question"]) => {
    setOverride((previous) => {
      const base = previous ?? latestStates.current;
      return [
        ...base.filter((state) => state.item !== item || state.question !== question),
        ...incoming.filter((state) => state.item === item && state.question === question),
      ];
    });
  }, []);

  const applyOperationStates = useCallback((incoming: ReviewState[], item: string, question: ReviewState["question"]) => {
    if (reconcileAfterOperations.current) applyQuestionStates(incoming, item, question);
    else setOverride(incoming);
  }, [applyQuestionStates]);

  /** Applies one server-projected lease operation. No response is retried, and fresh state replaces the changed card. */
  const changeClaim = useCallback(async (state: ReviewState, operation: "claim" | "renew" | "release" | "takeover"): Promise<ClaimOperationResult> => {
    const qk = `${state.item}:${state.question}`;
    const busyKey = `claim:${qk}`;
    if (busyRef.current[qk]) return "busy";
    if (Object.keys(busyRef.current).length > 0) reconcileAfterOperations.current = true;
    busyRef.current[qk] = true;
    setBusy((current) => ({ ...current, [busyKey]: true }));
    setNotice(undefined);
    setErrors((e) => ({ ...e, [qk]: "" }));
    try {
      const res = await claimReviewQuestion(view.caseId, { item: state.item, question: state.question, operation, expectedClaimHead: state.claim_head ?? "" });
      if (res.states) applyOperationStates(res.states, state.item, state.question);
      if (res.ok) {
        delete haltedLeases.current[qk];
        if (operation === "renew") {
          const renewed = res.states?.find((candidate) => candidate.item === state.item && candidate.question === state.question);
          if (renewed) {
            for (const key of Object.keys(draftHeads.current)) {
              if (key.startsWith(`${qk}:`)) draftHeads.current[key] = renewed.claim_head;
            }
          }
        } else {
          setDrafts((d) => Object.fromEntries(Object.entries(d).filter(([key]) => !key.startsWith(`${qk}:`))));
        }
        if (res.committed && !res.states) setNotice("The server recorded the claim operation, but its confirmation could not be read. Reload before continuing.");
        setAnnouncement(claimAnnouncement(state.question, operation));
        return "success";
      }
      if (res.failure.status === 401 || res.failure.status === 403 || res.failure.status === 409) haltedLeases.current[qk] = true;
      setErrors((e) => ({ ...e, [qk]: `Not saved: ${res.failure.error}` }));
      return "terminal";
    } catch {
      setErrors((e) => ({ ...e, [qk]: "Not saved: the claim request failed. Check the current state before trying again." }));
      return "transient";
    } finally {
      delete busyRef.current[qk];
      setBusy((current) => {
        const next = { ...current };
        delete next[busyKey];
        return next;
      });
      if (Object.keys(busyRef.current).length === 0 && reconcileAfterOperations.current) {
        reconcileAfterOperations.current = false;
        setRefreshRevision((revision) => revision + 1);
      }
    }
  }, [view.caseId, applyOperationStates]);

  /** Submits exactly one read. Only that question's draft is cleared; the server's answer is what the card then shows. */
  async function submitRead(state: ReviewState) {
    if (!state.next_read || !canAnswerClaim(state)) return;
    const qk = `${state.item}:${state.question}`;
    if (busyRef.current[qk]) return;
    const key = draftKey(state.item, state.question, state.next_read);
    const { decision, note = "" } = drafts[key] ?? {};
    if (!decision || (state.next_read === "r3" && !note.trim())) return;
    if (Object.keys(busyRef.current).length > 0) reconcileAfterOperations.current = true;
    busyRef.current[qk] = true;
    setBusy((current) => ({ ...current, [key]: true }));
    setNotice(undefined);
    setErrors((e) => ({ ...e, [qk]: "" }));
    try {
      const res = await submitReviewRead(view.caseId, { item: state.item, question: state.question, decision, note, expectedHead: state.head, claimHead: state.claim_head ?? "" });
      if (res.states) applyOperationStates(res.states, state.item, state.question);
      if (res.ok) {
        setDrafts((d) => Object.fromEntries(Object.entries(d).filter(([k]) => k !== key)));
        if (res.committed) {
          if (!res.states) {
            lockedAt.current[qk] = state.head;
            setLocked((l) => ({ ...l, [qk]: true }));
          }
          setNotice("The server recorded your read, but its confirmation could not be read. Do not submit again. Reload to see the current state.");
        }
        const label = state.question === "link" ? "identity" : "evidence";
        setAnnouncement(`Your ${READ_LABEL[res.read as "r1" | "r2" | "r3"]?.toLowerCase() ?? "read"} for the ${label} question was recorded. The question status has been updated.`);
        requestAnimationFrame(() => headings.current[`${state.item}:${state.question}`]?.focus());
        /* The server-rendered history gate ("withheld while a first read is hidden") is stale once r2 is in: re-render it. */
        if (readEndsConcealment(res.read)) reconcileAfterOperations.current = true;
      } else {
        setErrors((e) => ({ ...e, [qk]: readFailureSentence(res.failure) }));
        if (res.failure.status === 409) setAnnouncement(`The ${state.question === "link" ? "identity" : "evidence"} question changed on the server. The current state is shown.`);
        if (res.failure.status === 409) setDrafts((d) => Object.fromEntries(Object.entries(d).filter(([k]) => k !== key)));
      }
    } catch {
      setErrors((e) => ({ ...e, [qk]: "Not saved: the request failed. Check the current state before trying again." }));
    } finally {
      delete busyRef.current[qk];
      setBusy((current) => {
        const next = { ...current };
        delete next[key];
        return next;
      });
      if (Object.keys(busyRef.current).length === 0 && reconcileAfterOperations.current) {
        reconcileAfterOperations.current = false;
        setRefreshRevision((revision) => revision + 1);
      }
    }
  }

  useEffect(() => {
    latestStates.current = current;
  }, [current]);

  /* Clear a draft if its lease owner/head changed while the reviewer was answering. */
  useEffect(() => {
    setDrafts((existing) => Object.fromEntries(Object.entries(existing).filter(([key]) => {
      const [item, question, slot] = key.split(":");
      const state = current.find((s) => s.item === item && s.question === question && s.next_read === slot);
      return state && draftHeads.current[key] === state.claim_head;
    })));
    // A fresh current array is produced exactly when server or lease state changes.
  }, [current]);


  if (!live && Object.keys(view.crcItems ?? {}).length > 0) {
    return <div className="mt-6" role="status"><p>CRC care needs separate evidence-bound confirmations. This fixture preview cannot sign care obligations or declare them completed. Use an authenticated live review for each current item.</p>{sharedEvidence}</div>;
  }
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
  const answerCell = outcome && { ...outcome.answerCell, word: `${outcome.answerCell.word.replace(", signed", "")} (preview, not saved)` };

  const previewControls = pending && sourceWithheld ? (
    <p className="m-0 text-small text-muted" data-testid="source-withheld">
      The evidence answer cannot be previewed until the source report it judges is shown.
    </p>
  ) : pending ? (
    <button type="button" onClick={preview} className="min-h-11 justify-self-start rounded-control bg-cta px-4 font-semibold text-white hover:bg-jade-800">
      {`Preview: ${outcomeFor(pending, view.purpose).commit}`}
    </button>
  ) : (
    <p className="m-0 text-small text-muted">
      {draft.identity === "same" && hasEvidence
        ? view.programme === "crc" ? `Answer the ${view.pathology ? "pathology" : "colonoscopy"} report question to sign.` : `Answer the evidence question to sign.`
        : `Answer ${hasLink && hasEvidence ? "both questions" : hasLink ? "the identity question" : "the evidence question"} to sign.`}
    </p>
  );

  return (
    <MotionConfig reducedMotion="user">
      {live && (
        <>
          <ClaimExpiryRefresher states={current} refreshRevision={refreshRevision} />
          {current.map((state) => (
            <ClaimHeartbeat
              key={`${state.item}:${state.question}`}
              state={state}
              latestStates={latestStates}
              haltedLeases={haltedLeases}
              onRenew={changeClaim}
            />
          ))}
        </>
      )}
      <section aria-label="Your answers" data-testid={live ? "question-cards" : undefined} className="mt-3 flex min-w-0 flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
          <p className="m-0 max-w-[80ch] text-small text-muted">
            {live
              ? "In independent double review, two different credentials answer each question separately, and a disagreement goes to an authorised senior. Tokens show configured demo identities, not that separate people took part."
              : "This is a reconstruction. Nothing here is appended or saved. Signing needs the live API."}
          </p>
          {live && <ReviewerSignOut caseId={view.caseId} />}
        </div>

        {live &&
          current.map((state, i) => {
            const key = state.next_read ? draftKey(state.item, state.question, state.next_read) : "";
            const itemView = viewFor(state.item);
            const own = { api_review_states: current.filter((s) => s.item === state.item) };
            const evidenceFinal = finalDecisions(own, "evidence")[0];
            const context = (form: React.ReactNode) =>
              !itemView || !boundFor(state) ? null : state.question === "link" ? (
                <div data-context-item={state.item} data-context-question="link" className="border-b border-rule">
                  <IdentityBridge view={itemView} answer={(finalDecisions(own, "link")[0] as IdentityAnswer | undefined) ?? (drafts[key]?.decision as IdentityAnswer | undefined)}>
                    {form}
                  </IdentityBridge>
                </div>
              ) : itemView.imageBacked || !itemView.pathology ? (
                <div data-context-item={state.item} data-context-question="evidence" className="flex min-w-0 flex-col gap-3 border-t border-rule bg-limestone-100 p-[var(--rv-pad)]">
                  {itemView.imageBacked && <ReviewPhotos view={itemView} revealModel={imageModelRevealed(current, itemView.items.evidence)} />}
                  {!itemView.pathology && <ReadersTable view={itemView} signed={evidenceFinal ? resolvedCell(evidenceFinal) : undefined} />}
                </div>
              ) : null;
            return (
              <QuestionCard
                key={`${state.item}:${state.question}`}
                state={state}
                view={itemView ?? view}
                bound={boundFor(state)}
                position={i}
                total={current.length}
                context={context}
                draft={drafts[key]?.decision}
                onDraft={(value) => {
                  draftHeads.current[key] = state.claim_head;
                  setDrafts((d) => ({ ...d, [key]: { ...d[key], decision: value } }));
                  setErrors((e) => ({ ...e, [`${state.item}:${state.question}`]: "" }));
                }}
                note={drafts[key]?.note ?? ""}
                onNote={(value) => {
                  draftHeads.current[key] = state.claim_head;
                  setDrafts((d) => ({ ...d, [key]: { ...d[key], note: value } }));
                }}
                busy={Boolean(busy[key] || busy[`claim:${state.item}:${state.question}`])}
                claimBusy={Boolean(busy[`claim:${state.item}:${state.question}`])}
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

        {live && (
          <>
            <p role="status" aria-live="polite" className="sr-only" data-testid="read-announcement">
              {announcement}
            </p>
            {notice && (
              <p role="status" className="m-0 rounded-control bg-pill-warning-bg px-3 py-2 text-pill-warning-ink">
                {notice}
              </p>
            )}
            {progress && (
              <p className="m-0 text-small text-muted" data-testid="progress">
                {progress}.{" "}
                {view.items.scoped
                  ? resolved
                    ? "This question is resolved. Histology and other checks still decide the case."
                    : "The item stays open until this question is resolved."
                  : resolved
                    ? "Every question is resolved."
                    : "The item stays open until every question is resolved."}
              </p>
            )}
            {resolved && (
              <Link
                href={next.href}
                className="inline-flex min-h-11 items-center self-start rounded-control bg-cta px-4 font-semibold text-white no-underline hover:bg-jade-800 hover:text-white"
              >
                {next.label}
              </Link>
            )}
            {view.programme !== "crc" && sharedEvidence}
          </>
        )}

        {!live && (
          <div ref={formRef} className="flex min-w-0 flex-col gap-3">
            <div aria-live="polite" className="sr-only">
              {announce}
            </div>
            {hasLink && (
              <article aria-labelledby={`${q1}-h`} className="min-w-0 overflow-hidden rounded-card border border-line bg-surface [--rv-pad:14px] md:[--rv-pad:clamp(17px,2vw,32px)]">
                <header className="border-b border-rule px-[var(--rv-pad)] py-3">
                  <h2 id={`${q1}-h`} className="staff-display m-0 text-[clamp(22px,2.1vw,30px)] leading-tight text-jade-900">
                    Is it the same patient?
                  </h2>
                </header>
                <IdentityBridge view={view} answer={draft.identity}>
                  {!done && (
                    <div className="grid gap-3 p-[var(--rv-pad)]">
                      <fieldset className="m-0 min-w-0 border-0 p-0" aria-describedby={`${q1}-who`}>
                        <legend className="p-0 font-semibold">1. Is it the same patient?</legend>
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
                      {!hasEvidence && previewControls}
                    </div>
                  )}
                </IdentityBridge>
              </article>
            )}
            {(hasEvidence || (!hasLink && !hasEvidence)) && (
              <article aria-labelledby={`${q2}-h`} className="min-w-0 overflow-hidden rounded-card border border-line bg-surface [--rv-pad:14px] md:[--rv-pad:clamp(17px,2vw,32px)]">
                <header className="border-b border-rule px-[var(--rv-pad)] py-3">
                  <h2 id={`${q2}-h`} className="staff-display m-0 text-[clamp(22px,2.1vw,30px)] leading-tight text-jade-900">
                    {view.evidenceQuestion}
                  </h2>
                </header>
                {!done && (
                  <div className="grid gap-3 p-[var(--rv-pad)]">
                    <AnimatePresence initial={false}>
                      {asks && (
                        <motion.div key="q2" {...appear} className="grid gap-3">
                          <fieldset
                            className="m-0 min-w-0 border-0 p-0"
                            aria-describedby={`${q2}-who${view.programme === "crc" && hasLink ? ` ${q2}-ask` : ""}`}
                          >
                            <legend className="p-0 font-semibold">
                              {evidenceNumber}. {view.evidenceQuestion}
                            </legend>
                            <p id={`${q2}-who`} className="m-0 mt-0.5 text-small text-muted">
                              {view.disputed.length > 0
                                ? `The readers could not settle: ${view.disputed.map((d) => d.says).join("; ")}. ${view.items.scoped ? "Your answer settles only this question. The other checks stay open." : `One answer covers ${view.disputed.length > 1 ? "all of them" : "it"}.`} `
                                : "Use the exact programme evidence label. "}
                              Preview as clinician: {SIGNER.name}
                              {hasLink ? ", the same person as question 1" : ""}.
                            </p>
                            {view.programme === "crc" && hasLink && (
                              <p id={`${q2}-ask`} className="m-0 mt-0.5 text-small text-muted">
                                &ldquo;Ask the unit&rdquo; is designed, not built. It records the identity answer only and sends nothing.
                              </p>
                            )}
                            <EvidenceChoices name={`${q2}-report`} programme={view.programme} options={evidenceOptions} value={draft.report} onPick={pickReport} disabled={false} />
                          </fieldset>
                          {view.items.evidence && <SourceReport caseId={view.caseId} item={view.items.evidence} onAvailable={setFixtureSourceOk} preloaded={fixtureSource} />}
                        </motion.div>
                      )}
                    </AnimatePresence>
                    {!asks && hasLink && draft.identity !== "different" && (
                      <p className="m-0 text-small text-muted">Answer question 1 first. This question is asked once the records are the same patient.</p>
                    )}
                    {previewControls}
                  </div>
                )}
                <div className="flex min-w-0 flex-col gap-3 border-t border-rule bg-limestone-100 p-[var(--rv-pad)]">
                  {view.imageBacked && <ReviewPhotos view={view} revealModel={Boolean(done)} />}
                  {view.programme === "crc" && !view.pathology ? <ReadersTable view={view} signed={answerCell} /> : sharedEvidence}
                </div>
              </article>
            )}
          </div>
        )}

        <AnimatePresence>
          {!live && done && outcome && (
            <motion.div key="result" {...appear} role="status" className="rounded-card border border-line bg-surface px-4 py-4 md:px-6">
              <h3 ref={resultHeading} tabIndex={-1} className="m-0 text-body font-normal">
                <Pill tone={outcome.tone}>{outcome.preview}</Pill>
              </h3>
              <div className="mt-2">
                <HonestyLabel kind="designed" className={HONESTY_PLAIN}>Reconstruction. Nothing was saved.</HonestyLabel>
              </div>
              <p className="m-0 mt-2">
                No event was appended and no hash exists. If this were saved against the live API: {outcome.text(view.name.split(" ")[0], view.unit, hasLink)}
              </p>
              <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
                <Link
                  href={done.next.href}
                  className="inline-flex min-h-11 items-center rounded-control bg-cta px-4 font-semibold text-white no-underline hover:bg-jade-800 hover:text-white"
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

        <section aria-labelledby="case-so-far-h" className="rounded-card border border-line bg-surface px-4 py-4 md:px-6">
          <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
            <h2 id="case-so-far-h" className="m-0 text-body font-semibold">
              The case so far
            </h2>
            <div aria-label="Key" className="flex flex-wrap gap-3 text-small text-muted">
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
          </div>
          <SealedChain
            nodes={
              live
                ? [...view.trail, { id: "now", kind: "current", who: resolved ? "Case data" : "Two independent reads", what: resolved ? "no downstream effect inferred" : "still being answered", when: resolved ? (view.items.scoped ? "this question resolved" : "resolved") : "waiting" }]
                : previewTrail(view, draft, done)
            }
            className="mt-3"
          />
        </section>
      </section>
    </MotionConfig>
  );
}
