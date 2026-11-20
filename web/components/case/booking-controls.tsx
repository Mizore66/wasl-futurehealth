"use client";

import Link from "next/link";
import { useId, useRef, useState } from "react";
import { recordBooking, type BookingProjection, type BookingWriteResult } from "@/app/app/patients/[id]/booking-actions";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { CONDITION_LABELS } from "@/components/worklist/referrals";
import { dayNumber, slotUnavailableWords } from "./booking-words";
import type { BookingAction, EligibleBookingSlot, ReferralCondition } from "@/lib/api/schema";

type Mode = "accept" | "choose" | "reschedule" | "cancel";

const OPEN = new Set(["open", "no_show", "inadequate", "referral_rejected"]);

function slotWords(slot: EligibleBookingSlot) {
  return `${slot.unit} · Day ${dayNumber(slot.day)} · ${slot.region}${slot.cross_region ? " · cross-region" : ""}`;
}

/**
 * Coordinator controls for synthetic seats only. The browser sends no actor or role and never updates a reservation
 * optimistically: both ordinary success and conflict handling install a freshly read server projection.
 */
export function BookingControls({
  caseId,
  initial,
  live,
  retrievalPending = false,
  onProjection,
}: {
  caseId: string;
  initial: BookingProjection;
  live: boolean;
  /** A completed-elsewhere record retrieval recorded after `initial` was read. */
  retrievalPending?: boolean;
  onProjection?: (projection: BookingProjection) => void;
}) {
  const [local, setLocal] = useState<{ base: BookingProjection; value: BookingProjection } | null>(null);
  const [mode, setMode] = useState<Mode | null>(null);
  const [selected, setSelected] = useState("");
  const [reason, setReason] = useState("");
  const [pending, setPending] = useState(false);
  const [notice, setNotice] = useState("");
  const [locked, setLocked] = useState(false);
  const reasonId = useId();
  const triggerRef = useRef<HTMLButtonElement>(null);

  // A revalidated parent projection supersedes this component's last returned projection without an effect.
  // Callers key by case ID, so selecting another patient also clears pending notices and locks.
  const current = local?.base === initial ? local.value : initial;
  const { booking, overlay } = current;
  const summary = overlay.booking;
  const active = summary?.status === "reserved";
  const candidates = booking.eligible_slots ?? [];
  // A slot that misses the receiving unit's slot-checkable acceptance conditions is never offered: the server refuses it.
  const available = candidates.filter((slot) => slot.availability === "available" && !!slot.version && slot.referral_conditions_met !== false);
  const requirements = booking.readiness?.requirements ?? [];
  const suggested = candidates.find((slot) => slot.slot_id === booking.suggested_slot_id);
  const hasCatalogue = !!booking.suggestion_version && !!booking.calendar_version && booking.eligible_slots !== undefined;
  const notReady = booking.readiness?.ready === false;
  const eligibleCase = current.case.consent === "granted" && !current.case.closed && OPEN.has(current.case.state) && !notReady;
  const ledgerReady = overlay.ledger_status === "ok";
  const writesReady = live && locked === false && ledgerReady && !pending;
  const reconciling = retrievalPending || overlay.reconciliation?.status === "pending";
  const backfill = current.case.recovery?.backfill;
  const reserveReady = writesReady && !backfill && hasCatalogue && eligibleCase && !reconciling && candidates.length > 0;
  const acceptReady = reserveReady && booking.reservable === true && suggested?.availability === "available" && !!suggested.version && suggested.referral_conditions_met !== false;
  const chooseReady = reserveReady && available.length > 0;
  const cancelReady = writesReady && active && !!summary?.reservation_id && !!summary.reservation_version;

  let disabledReason = "";
  if (!live) disabledReason = "Simulated bookings can't be saved here. This view is read-only.";
  else if (locked) disabledReason = "Reload the page before booking again.";
  else if (!ledgerReady) disabledReason = "Booking is off: this case's record failed its tamper check.";
  else if (!hasCatalogue && !active) disabledReason = "This case has no list of slots, so it can't be booked here.";
  else if (reconciling) disabledReason = "A record retrieval for a procedure done elsewhere is pending. Record its outcome before booking.";
  else if (notReady) disabledReason = `Not clinically ready: ${booking.readiness?.reason ?? "a referral or repeat disposition is unresolved"}.`;
  else if (!eligibleCase && !active) disabledReason = "Booking needs an open case that is ready to book, with the patient's contact permission.";
  else if (!active && hasCatalogue) disabledReason = slotUnavailableWords(candidates, booking.suggested_slot_id, booking.reservable);

  const open = (next: Mode) => {
    setNotice("");
    setReason("");
    setSelected(next === "accept" ? (suggested?.slot_id ?? "") : next === "cancel" ? "" : (available[0]?.slot_id ?? ""));
    setMode(next);
  };

  const install = (projection: BookingProjection | null) => {
    if (!projection) return false;
    setLocal({ base: initial, value: projection });
    onProjection?.(projection);
    return true;
  };

  const failed = (result: Extract<BookingWriteResult, { ok: false }>) => {
    if (result.status === 409) {
      if (install(result.projection)) {
        setNotice(`Not saved: ${result.error}. The latest booking is now shown.`);
      } else {
        setLocked(true);
        setNotice(`Not saved: ${result.error}. Reload the page before trying again.`);
      }
    } else {
      if (result.status >= 500) setLocked(true);
      setNotice(`Not saved: ${result.error}${result.status >= 500 ? " Reload the page before trying again." : ""}`);
    }
    setMode(null);
  };

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (!mode || pending) return;
    let action: BookingAction;
    if (mode === "cancel") {
      if (!cancelReady || !summary?.reservation_id || !summary.reservation_version) return;
      action = {
        action: "booking_cancel", reservation_id: summary.reservation_id,
        expected_reservation_version: summary.reservation_version, reason,
      };
    } else {
      const slot = candidates.find((candidate) => candidate.slot_id === selected);
      if (!slot?.version || !booking.suggestion_version || (mode === "accept" ? !acceptReady : !chooseReady)) return;
      if (mode === "accept") {
        action = { action: "booking_accept", expected_suggestion_version: booking.suggestion_version, slot_id: slot.slot_id, expected_slot_version: slot.version };
      } else if (mode === "choose") {
        action = {
          action: "booking_choose_slot", expected_suggestion_version: booking.suggestion_version,
          slot_id: slot.slot_id, expected_slot_version: slot.version, reason,
        };
      } else {
        if (!summary?.reservation_id || !summary.reservation_version) return;
        action = {
          action: "booking_reschedule", reservation_id: summary.reservation_id,
          expected_reservation_version: summary.reservation_version, expected_suggestion_version: booking.suggestion_version,
          slot_id: slot.slot_id, expected_slot_version: slot.version, reason,
        };
      }
    }
    setPending(true);
    setNotice("");
    try {
      const result = await recordBooking(caseId, overlay.head, action);
      if (!result.ok) return failed(result);
      if (!install(result.projection)) {
        setLocked(true);
        setNotice(result.warning ?? "Saved. Reload the page before doing anything else.");
      } else {
        setNotice("Simulated booking saved. No clinic or patient was contacted.");
      }
      setMode(null);
    } catch {
      setLocked(true);
      setNotice("We couldn't confirm the save. Reload the page before trying again; it won't be booked twice.");
      setMode(null);
    } finally {
      setPending(false);
    }
  };

  return (
    <div className="mt-4 border-t border-rule pt-4" data-testid="booking-controls">
      <p className="m-0 font-medium" data-testid="reservation-state">
        {active
          ? "Simulated reservation confirmed"
          : summary?.status === "cancelled"
            ? "Simulated reservation cancelled · rebook"
            : "No simulated reservation"}
      </p>
      {summary?.slot && (
        <p className="m-0 mt-1 text-muted">
          {summary.slot.unit} · Day {dayNumber(summary.slot.day)} · {summary.slot.procedure} · deadline Day {dayNumber(summary.slot.deadline)}
        </p>
      )}
      {booking.reservation_explanation === "overbooked_suggestion_not_reservable" && (
        <p className="m-0 mt-2 text-warning">The suggested session is overbooked and can&apos;t be booked. Choose another slot.</p>
      )}
      {booking.reservation_explanation === "suggestion_misses_referral_conditions" && (
        <p className="m-0 mt-2 text-warning">The suggested slot does not meet the receiving unit&apos;s acceptance conditions. Choose a slot that does.</p>
      )}
      {requirements.length > 0 && (
        <div className="mt-2" data-testid="booking-referral-conditions">
          <p className="m-0 font-medium">Receiving unit&apos;s acceptance conditions</p>
          <ul className="m-0 mt-1 list-disc pl-5 text-small">
            {requirements.map((r) => (
              <li key={r.code}>
                {CONDITION_LABELS[r.code as ReferralCondition] ?? r.code}
                <span className="text-muted">
                  {r.check !== "slot"
                    ? " · arrange before the procedure; Wasl cannot check this"
                    : r.code === "arabic_interpreter"
                      ? " · Arabic-speaking session checked against the chosen slot; interpreter not confirmed"
                      : " · checked against the chosen slot"}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
      {disabledReason && <p className="m-0 mt-2 text-small text-muted">{disabledReason}</p>}
      {backfill && (
        <p className="m-0 mt-2 text-small text-muted" data-testid="booking-recovery-backfill">
          A simulated recovery slot is already confirmed for this case (Day {backfill.day}), so ordinary reservations are disabled.{" "}
          <Link href={`/app/recovery?slot=${backfill.slot_id}`}>Open slot recovery</Link>
        </p>
      )}

      <div className="mt-3 flex flex-wrap gap-2">
        {active ? (
          <>
            <Button ref={triggerRef} type="button" variant="quiet" size="touch" disabled={!chooseReady} onClick={() => open("reschedule")}>Reschedule simulated slot</Button>
            <Button ref={triggerRef} type="button" variant="quiet" size="touch" disabled={!cancelReady} onClick={() => open("cancel")}>Cancel simulated reservation</Button>
          </>
        ) : (
          <>
            <Button ref={triggerRef} type="button" variant="cta" size="touch" disabled={!acceptReady} onClick={() => open("accept")}>Accept simulated slot</Button>
            <Button ref={triggerRef} type="button" variant="quiet" size="touch" disabled={!chooseReady} onClick={() => open("choose")}>Choose another simulated slot</Button>
          </>
        )}
      </div>
      {pending && <p role="status" className="m-0 mt-2 text-muted">Saving the simulated reservation event…</p>}
      {notice && <p role={notice.startsWith("Not saved") ? "alert" : "status"} className="m-0 mt-3 text-muted" data-testid="booking-notice">{notice}</p>}

      <Dialog open={mode !== null} onOpenChange={(open) => !open && !pending && setMode(null)}>
        <DialogContent className="max-h-[90vh] overflow-y-auto border-line bg-surface text-ink" onCloseAutoFocus={(event) => {
          event.preventDefault();
          triggerRef.current?.focus();
        }}>
          <DialogHeader>
            <DialogTitle>
              {mode === "accept" ? "Accept simulated slot" : mode === "choose" ? "Choose another simulated slot" : mode === "reschedule" ? "Reschedule simulated slot" : "Cancel simulated reservation"}
            </DialogTitle>
            <DialogDescription className="text-muted">
              {mode === "cancel"
                ? "This only releases the simulated booking. No clinic or patient is contacted and the clinical record does not change."
                : "This holds a simulated slot in Wasl only. No clinic or patient is contacted."}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={submit} className="grid gap-4">
            {mode === "accept" && suggested && <p className="m-0 rounded-control bg-ground p-3 font-medium">{slotWords(suggested)}</p>}
            {(mode === "choose" || mode === "reschedule") && (
              <fieldset className="m-0 grid gap-2 border-0 p-0">
                <legend className="mb-2 font-medium">Currently eligible simulated slots</legend>
                {available.map((slot) => (
                  <label key={slot.slot_id} className="flex min-h-11 cursor-pointer items-start gap-3 rounded-control border border-control p-3 has-checked:border-ink has-checked:bg-active">
                    <input type="radio" name="simulated-slot" value={slot.slot_id} checked={selected === slot.slot_id} onChange={() => setSelected(slot.slot_id)} required className="mt-1" />
                    <span>
                      <span className="block font-medium">{slotWords(slot)}</span>
                      <span className="block text-small text-muted">Deadline Day {slot.deadline} · {slot.within_deadline ? "inside deadline" : "past deadline"} · predicted no-show {Math.round(slot.predicted_no_show * 100)}%</span>
                    </span>
                  </label>
                ))}
              </fieldset>
            )}
            {mode !== "accept" && (
              <div>
                <label htmlFor={reasonId} className="mb-1 block font-medium">Reason (required)</label>
                <input id={reasonId} name="reason" value={reason} onChange={(event) => setReason(event.target.value)} required maxLength={200} autoComplete="off" className="min-h-11 w-full rounded-control border border-control bg-surface px-3 text-ink" />
              </div>
            )}
            <div className="flex flex-wrap justify-end gap-2">
              <Button type="button" variant="quiet" size="touch" disabled={pending} onClick={() => setMode(null)}>Back</Button>
              <Button type="submit" variant="cta" size="touch" disabled={pending || (mode !== "accept" && !reason.trim()) || ((mode === "choose" || mode === "reschedule") && !selected)}>
                {pending ? "Saving…" : mode === "cancel" ? "Release simulated booking" : "Save simulated reservation"}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
