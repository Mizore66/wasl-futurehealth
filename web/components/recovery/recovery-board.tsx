"use client";

import { Check, ChevronDown, FolderClosed, History, Lock, LockOpen, TriangleAlert, User } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState, useSyncExternalStore } from "react";
import { cancelBackfill, confirmBackfill } from "@/app/app/recovery/actions";
import { PageHeader } from "@/components/shell/page-header";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { providerLabel } from "@/components/worklist/model";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { RecoveryCandidate, RecoveryDetail, RecoveryList, RecoverySlot } from "@/lib/api/recovery-schema";
import { cx } from "@/lib/cx";
import { caseHref, shortId } from "@/lib/domain/ids";
import {
  CANDIDATE_PAGE,
  candidateMatches,
  candidatePages,
  exclusionReasons,
  focalCandidate,
  outreachState,
  procedureLabel,
  releaseReason,
  sentence,
  slotsByDay,
  words,
} from "./candidates";
import { TimeWindow } from "./time-window";

/* Disclosures start open where the context column sits beside the candidates, and closed on a phone. */
const WIDE = "(min-width: 960px)";
function useWide() {
  return useSyncExternalStore(
    (onChange) => {
      const query = window.matchMedia(WIDE);
      query.addEventListener("change", onChange);
      return () => query.removeEventListener("change", onChange);
    },
    () => window.matchMedia(WIDE).matches,
    () => true,
  );
}

const gutter = "px-[var(--g)]";
const summaryBase =
  "flex min-h-11 w-fit cursor-pointer list-none items-center gap-[7px] rounded-control text-[14px] font-semibold leading-[1.35] [&::-webkit-details-marker]:hidden";
const primary =
  "inline-flex min-h-11 items-center justify-center gap-[7px] rounded-control bg-cta px-4 text-[14px] font-semibold text-limestone-50 hover:bg-jade-800 active:scale-[0.96] disabled:cursor-not-allowed disabled:opacity-60 disabled:active:scale-100 @max-[700px]:px-[9px] @max-[700px]:text-[13px]";
const secondary =
  "inline-flex min-h-11 items-center justify-center gap-[7px] rounded-control border border-control bg-surface px-4 text-[14px] font-semibold text-ink hover:bg-jade-50 active:scale-[0.96] disabled:cursor-not-allowed disabled:opacity-60";

function Fact({ term, children }: { term: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-wrap items-baseline justify-between gap-2 border-t border-line py-[clamp(10px,1.6cqi,20px)]">
      <dt className="text-[13px] font-semibold leading-[1.4] text-muted @max-[700px]:text-[12px]">{term}</dt>
      <dd className="m-0 min-w-0 break-words text-right text-[16px] font-semibold leading-[1.4] @max-[700px]:text-[14px]">{children}</dd>
    </div>
  );
}

