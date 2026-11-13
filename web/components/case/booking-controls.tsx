"use client";

import { useId, useRef, useState } from "react";
import { recordBooking, type BookingProjection, type BookingWriteResult } from "@/app/app/patients/[id]/booking-actions";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import type { BookingAction, EligibleBookingSlot } from "@/lib/api/schema";

type Mode = "accept" | "choose" | "reschedule" | "cancel";

const OPEN = new Set(["open", "no_show", "inadequate", "referral_rejected"]);

function slotWords(slot: EligibleBookingSlot) {
  return `${slot.unit} · Day ${slot.day} · ${slot.region}${slot.cross_region ? " · cross-region" : ""}`;
}

/**
 * Coordinator controls for synthetic seats only. The browser sends no actor or role and never updates a reservation
 * optimistically: both ordinary success and conflict handling install a freshly read server projection.
 */
export function BookingControls({
  caseId,
  initial,
  live,
  onProjection,
}: {
  caseId: string;
  initial: BookingProjection;
  live: boolean;
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
  const available = candidates.filter((slot) => slot.availability === "available" && !!slot.version);
  const suggested = candidates.find((slot) => slot.slot_id === booking.suggested_slot_id);
  const hasCatalogue = !!booking.suggestion_version && !!booking.calendar_version && booking.eligible_slots !== undefined;
  const eligibleCase = current.case.consent === "granted" && !current.case.closed && OPEN.has(current.case.state);
  const ledgerReady = overlay.ledger_status === "ok";
  const writesReady = live && locked === false && ledgerReady && !pending;
  const reserveReady = writesReady && hasCatalogue && eligibleCase && candidates.length > 0;
  const acceptReady = reserveReady && booking.reservable === true && suggested?.availability === "available" && !!suggested.version;
  const chooseReady = reserveReady && available.length > 0;
  const cancelReady = writesReady && active && !!summary?.reservation_id && !!summary.reservation_version;

  let disabledReason = "";
  if (!live) disabledReason = "Fixture mode cannot save simulated reservations.";
  else if (locked) disabledReason = "Reload this case before another booking action.";
  else if (!ledgerReady) disabledReason = "The case ledger is invalid, so booking writes are disabled.";
  else if (!hasCatalogue && !active) disabledReason = "This older booking document has no versioned slot catalogue. Regenerate it to enable reservations.";
  else if (!eligibleCase && !active) disabledReason = "A simulated reservation needs an open booking-eligible case with canonical contact permission granted.";

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
        setNotice(`Not saved: ${result.error}. The latest server state is now shown.`);
      } else {
        setLocked(true);
        setNotice(`Not saved: ${result.error}. The latest state could not be read; reload before another attempt.`);
      }
    } else {
      if (result.status >= 500) setLocked(true);
      setNotice(`Not saved: ${result.error}${result.status >= 500 ? " Reload before another attempt." : ""}`);
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
        setNotice(result.warning ?? "The event was saved, but its state could not be read. Reload before another action.");
      } else {
        setNotice("Saved to the simulated reservation ledger. No provider system or patient was contacted.");
      }
      setMode(null);
    } catch {
      setLocked(true);
      setNotice("The save result is uncertain. Reload this case before retrying; the previous case head prevents a duplicate append.");
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
          {summary.slot.unit} · Day {summary.slot.day} · {summary.slot.procedure} · deadline Day {summary.slot.deadline}
        </p>
      )}
      {booking.reservation_explanation === "overbooked_suggestion_not_reservable" && (
        <p className="m-0 mt-2 text-warning">The suggested session is overbooked, so that suggestion is not a reservable seat. A different regular eligible slot may be chosen.</p>
      )}
      {disabledReason && <p className="m-0 mt-2 text-small text-muted">{disabledReason}</p>}

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
                ? "This only releases the synthetic reservation. It does not contact a provider or patient and does not change the clinical record."
                : "This reserves only a synthetic seat in Wasl. No provider scheduling system or patient is contacted."}
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
                {pending ? "Saving…" : mode === "cancel" ? "Release synthetic reservation" : "Save simulated reservation"}
              </Button>
            </div>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
