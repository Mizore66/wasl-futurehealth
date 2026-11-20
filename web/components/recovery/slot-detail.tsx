"use client";

import { Check, ChevronDown, FolderClosed, FolderOpen, Lock, LockOpen, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { cancelBackfill, confirmBackfill } from "@/app/app/recovery/actions";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { providerLabel } from "@/components/worklist/model";
import { Pager } from "@/components/worklist/pager";
import { pageOfId, pagerFocusAfter, pageStatusText, paginate, type PagerControl } from "@/components/worklist/paging";
import { TAB_FOCUS, useRovingTabs } from "@/components/worklist/use-roving-tabs";
import type { RecoveryCandidate, RecoverySlot } from "@/lib/api/recovery-schema";
import { cx } from "@/lib/cx";
import { caseHref, shortId } from "@/lib/domain/ids";
import {
  CANDIDATE_PAGE,
  candidateMatches,
  candidateSets,
  exclusionReasons,
  focalCandidate,
  gapWords,
  leadWords,
  outreachState,
  procedureLabel,
  releaseReason,
  replaceRecoveryParams,
  revealIn,
  sentence,
  words,
  type CandidateSet,
  type RecoveryView,
  type SlotStatus,
} from "./candidates";

const STATUS_WORDS: Record<SlotStatus, string> = { open: "Open", filled: "Filled", stale: "Stale" };

export function SlotStatusWord({ status }: { status: SlotStatus }) {
  return (
    <span
      data-status={status}
      className={cx("whitespace-nowrap text-small font-semibold", status === "open" ? "text-jade-700" : status === "stale" ? "text-ochre-ink" : "text-muted")}
    >
      {STATUS_WORDS[status]}
    </span>
  );
}

const gutter = "px-4 lg:px-[clamp(20px,2.4vw,40px)]";
const primary =
  "inline-flex min-h-11 items-center justify-center gap-[7px] rounded-control bg-cta px-4 text-[14px] font-semibold text-limestone-50 hover:bg-jade-800 active:scale-[0.96] disabled:cursor-not-allowed disabled:opacity-60 disabled:active:scale-100";
const secondary =
  "inline-flex min-h-11 items-center justify-center gap-[7px] rounded-control border border-control bg-surface px-4 text-[14px] font-semibold text-ink hover:bg-jade-50 active:scale-[0.96] disabled:cursor-not-allowed disabled:opacity-60";
const SET_WORDS: Record<CandidateSet, string> = { eligible: "Eligible", excluded: "Excluded" };
const SETS = ["eligible", "excluded"] as const;

function Fact({ mark, term, value, note, tone, testId }: { mark?: string; term: string; value: string; note?: string; tone?: "past" | "ok"; testId?: string }) {
  return (
    <div data-mark={mark} className="min-w-0">
      <dt className="text-small font-semibold text-muted">{term}</dt>
      <dd
        data-testid={testId}
        className={cx("m-0 text-[17px] font-semibold leading-[1.3]", tone === "past" ? "text-clay-ink" : tone === "ok" ? "text-jade-800" : "text-ink")}
      >
        {value}
        {note && <span className="block text-small font-medium text-muted">{note}</span>}
      </dd>
    </div>
  );
}

/** Every check behind a disclosure, rendered only once opened: a slot can list over a thousand candidates. */
function AllChecks({ candidate }: { candidate: RecoveryCandidate }) {
  const [open, setOpen] = useState(false);
  return (
    <details className="group/checks" onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary className="flex min-h-11 w-fit cursor-pointer list-none items-center gap-[7px] text-[13px] font-semibold text-jade-700 [&::-webkit-details-marker]:hidden">
        All checks ({candidate.checks.length})
        <ChevronDown aria-hidden className="size-3.5 transition-transform group-open/checks:rotate-180" />
      </summary>
      {open && (
        <ul className="m-0 mb-2 list-disc pl-5 text-small text-ink-soft">
          {candidate.checks.map((check) => (
            <li key={check.code} data-result={check.result}>
              {sentence(words(check.code))}: {check.result} · {words(check.reason_code)}
            </li>
          ))}
        </ul>
      )}
    </details>
  );
}

function Matches({ candidate }: { candidate: RecoveryCandidate }) {
  const { matches, warnings } = candidateMatches(candidate);
  if (matches.length + warnings.length === 0) return null;
  return (
    <ul aria-label="Slot match" className="m-0 flex list-none flex-wrap gap-x-4 gap-y-1 p-0">
      {matches.map((match) => (
        <li key={match} className="flex items-center gap-1.5 text-small font-[550]">
          <Check aria-hidden className="size-4 flex-none text-jade-600" />
          {match}
        </li>
      ))}
      {warnings.map((warning) => (
        <li key={warning} className="flex items-center gap-1.5 text-small font-[550] text-ochre-ink">
          <TriangleAlert aria-hidden className="size-4 flex-none" />
          {warning}
        </li>
      ))}
    </ul>
  );
}

/** One line under a candidate: due day against the slot, then how many match checks pass and warn. */
function RowSummary({ candidate, slotDay }: { candidate: RecoveryCandidate; slotDay: number }) {
  const gap = gapWords(slotDay, candidate.deadline_day);
  const { matches, warnings } = candidateMatches(candidate);
  return (
    <span className="block text-small text-muted">
      Due day {Math.floor(candidate.deadline_day)} · <span className={gap.past ? "text-clay-ink" : undefined}>{gap.text}</span>
      {matches.length > 0 && <> · {matches.length} {matches.length === 1 ? "match" : "matches"}</>}
      {warnings.length > 0 && (
        <span className="text-ochre-ink">
          {" "}
          · {warnings.length} {warnings.length === 1 ? "warning" : "warnings"}
        </span>
      )}
    </span>
  );
}

export function SlotDetail({ slot, asOfDay, readOnly, view }: { slot: RecoverySlot; asOfDay: number; readOnly: boolean; view: RecoveryView }) {
  const router = useRouter();
  const { assigned, eligible, excluded } = candidateSets(slot);
  const focal = focalCandidate(slot);
  const [caseChoice, setCaseChoice] = useState(view.case);
  const [setChoice, setSetChoice] = useState(view.set);
  const [pageChoice, setPageChoice] = useState(view.cpage);
  const [sourceOpen, setSourceOpen] = useState(false);
  const [confirming, setConfirming] = useState<RecoveryCandidate | null>(null);
  const [outreach, setOutreach] = useState(false);
  const [requestID, setRequestID] = useState("");
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [ledgerHash, setLedgerHash] = useState("");
  const [releasing, setReleasing] = useState(false);
  const [reason, setReason] = useState("");
  const submitting = useRef(false);
  const scroller = useRef<HTMLDivElement>(null);
  const pager = useRef<HTMLDivElement>(null);
  useEffect(() => revealIn(scroller.current, "[aria-expanded=true]"), []);

  const selectable = assigned ? [assigned, ...eligible] : eligible;
  const selected = selectable.find((c) => c.case_id === caseChoice) ?? focal;
  const set: CandidateSet = setChoice ?? (eligible.length === 0 && !assigned && excluded.length > 0 ? "excluded" : "eligible");
  const rows = set === "eligible" ? eligible : excluded;
  const paging = paginate(rows, pageChoice ?? (set === "eligible" ? pageOfId(rows, selected?.case_id, CANDIDATE_PAGE) : null) ?? 1, CANDIDATE_PAGE);
  const canConfirm = slot.status === "open";
  const filledHead = assigned?.case_head;

  const chooseSet = (next: CandidateSet) => {
    setSetChoice(next);
    setPageChoice(null);
    replaceRecoveryParams({ set: next, cpage: null });
    scroller.current?.scrollTo({ top: 0 });
  };
  const tabs = useRovingTabs({ ids: SETS, selected: set, onSelect: chooseSet, prefix: "rc-cands" });

  const goPage = (to: number, via: PagerControl) => {
    const target = paginate(rows, to, CANDIDATE_PAGE);
    setPageChoice(target.page);
    replaceRecoveryParams({ cpage: target.page });
    scroller.current?.scrollTo({ top: 0 });
    const focus = pagerFocusAfter(via, target);
    if (focus) requestAnimationFrame(() => pager.current?.querySelector<HTMLElement>(`[data-focus="pager-${focus}"]`)?.focus());
  };

  const select = (candidate: RecoveryCandidate) => {
    setCaseChoice(candidate.case_id);
    replaceRecoveryParams({ case: candidate.case_id });
  };

  const openRelease = () => {
    setReleasing(true);
    setReason("");
    setRequestID(crypto.randomUUID());
    setMessage("");
    setLedgerHash("");
  };

  const release = async () => {
    const trimmed = reason.trim();
    if (!slot.backfill || !filledHead || !trimmed || submitting.current || readOnly) return;
    submitting.current = true;
    setPending(true);
    setMessage("");
    try {
      const result = await cancelBackfill(slot.slot_id, {
        case_id: slot.backfill.case_id,
        expected_slot_head: slot.head,
        expected_case_head: filledHead,
        request_id: requestID,
        reason: trimmed,
      });
      setReleasing(false);
      if (result.ok) {
        setLedgerHash(result.response.event.hash);
        setMessage("Simulated backfill cancelled; the synthetic seat is released. The confirmation remains in the ledger.");
      } else {
        setMessage(`Cancellation refused: ${result.error}`);
      }
      router.refresh();
    } catch {
      setReleasing(false);
      setMessage("Cancellation could not be completed. Reload the projection before retrying; no release was recorded in this browser.");
      router.refresh();
    } finally {
      submitting.current = false;
      setPending(false);
    }
  };

  const openConfirmation = (candidate: RecoveryCandidate) => {
    setConfirming(candidate);
    setOutreach(false);
    setRequestID(crypto.randomUUID());
    setMessage("");
  };

  const confirm = async () => {
    if (!confirming || submitting.current || readOnly) return;
    submitting.current = true;
    setPending(true);
    setMessage("");
    try {
      const result = await confirmBackfill(slot.slot_id, {
        case_id: confirming.case_id,
        expected_slot_head: slot.head,
        expected_case_head: confirming.case_head,
        request_id: requestID,
        outreach_intent: outreach,
        note: "",
      });
      if (result.ok) {
        setConfirming(null);
        setLedgerHash(result.response.event.hash);
        setMessage("Simulated backfill confirmed; no provider booking or message was made.");
        router.refresh();
      } else {
        // Never retain stale heads or optimistic success after a refusal.
        setConfirming(null);
        setMessage(`Confirmation refused: ${result.error}`);
        if (result.status === 409) router.refresh();
      }
    } catch {
      setConfirming(null);
      setMessage("Confirmation could not be completed. Reload the projection before retrying; no success was recorded in this browser.");
      router.refresh();
    } finally {
      submitting.current = false;
      setPending(false);
    }
  };

  const slotDay = Math.floor(slot.day);
  const gap = selected ? gapWords(slot.day, selected.deadline_day) : null;
  const topId = eligible[0]?.case_id;

  /** The selected candidate's decision: what matches, whether outreach is held, every check, and the action. */
  const decision = (candidate: RecoveryCandidate) => {
    const isAssigned = candidate.case_id === assigned?.case_id;
    const hold = outreachState(candidate, isAssigned ? slot.outreach : null);
    const held = hold.tone === "held";
    return (
      <div id="rc-decision" className={cx("grid gap-3 pb-4 lg:pl-[calc(clamp(20px,2.4vw,40px)+2.75rem)]", gutter)}>
        <Matches candidate={candidate} />
        <div data-testid="recovery-outreach" data-tone={hold.tone} className="rounded-control border border-line bg-surface px-3 py-2">
          <div className="flex flex-wrap items-center gap-x-5 gap-y-1">
            <p className={cx("m-0 flex items-center gap-[7px] text-[14px] font-semibold", held ? "text-ochre-ink" : "text-jade-700")}>
              {held ? <Lock aria-hidden className="size-4" /> : <LockOpen aria-hidden className="size-4" />}
              {hold.title}
            </p>
            <dl className="m-0 flex flex-wrap gap-x-5 gap-y-1 text-small">
              <div className="flex gap-1.5">
                <dt className="text-muted">Contact permission</dt>
                <dd className="m-0 font-semibold">{sentence(words(candidate.outreach.consent))}</dd>
              </div>
              <div className="flex gap-1.5">
                <dt className="text-muted">Second identifier</dt>
                <dd className="m-0 font-semibold">{sentence(words(candidate.outreach.second_identifier))}</dd>
              </div>
            </dl>
          </div>
          <p className="m-0 mt-1 text-small text-ink-soft">Outreach is decided apart from the slot assignment; no message is ever sent.</p>
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          {isAssigned ? (
            <>
              <span className="inline-flex min-h-11 items-center gap-[7px] font-semibold text-jade-800">
                <Check aria-hidden className="size-4" />
                Backfill confirmed
              </span>
              {!readOnly && filledHead && (
                <button type="button" onClick={openRelease} className={secondary}>
                  Cancel backfill
                </button>
              )}
            </>
          ) : (
            canConfirm && (
              <button
                type="button"
                disabled={readOnly}
                aria-describedby={readOnly ? "rc-read-only" : undefined}
                onClick={() => openConfirmation(candidate)}
                className={primary}
              >
                <Check aria-hidden className="size-4" />
                Confirm backfill
              </button>
            )
          )}
          <Link href={caseHref("/app/patients", candidate.case_id)} className="text-small text-jade-800 underline underline-offset-2">
            Open case
          </Link>
        </div>
        {isAssigned && <p className="m-0 text-small text-ink-soft">Simulated: no provider booking or message was made.</p>}
        <div className="-my-2">
          <AllChecks candidate={candidate} />
        </div>
      </div>
    );
  };

  const candidateRow = (candidate: RecoveryCandidate, label: React.ReactNode, testId?: string) => {
    const on = candidate.case_id === selected?.case_id;
    return (
      <li key={candidate.case_id} data-eligible="true" data-testid={testId} className={cx("border-b border-rule", on && "bg-jade-50")}>
        <button
          type="button"
          aria-expanded={on}
          aria-controls={on ? "rc-decision" : undefined}
          onClick={() => select(candidate)}
          className={cx(
            "flex min-h-[52px] w-full cursor-pointer items-center gap-3 bg-transparent py-2.5 text-left hover:bg-jade-50",
            gutter,
            on && "shadow-[inset_3px_0_var(--color-jade-600)]",
          )}
        >
          <span className="w-8 flex-none text-center text-small font-semibold tabular-nums text-jade-800">{label}</span>{" "}
          {/* The spaces keep the button's accessible name readable: flex items are not separated in it. */}
          <span className="min-w-0 flex-1">
            <span className="flex flex-wrap items-baseline gap-x-2">
              <strong className="font-semibold text-ink [overflow-wrap:anywhere]">{shortId(candidate.case_id)}</strong>{" "}
              {candidate.case_id === topId && !assigned && <span className="text-small font-semibold text-jade-700">Top candidate </span>}
              {candidate.priority && <span className="text-small font-semibold text-ochre-ink">{sentence(candidate.priority)} </span>}
            </span>{" "}
            <RowSummary candidate={candidate} slotDay={slot.day} />
          </span>
          <ChevronDown aria-hidden className={cx("size-4 flex-none text-muted transition-transform", on && "rotate-180")} />
        </button>
        {on && decision(candidate)}
      </li>
    );
  };

  return (
    <section aria-labelledby="rc-title" className="flex min-h-0 min-w-0 flex-col">
      <header className={cx("flex-none border-b border-line py-4", gutter)}>
        <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
          <div className="min-w-0">
            <h2 id="rc-title" className="staff-display m-0 text-[clamp(24px,2.2vw,32px)] font-medium leading-[1.1] text-jade-900">
              {procedureLabel(slot.procedure)} <span className="text-muted">· {slot.unit}</span>
            </h2>
            <p className="m-0 mt-1 flex flex-wrap items-baseline gap-x-2 text-small text-muted">
              <span>
                {providerLabel(slot.provider)} · Region {slot.region}
              </span>
              <SlotStatusWord status={slot.status} />
            </p>
          </div>
          <button
            type="button"
            onClick={() => setSourceOpen((open) => !open)}
            aria-controls="rc-source"
            aria-expanded={sourceOpen}
            className={cx(secondary, "min-h-10 gap-2 px-3")}
          >
            {sourceOpen ? <FolderOpen aria-hidden className="size-4 text-jade-700" /> : <FolderClosed aria-hidden className="size-4 text-jade-700" />}
            Slot source
          </button>
        </div>
        <dl className="m-0 mt-3 grid grid-cols-2 gap-x-6 gap-y-2 sm:grid-cols-4">
          <Fact mark="slot" term={slot.backfill ? "Assigned" : slot.status === "stale" ? "Stale slot" : "Slot"} value={`Day ${slotDay}`} note={leadWords(slotDay - Math.floor(asOfDay))} />
          <Fact mark="requested" term="Requested" value={selected ? `Day ${Math.floor(selected.requested_day)}` : "None"} />
          <Fact mark="due" term="Due" value={selected ? `Day ${Math.floor(selected.deadline_day)}` : "None"} />
          <Fact term="Against due" value={gap?.text ?? "Nobody to measure"} tone={gap ? (gap.past ? "past" : "ok") : undefined} testId="recovery-gap" />
        </dl>
        <p className="m-0 mt-2 text-small text-muted">
          {selected
            ? `Requested and due for ${shortId(selected.case_id)}, ${selected.case_id === assigned?.case_id ? "the assigned case" : selected.case_id === topId ? "the top candidate" : `rank ${selected.rank}`}.`
            : "No eligible candidate to measure against."}
        </p>
        {message && (
          <div role="status" className="mt-3 rounded-control border border-line bg-surface p-3 text-small">
            {message}
            {ledgerHash && <span className="ml-2 break-all">Ledger {ledgerHash}</span>}
          </div>
        )}
      </header>

      {sourceOpen && (
        <div id="rc-source" className={cx("flex-none border-b border-line bg-limestone-100 py-3", gutter)}>
          <dl className="m-0 grid gap-x-6 gap-y-2 text-small sm:grid-cols-3">
            <div className="min-w-0">
              <dt className="font-semibold text-muted">Source case</dt>
              <dd className="m-0 font-semibold">
                <Link href={caseHref("/app/patients", slot.source_case)} className="text-ink underline-offset-2 hover:underline">
                  {shortId(slot.source_case)}
                </Link>
              </dd>
            </div>
            <div className="min-w-0">
              <dt className="font-semibold text-muted">Release reason</dt>
              <dd className="m-0 font-semibold">{releaseReason(slot.release.projection_basis)}</dd>
            </div>
            <div className="min-w-0">
              <dt className="font-semibold text-muted">Source appointment</dt>
              <dd className="m-0 font-semibold [overflow-wrap:anywhere]">{shortId(slot.release.source_appointment)}</dd>
            </div>
          </dl>
          <p className="m-0 mt-2 text-small text-ink-soft">A future synthetic session, not the missed provider slot.</p>
          {slot.history.length > 0 && (
            <details className="group/hist mt-1">
              <summary className="flex min-h-11 w-fit cursor-pointer list-none items-center gap-[7px] text-[13px] font-semibold text-jade-700 [&::-webkit-details-marker]:hidden">
                Ledger history ({slot.history.length})
                <ChevronDown aria-hidden className="size-3.5 transition-transform group-open/hist:rotate-180" />
              </summary>
              <ol className="m-0 max-h-40 list-decimal overflow-y-auto pl-5 text-small" data-testid="recovery-slot-history">
                {slot.history.map((event) => (
                  <li key={event.hash} className="break-all">
                    {event.kind ? words(event.kind) : "recovery event"}
                    {event.case ? ` · ${shortId(event.case)}` : ""} · {event.at}
                    {event.reason ? ` · reason: ${event.reason}` : ""}
                  </li>
                ))}
              </ol>
            </details>
          )}
        </div>
      )}

      <div {...tabs.listProps} aria-label="Candidates" className={cx("flex flex-none gap-5 border-b border-line", gutter)}>
        {SETS.map((key) => (
          <button
            key={key}
            {...tabs.tabProps(key)}
            onClick={() => chooseSet(key)}
            className={cx(
              "-mb-px inline-flex min-h-11 cursor-pointer items-center gap-1.5 border-b-2 bg-transparent text-body",
              TAB_FOCUS,
              set === key ? "border-jade-700 font-semibold text-jade-900" : "border-transparent font-medium text-muted hover:text-jade-800",
            )}
          >
            {SET_WORDS[key]}
            <span data-testid={`candidate-count-${key}`} className="text-small tabular-nums text-muted">
              {key === "eligible" ? eligible.length + (assigned ? 1 : 0) : excluded.length}
            </span>
          </button>
        ))}
      </div>

      <div ref={scroller} id={tabs.panelId} role="tabpanel" aria-labelledby={tabs.tabId(set)} className="relative min-h-0 flex-1 overflow-y-auto overscroll-contain">
        {slot.status === "stale" && (
          <p className={cx("m-0 border-b border-line bg-ochre-wash py-3 text-small text-ochre-ink", gutter)}>
            Slot is stale{slot.stale_reason_codes.length ? `: ${slot.stale_reason_codes.map(words).join(", ")}` : ""}. It cannot be backfilled.
          </p>
        )}
        {slot.candidates.length === 0 ? (
          <p role="status" className={cx("m-0 py-4 text-muted", gutter)}>
            No waitlist candidates were generated.
          </p>
        ) : (
          <ol className="m-0 list-none p-0">
            {set === "eligible" && assigned && candidateRow(assigned, <Check aria-label="Assigned" className="mx-auto size-4" />, "recovery-slot-backfill")}
            {set === "eligible"
              ? paging.rows.map((candidate) =>
                  candidateRow(
                    candidate,
                    <>
                      <span className="sr-only">Rank </span>
                      {candidate.rank}
                    </>,
                  ),
                )
              : paging.rows.map((candidate) => (
                  <li key={candidate.case_id} data-eligible="false" className="border-b border-rule">
                    <details className="group/ex">
                      <summary
                        className={cx(
                          "flex min-h-[52px] cursor-pointer list-none items-center gap-3 py-2.5 hover:bg-limestone-100 focus-visible:outline-offset-[-3px] [&::-webkit-details-marker]:hidden",
                          gutter,
                        )}
                      >
                        <span className="w-8 flex-none" />
                        <span className="min-w-0 flex-1">
                          <span className="flex flex-wrap items-baseline gap-x-2">
                            <strong className="font-semibold text-muted [overflow-wrap:anywhere]">{shortId(candidate.case_id)}</strong>
                            <span className="text-small font-semibold text-muted">Excluded</span>
                          </span>
                          <span className="block text-small text-ink-soft" data-testid="excluded-by">
                            {exclusionReasons(candidate).join("; ")}
                          </span>
                        </span>
                        <ChevronDown aria-hidden className="size-4 flex-none text-muted transition-transform group-open/ex:rotate-180" />
                      </summary>
                      <div className={cx("pb-3 lg:pl-[calc(clamp(20px,2.4vw,40px)+2.75rem)]", gutter)}>
                        <p className="m-0 text-small text-muted">
                          Requested day {Math.floor(candidate.requested_day)} · due day {Math.floor(candidate.deadline_day)} ·{" "}
                          <Link href={caseHref("/app/patients", candidate.case_id)} className="underline underline-offset-2">
                            Open case
                          </Link>
                        </p>
                        <AllChecks candidate={candidate} />
                      </div>
                    </details>
                  </li>
                ))}
            {rows.length === 0 && (
              <li className={cx("py-4 text-muted", gutter)}>{set === "eligible" ? "Nobody else is eligible for this slot." : "Nobody is excluded."}</li>
            )}
          </ol>
        )}
        {set === "eligible" && rows.length > 0 && (
          <p className={cx("m-0 py-3 text-small text-muted", gutter)}>Ranked by urgent priority, earliest due day, requested day, then case ID.</p>
        )}
      </div>

      <div ref={pager} className="flex-none border-t border-line">
        <Pager paging={paging} label={`${SET_WORDS[set]} candidate pages`} testId="candidate-page-range" onPage={goPage} className="px-4 py-1.5" />
        {paging.pages > 1 && (
          <p role="status" aria-live="polite" aria-atomic="true" className="sr-only">
            {pageStatusText(paging)}
          </p>
        )}
        {readOnly && (
          // Only the disabled confirm button's description: no visible footnote under the board.
          <p id="rc-read-only" className="sr-only">
            Fixture mode is read-only. Confirmations are never simulated in browser state.
          </p>
        )}
      </div>

      <ConfirmDialog
        slot={slot}
        selected={confirming}
        warnings={(confirming?.checks.filter((check) => check.result === "warning") ?? []).map((warning) => words(warning.reason_code))}
        outreach={outreach}
        setOutreach={setOutreach}
        pending={pending}
        onClose={() => setConfirming(null)}
        onConfirm={confirm}
      />

      <Dialog open={releasing} onOpenChange={(open) => { if (!open && !pending) setReleasing(false); }}>
        <DialogContent className="staff-tokens max-h-[90dvh] overflow-y-auto bg-surface text-ink" showCloseButton={!pending}>
          <DialogHeader>
            <DialogTitle>Cancel simulated backfill</DialogTitle>
            <DialogDescription>Releases the synthetic seat so the case can be rebooked. A compensating ledger event references the confirmation; nothing is deleted.</DialogDescription>
          </DialogHeader>
          <label className="flex flex-col gap-1 text-small">
            <span>Reason (required)</span>
            <textarea value={reason} maxLength={200} disabled={pending} onChange={(event) => setReason(event.target.value)} className="min-h-20 rounded-control border border-control p-2" />
          </label>
          <DialogFooter>
            <button type="button" disabled={pending} onClick={() => setReleasing(false)} className={secondary}>Keep backfill</button>
            <button type="button" disabled={pending || reason.trim() === ""} onClick={release} className={primary}>
              {pending ? "Cancelling…" : "Cancel simulated backfill"}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

function ConfirmDialog({
  slot,
  selected,
  warnings,
  outreach,
  setOutreach,
  pending,
  onClose,
  onConfirm,
}: {
  slot: RecoverySlot;
  selected: RecoveryCandidate | null;
  warnings: string[];
  outreach: boolean;
  setOutreach: (value: boolean) => void;
  pending: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <Dialog open={selected !== null} onOpenChange={(open) => { if (!open && !pending) onClose(); }}>
      <DialogContent className="staff-tokens max-h-[90dvh] overflow-y-auto bg-surface text-ink" showCloseButton={!pending}>
        <DialogHeader>
          <DialogTitle>Confirm simulated backfill</DialogTitle>
          <DialogDescription>Assigns the slot. No provider reservation or patient message will be created.</DialogDescription>
        </DialogHeader>
        {selected && (
          <div className="text-small">
            <p><strong>Slot:</strong> day {slot.day} · {slot.unit}, released by {shortId(slot.source_case)}.</p>
            <p><strong>Case:</strong> {shortId(selected.case_id)}, requested day {Math.floor(selected.requested_day)}, due day {Math.floor(selected.deadline_day)}.</p>
            {warnings.length > 0 && <div><strong>Warnings</strong><ul className="list-disc pl-5">{warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul></div>}
            <label className="mt-4 flex min-h-11 items-start gap-2">
              <input type="checkbox" checked={outreach} disabled={!selected.outreach.allowed || pending} onChange={(event) => setOutreach(event.target.checked)} />
              <span>
                Record a simulated outreach intent (never sent)
                {!selected.outreach.allowed && ` — held: ${selected.outreach.reason_codes.map(words).join(", ")}`}
              </span>
            </label>
          </div>
        )}
        <DialogFooter>
          <button type="button" disabled={pending} onClick={onClose} className={secondary}>Cancel</button>
          <button type="button" disabled={pending || !selected} onClick={onConfirm} className={primary}>
            {pending ? "Confirming…" : "Confirm simulated backfill"}
          </button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
