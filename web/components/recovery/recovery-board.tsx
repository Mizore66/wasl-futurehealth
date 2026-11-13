"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useRef, useState } from "react";
import { confirmBackfill } from "@/app/app/recovery/actions";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import type { RecoveryCandidate, RecoveryDetail, RecoveryList } from "@/lib/api/recovery-schema";
import { caseHref } from "@/lib/domain/ids";
import { shortId } from "@/lib/domain/ids";

const words = (value: string) => value.replaceAll("_", " ");

export function RecoveryBoard({ list, detail, readOnly }: { list: RecoveryList; detail: RecoveryDetail | null; readOnly: boolean }) {
  const router = useRouter();
  const [selected, setSelected] = useState<RecoveryCandidate | null>(null);
  const [outreach, setOutreach] = useState(false);
  const [requestID, setRequestID] = useState("");
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [ledgerHash, setLedgerHash] = useState("");
  const submitting = useRef(false);
  const slot = detail?.slot;

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

  const warnings = selected?.checks.filter((check) => check.result === "warning") ?? [];

  return (
    <div className="mt-6 grid items-start gap-6 xl:grid-cols-[minmax(260px,0.7fr)_minmax(0,1.6fr)]">
      <section aria-labelledby="released-slots" className="min-w-0">
        <h2 id="released-slots" className="m-0 text-section font-medium">Released-capacity projections</h2>
        <p className="text-small text-muted">Synthetic future sessions derived after trusted no-shows. They are not original appointments or real cancellations.</p>
        {list.status !== "ready" ? (
          <p role="status" className="rounded-card border border-line bg-surface p-4">Recovery input is {words(list.status)}. Run the booking demo to generate it.</p>
        ) : list.slots.length === 0 ? (
          <p role="status" className="rounded-card border border-line bg-surface p-4">No simulated released capacity is available.</p>
        ) : (
          <ul className="m-0 mt-3 flex list-none flex-col gap-2 p-0">
            {list.slots.map((item) => (
              <li key={item.slot_id}>
                <Link
                  href={`/app/recovery?slot=${item.slot_id}`}
                  aria-current={slot?.slot_id === item.slot_id ? "true" : undefined}
                  className="block min-h-11 rounded-control border border-line bg-surface p-3 text-ink no-underline hover:bg-active aria-current:border-ink"
                >
                  <span className="flex flex-wrap items-center justify-between gap-2">
                    <strong>Day {item.day} · {item.unit}</strong>
                    <span className="text-small text-muted">{item.status}</span>
                  </span>
                  <span className="mt-1 block text-small text-muted">{item.programme === "crc" ? "Colonoscopy" : "Colposcopy"} · {item.region}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section aria-labelledby="candidate-list" className="min-w-0">
        {!slot ? (
          <div className="rounded-card border border-line bg-surface p-6">
            <h2 id="candidate-list" className="m-0 text-section font-medium">Select a projection</h2>
            <p className="mb-0 text-muted">Its no-show provenance, capacity capabilities and complete ranked candidate list will appear here.</p>
          </div>
        ) : (
          <>
            <div className="border-b border-line pb-4">
              <p className="m-0 text-small font-medium text-ink">Simulated released-capacity projection</p>
              <h2 id="candidate-list" className="m-0 mt-1 text-section font-medium">Day {slot.day} · {slot.unit}</h2>
              <p className="mb-0 text-small text-muted">
                Source no-show <Link className="underline underline-offset-2" href={caseHref("/app/patients", slot.source_case)}>{shortId(slot.source_case)}</Link> · {slot.release.source_appointment}. This is a future synthetic session, not the missed provider slot.
              </p>
            </div>
            <p className="text-small text-muted">Ordering: urgent priority, earliest absolute deadline, requested day, then case ID. Consent gates messaging only; it does not change slot eligibility.</p>
            {slot.candidates.length === 0 ? <p role="status">No waitlist candidates were generated.</p> : (
              <ol className="m-0 flex list-none flex-col gap-3 p-0">
                {slot.candidates.map((candidate) => (
                  <li key={candidate.case_id} className="rounded-card border border-line bg-surface p-4" data-eligible={candidate.eligible}>
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <h3 className="m-0 text-body font-medium">
                          {candidate.eligible ? `Rank ${candidate.rank} · ` : "Excluded · "}
                          <Link className="underline underline-offset-2" href={caseHref("/app/patients", candidate.case_id)}>{shortId(candidate.case_id)}</Link>
                        </h3>
                        <p className="m-0 mt-1 text-small text-muted">Requested day {candidate.requested_day} · absolute deadline day {candidate.deadline_day}{candidate.priority ? ` · ${candidate.priority}` : ""}</p>
                      </div>
                      {candidate.eligible && slot.status === "open" && !readOnly && (
                        <button type="button" onClick={() => openConfirmation(candidate)} className="min-h-11 rounded-control bg-cta px-4 font-medium text-white hover:bg-ink">Select case</button>
                      )}
                    </div>
                    {!candidate.outreach.allowed && candidate.eligible && <p className="text-small font-medium">Slot eligible; outreach not permitted.</p>}
                    <details className="mt-2">
                      <summary className="min-h-11 cursor-pointer">Checks and explanations</summary>
                      <ul className="m-0 mt-2 list-disc pl-5 text-small">
                        {candidate.checks.map((check) => <li key={check.code} data-result={check.result}>{check.code}: {check.result} — {words(check.reason_code)}</li>)}
                      </ul>
                      <p className="text-small text-muted">Constraints: {JSON.stringify(candidate.constraints)}</p>
                    </details>
                  </li>
                ))}
              </ol>
            )}
          </>
        )}
      </section>

      {message && <div role="status" className="xl:col-span-2 rounded-control border border-line bg-surface p-3">{message}{ledgerHash && <span className="ml-2 text-small break-all">Ledger {ledgerHash}</span>}</div>}
      {readOnly && <p className="xl:col-span-2 text-small text-muted">Fixture mode is read-only. Confirmations are never simulated in browser state.</p>}

      <Dialog open={selected !== null} onOpenChange={(open) => { if (!open && !pending) setSelected(null); }}>
        <DialogContent className="max-h-[90dvh] overflow-y-auto bg-surface text-ink" showCloseButton={!pending}>
          <DialogHeader>
            <DialogTitle>Confirm simulated backfill</DialogTitle>
            <DialogDescription>No provider reservation or patient message will be created.</DialogDescription>
          </DialogHeader>
          {selected && slot && (
            <div className="text-small">
              <p><strong>Source:</strong> {slot.source_case} no-show; synthetic session {slot.session_id}, day {slot.day}.</p>
              <p><strong>Target:</strong> {selected.case_id}, requested day {selected.requested_day}, absolute deadline day {selected.deadline_day}.</p>
              {warnings.length > 0 && <div><strong>Warnings</strong><ul className="list-disc pl-5">{warnings.map((warning) => <li key={warning.code}>{words(warning.reason_code)}</li>)}</ul></div>}
              <label className="mt-4 flex min-h-11 items-start gap-2">
                <input type="checkbox" checked={outreach} disabled={!selected.outreach.allowed || pending} onChange={(event) => setOutreach(event.target.checked)} />
                <span>Record a simulated outreach intent (never dispatched){!selected.outreach.allowed && ` — unavailable: ${selected.outreach.reason_codes.map(words).join(", ")}`}</span>
              </label>
            </div>
          )}
          <DialogFooter>
            <button type="button" disabled={pending} onClick={() => setSelected(null)} className="min-h-11 rounded-control border border-control px-4">Cancel</button>
            <button type="button" disabled={pending || !selected} onClick={confirm} className="min-h-11 rounded-control bg-cta px-4 font-medium text-white disabled:opacity-60">
              {pending ? "Confirming…" : "Confirm simulated backfill"}
            </button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