/** Every check behind a disclosure, rendered only once opened: a slot can list over a thousand candidates. */
function AllChecks({ candidate }: { candidate: RecoveryCandidate }) {
  const [open, setOpen] = useState(false);
  return (
    <details className="group/checks" onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary className={cx(summaryBase, "text-[13px] text-jade-700")}>
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

function CandidateId({ candidate, label, muted = false }: { candidate: RecoveryCandidate; label: string; muted?: boolean }) {
  return (
    <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
      <span className="flex min-w-0 items-center gap-2">
        <User aria-hidden className={cx("size-[18px] flex-none", muted ? "text-muted" : "text-jade-700")} />
        <strong className={cx("staff-display min-w-0 text-[clamp(20px,2.3cqi,32px)] font-medium leading-[1.1] [overflow-wrap:anywhere]", muted ? "text-muted" : "text-ink")}>
          {shortId(candidate.case_id)}
        </strong>
      </span>
      <span className="text-[13px] font-semibold leading-[1.4] text-muted @max-[700px]:ml-[26px] @max-[700px]:basis-full @max-[700px]:text-[12px]">{label}</span>
    </span>
  );
}

function Matches({ candidate }: { candidate: RecoveryCandidate }) {
  const { matches, warnings } = candidateMatches(candidate);
  if (matches.length + warnings.length === 0) return null;
  return (
    <ul aria-label="Slot match" className="col-span-full m-0 flex list-none flex-wrap gap-x-[19px] gap-y-[5px] p-0 @max-[700px]:gap-x-4">
      {matches.map((match) => (
        <li key={match} className="flex items-center gap-1.5 text-[14px] font-[550] @max-[700px]:text-[12px]">
          <Check aria-hidden className="size-4 flex-none text-jade-600" />
          {match}
        </li>
      ))}
      {warnings.map((warning) => (
        <li key={warning} className="flex items-center gap-1.5 text-[14px] font-[550] text-ochre-ink @max-[700px]:text-[12px]">
          <TriangleAlert aria-hidden className="size-4 flex-none" />
          {warning}
        </li>
      ))}
    </ul>
  );
}

export function RecoveryBoard({ list, detail, readOnly }: { list: RecoveryList; detail: RecoveryDetail | null; readOnly: boolean }) {
  const router = useRouter();
  const wide = useWide();
  const [selected, setSelected] = useState<RecoveryCandidate | null>(null);
  const [outreach, setOutreach] = useState(false);
  const [requestID, setRequestID] = useState("");
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [ledgerHash, setLedgerHash] = useState("");
  const [releasing, setReleasing] = useState(false);
  const [reason, setReason] = useState("");
  const [holdOpen, setHoldOpen] = useState<boolean | null>(null);
  const [sourceOpen, setSourceOpen] = useState<boolean | null>(null);
  const sourceRef = useRef<HTMLDetailsElement>(null);
  const submitting = useRef(false);
  const [eligibleShown, setEligibleShown] = useState(CANDIDATE_PAGE);
  const [excludedShown, setExcludedShown] = useState(CANDIDATE_PAGE);
  const slot = detail?.slot;
  const assigned = slot?.backfill?.case_id;
  // The assigned case has its own row at the top; the lists below hold everyone else.
  const others = slot ? slot.candidates.filter((candidate) => candidate.case_id !== assigned) : [];
  const pages = slot ? candidatePages(others, eligibleShown, excludedShown) : null;
  const filled = slot?.backfill ? slot.candidates.find((candidate) => candidate.case_id === slot.backfill?.case_id) : undefined;
  const filledHead = filled?.case_head;
  const focal = slot ? focalCandidate(slot) : null;

  const openRelease = () => {
    setReleasing(true);
    setReason("");
    setRequestID(crypto.randomUUID());
    setMessage("");
    setLedgerHash("");
  };

  const release = async () => {
    const trimmed = reason.trim();
    if (!slot?.backfill || !filledHead || !trimmed || submitting.current || readOnly) return;
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
    setSelected(candidate);
    setOutreach(false);
    setRequestID(crypto.randomUUID());
    setMessage("");
  };

  const confirm = async () => {
    if (!slot || !selected || submitting.current || readOnly) return;
    submitting.current = true;
    setPending(true);
    setMessage("");
    try {
      const result = await confirmBackfill(slot.slot_id, {
        case_id: selected.case_id,
        expected_slot_head: slot.head,
        expected_case_head: selected.case_head,
        request_id: requestID,
        outreach_intent: outreach,
        note: "",
      });
      if (result.ok) {
        setSelected(null);
        setLedgerHash(result.response.event.hash);
        setMessage("Simulated backfill confirmed; no provider booking or message was made.");
        router.refresh();
      } else {
        // Never retain stale heads or optimistic success after a refusal.
        setSelected(null);
        setMessage(`Confirmation refused: ${result.error}`);
        if (result.status === 409) router.refresh();
      }
    } catch {
      setSelected(null);
      setMessage("Confirmation could not be completed. Reload the projection before retrying; no success was recorded in this browser.");
      router.refresh();
    } finally {
      submitting.current = false;
      setPending(false);
    }
  };

  const toggleSource = () => {
    const next = !(sourceOpen ?? wide);
    setSourceOpen(next);
    if (next) requestAnimationFrame(() => sourceRef.current?.scrollIntoView({ block: "nearest" }));
  };

  const warnings = selected?.checks.filter((check) => check.result === "warning") ?? [];
  const canConfirm = slot?.status === "open";
  const otherSlots = slotsByDay(list.slots);

  const slotNav = otherSlots.length > 1 && (
    <nav aria-label="Released slots" className="mb-3">
      <ul className="m-0 flex list-none gap-2 overflow-x-auto p-0 pb-1">
        {otherSlots.map((item) => (
          <li key={item.slot_id} className="flex-none">
            <Link
              href={`/app/recovery?slot=${item.slot_id}`}
              aria-current={slot?.slot_id === item.slot_id ? "page" : undefined}
              className="flex min-h-11 flex-col justify-center rounded-control border border-line bg-surface px-3 py-1.5 text-small text-ink no-underline hover:bg-jade-50 aria-[current=page]:border-jade-700 aria-[current=page]:bg-jade-50"
            >
              <strong className="font-semibold">Day {item.day} · {item.unit}</strong>
              <span className="text-muted">{procedureLabel(item.procedure)} · {item.status}</span>
            </Link>
          </li>
        ))}
      </ul>
    </nav>
  );

  if (!slot) {
    return (
      <>
        <PageHeader title="Recovery" sub="Released capacity from missed appointments" aside={<HonestyLabel kind="simulated">Simulated</HonestyLabel>} />
        {slotNav}
        <p role="status" className="m-0 rounded-[8px] border border-line bg-surface p-4">
          {list.status !== "ready"
            ? `Recovery input is ${words(list.status)}. Run the booking demo to generate it.`
            : list.slots.length === 0
              ? "No simulated released capacity is available."
              : "Select a released slot to see its candidates."}
        </p>
      </>
    );
  }

  const hold = outreachState(focal, slot.outreach);
  const held = hold.tone === "held";
  const hasRequested = focal !== null;

  return (
    <>
      {slotNav}
      <article
        aria-labelledby="rc-title"
        className="@container overflow-hidden rounded-[8px] border border-line bg-surface text-ink [--g:clamp(19px,2.65cqi,42px)] @max-[700px]:[--g:16px]"
      >
        <PageHeader
          className="mb-0! rounded-none! border-0! border-b! border-line! py-[clamp(13px,1.8cqi,24px)]! @max-[700px]:px-4!"
          title={<span id="rc-title">{procedureLabel(slot.procedure)}</span>}
          titleAside={
            <button
              type="button"
              onClick={toggleSource}
              aria-label="Slot source"
              title="Slot source"
              aria-controls="rc-source"
              aria-expanded={sourceOpen ?? wide}
              className="inline-flex size-11 flex-none items-center justify-center rounded-control bg-jade-100/70 text-jade-800 hover:bg-jade-100"
            >
              <FolderClosed aria-hidden className="size-[18px]" />
            </button>
          }
          sub={`${providerLabel(slot.provider)} · ${slot.unit}`}
          aside={<HonestyLabel kind="simulated">Simulated</HonestyLabel>}
        />

        <div className="grid min-h-[clamp(210px,31svh,325px)] items-center bg-jade-50 px-[var(--g)] py-[30px] @max-[700px]:min-h-0 @max-[700px]:py-[15px]">
          <TimeWindow
            requested={hasRequested ? focal.requested_day : null}
            slotDay={slot.day}
            due={hasRequested ? focal.deadline_day : null}
            assigned={Boolean(slot.backfill)}
            stale={slot.status === "stale"}
          />
        </div>

        <div className="grid border-t border-line @min-[701px]:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]">
          <section aria-labelledby="candidate-list" className="min-w-0">
            <div className={cx("flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 py-3", gutter)}>
              <h2 id="candidate-list" className="m-0 text-[15px] font-semibold leading-[1.4]">Candidates</h2>
              {pages && (
                <p className="m-0 text-small text-muted" data-testid="candidate-count">
                  {others.length - pages.totalExcluded + (assigned ? 1 : 0)} eligible · {pages.totalExcluded} excluded
                </p>
              )}
            </div>
            {slot.status === "stale" && (
              <p className={cx("m-0 border-t border-line bg-ochre-wash py-3 text-small text-ochre-ink", gutter)}>
                Slot is stale{slot.stale_reason_codes.length ? `: ${slot.stale_reason_codes.map(words).join(", ")}` : ""}. It cannot be backfilled.
              </p>
            )}

            {slot.backfill && (
              <div data-testid="recovery-slot-backfill" className={cx("grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-2 border-t border-line bg-jade-50 py-[14px] @min-[701px]:py-6", gutter)}>
                {filled ? <CandidateId candidate={filled} label="Assigned" /> : <strong>{shortId(slot.backfill.case_id)}</strong>}
                <div className="flex flex-wrap items-center justify-end gap-2 @max-[700px]:col-span-full @max-[700px]:justify-start">
                  <span className="inline-flex min-h-11 items-center gap-[7px] font-semibold text-jade-800">
                    <Check aria-hidden className="size-4" />
                    Backfill confirmed
                  </span>
                  {!readOnly && filledHead && (
                    <button type="button" onClick={openRelease} className={secondary}>Cancel backfill</button>
                  )}
                </div>
                <p className="col-span-full m-0 text-small text-ink-soft">
                  Simulated: no provider booking or message was made.{" "}
                  <Link href={caseHref("/app/patients", slot.backfill.case_id)} className="underline underline-offset-2">Open case</Link>
                </p>
                {filled && <Matches candidate={filled} />}
              </div>
            )}

            {!pages || slot.candidates.length === 0 ? (
              <p role="status" className={cx("m-0 border-t border-line py-4 text-muted", gutter)}>No waitlist candidates were generated.</p>
            ) : (
              <>
                <ol className="m-0 list-none p-0">
                  {pages.eligible.map((candidate, index) => {
                    const top = index === 0 && !assigned;
                    const label = `Rank ${candidate.rank}`;
                    return (
                      <li
                        key={candidate.case_id}
                        data-eligible="true"
                        className={cx(
                          "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-2 border-t border-line py-[14px] @max-[700px]:gap-x-2 @max-[700px]:py-[10px]",
                          top ? "bg-jade-50 @min-[701px]:py-7" : "bg-surface",
                          gutter,
                        )}
                      >
                        <h3 className="m-0 min-w-0 font-normal">
                          <Link href={caseHref("/app/patients", candidate.case_id)} className="text-ink no-underline hover:underline">
                            <CandidateId candidate={candidate} label={label} />
                          </Link>
                        </h3>
                        {canConfirm && (
                          top ? (
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
                          ) : (
                            <button
                              type="button"
                              disabled={readOnly}
                              aria-describedby={readOnly ? "rc-read-only" : undefined}
                              aria-label={`Choose ${shortId(candidate.case_id)}`}
                              onClick={() => openConfirmation(candidate)}
                              className={secondary}
                            >
                              Choose
                            </button>
                          )
                        )}
                        <Matches candidate={candidate} />
                        {!top && (
                          <p className="col-span-full m-0 text-small text-muted">
                            Requested day {Math.floor(candidate.requested_day)} · due day {Math.floor(candidate.deadline_day)}
                            {candidate.priority ? ` · ${candidate.priority}` : ""}
                          </p>
                        )}
                        {top && candidate.priority && <p className="col-span-full m-0 text-small font-semibold text-ochre-ink">{sentence(candidate.priority)}</p>}
                        <div className="col-span-full -my-1">
                          <AllChecks candidate={candidate} />
                        </div>
                      </li>
                    );
                  })}
                  {pages.excluded.map((candidate) => (
                    <li key={candidate.case_id} data-eligible="false" className="border-t border-line bg-surface">
                      <details className="group/ex">
                        <summary
                          className={cx(
                            "grid min-h-11 cursor-pointer list-none grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-2 py-[14px] hover:bg-limestone-100 focus-visible:outline-offset-[-3px] @max-[700px]:gap-x-2 @max-[700px]:py-[10px] [&::-webkit-details-marker]:hidden",
                            gutter,
                          )}
                        >
                          <CandidateId candidate={candidate} label="Excluded" muted />
                          <ChevronDown aria-hidden className="size-4 text-muted transition-transform group-open/ex:rotate-180" />
                        </summary>
                        <div className={cx("pb-[15px]", gutter)}>
                          <p className="m-0 text-[14px] text-muted" data-testid="excluded-by">
                            {exclusionReasons(candidate).join("; ")}
                          </p>
                          <p className="m-0 mt-1 text-small text-muted">
                            Requested day {Math.floor(candidate.requested_day)} · due day {Math.floor(candidate.deadline_day)} ·{" "}
                            <Link href={caseHref("/app/patients", candidate.case_id)} className="underline underline-offset-2">Open case</Link>
                          </p>
                          <AllChecks candidate={candidate} />
                        </div>
                      </details>
                    </li>
                  ))}
                </ol>
                {(pages.hiddenEligible > 0 || pages.hiddenExcluded > 0) && (
                  <div className={cx("flex flex-wrap gap-3 border-t border-line py-3", gutter)}>
                    {pages.hiddenEligible > 0 && (
                      <button type="button" onClick={() => setEligibleShown((n) => n + CANDIDATE_PAGE)} className={secondary}>
                        Show {Math.min(CANDIDATE_PAGE, pages.hiddenEligible)} more eligible ({pages.hiddenEligible} not shown)
                      </button>
                    )}
                    {pages.hiddenExcluded > 0 && (
                      <button type="button" onClick={() => setExcludedShown((n) => n + CANDIDATE_PAGE)} className={secondary}>
                        Show {Math.min(CANDIDATE_PAGE, pages.hiddenExcluded)} more excluded ({pages.hiddenExcluded} not shown)
                      </button>
                    )}
                  </div>
                )}
                <p className={cx("m-0 border-t border-line py-3 text-small text-muted", gutter)}>
                  Ranked by urgent priority, earliest due day, requested day, then case ID.
                </p>
              </>
            )}
          </section>

          <div className="flex min-w-0 flex-col border-t border-line bg-limestone-100 @min-[701px]:border-t-0 @min-[701px]:border-l">
            <details
              open={holdOpen ?? wide}
              onToggle={(event) => setHoldOpen(event.currentTarget.open)}
              className={cx("group/hold min-w-0 pt-1.5 pb-3 @max-[700px]:py-[3px] @min-[701px]:flex-1", gutter)}
              data-testid="recovery-outreach"
              data-tone={hold.tone}
            >
              <summary className={cx(summaryBase, held ? "text-ochre-ink" : "text-jade-700")}>
                {held ? <Lock aria-hidden className="size-4" /> : <LockOpen aria-hidden className="size-4" />}
                {hold.title}
                <ChevronDown aria-hidden className="size-3.5 transition-transform group-open/hold:rotate-180" />
              </summary>
              {focal ? (
                <>
                  <dl className="m-0">
                    <Fact term="Contact permission">{sentence(words(focal.outreach.consent))}</Fact>
                    <Fact term="Second identifier">{sentence(words(focal.outreach.second_identifier))}</Fact>
                  </dl>
                  <p className="m-0 border-t border-line pt-3 text-small text-ink-soft">
                    {assigned ? "Assigned to" : "For"} {shortId(focal.case_id)}. Outreach is decided apart from the slot assignment; no message is ever sent.
                  </p>
                </>
              ) : (
                <p className="m-0 border-t border-line pt-3 text-small text-muted">No candidate to contact.</p>
              )}
            </details>
            <details
              id="rc-source"
              ref={sourceRef}
              open={sourceOpen ?? wide}
              onToggle={(event) => setSourceOpen(event.currentTarget.open)}
              className={cx("group/src min-w-0 border-t border-line bg-surface pt-1.5 pb-3 @max-[700px]:py-[3px] @min-[701px]:flex-1", gutter)}
            >
              <summary className={cx(summaryBase, "text-jade-700")}>
                <FolderClosed aria-hidden className="size-4" />
                Slot source
                <ChevronDown aria-hidden className="size-3.5 transition-transform group-open/src:rotate-180" />
              </summary>
              <dl className="m-0">
                <Fact term="Source case">
                  <Link href={caseHref("/app/patients", slot.source_case)} className="text-ink underline-offset-2 hover:underline">{shortId(slot.source_case)}</Link>
                </Fact>
                <Fact term="Release reason">{releaseReason(slot.release.projection_basis)}</Fact>
                <Fact term="Source appointment">{shortId(slot.release.source_appointment)}</Fact>
              </dl>
              <p className="m-0 border-t border-line pt-3 text-small text-ink-soft">A future synthetic session, not the missed provider slot.</p>
            </details>
            {slot.history.length > 0 && (
              <details className={cx("group/hist min-w-0 border-t border-line bg-surface pt-1.5 pb-3", gutter)}>
                <summary className={cx(summaryBase, "text-jade-700")}>
                  <History aria-hidden className="size-4" />
                  Ledger history ({slot.history.length})
                  <ChevronDown aria-hidden className="size-3.5 transition-transform group-open/hist:rotate-180" />
                </summary>
                <ol className="m-0 list-decimal pl-5 text-small" data-testid="recovery-slot-history">
                  {slot.history.map((event) => (
                    <li key={event.hash} className="break-all">
                      {event.kind ? words(event.kind) : "recovery event"}{event.case ? ` · ${shortId(event.case)}` : ""} · {event.at}{event.reason ? ` · reason: ${event.reason}` : ""}
                    </li>
                  ))}
                </ol>
              </details>
            )}
          </div>
        </div>
      </article>

      {message && (
        <div role="status" className="mt-3 rounded-control border border-line bg-surface p-3">
          {message}
          {ledgerHash && <span className="ml-2 break-all text-small">Ledger {ledgerHash}</span>}
        </div>
      )}
      {readOnly && (
        <p id="rc-read-only" className="mb-0 mt-3 text-small text-muted">
          Fixture mode is read-only. Confirmations are never simulated in browser state.
        </p>
      )}

      <ConfirmDialog
        slot={slot}
        selected={selected}
        warnings={warnings.map((warning) => words(warning.reason_code))}
        outreach={outreach}
        setOutreach={setOutreach}
        pending={pending}
        onClose={() => setSelected(null)}
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
    </>
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
