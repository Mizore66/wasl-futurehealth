"use client";

import { useCallback, useState } from "react";
import {
  PatientJourneyResponse,
  PatientMutationResponse,
  type PatientJourney,
  type PatientMutation,
  type PatientMutationData,
  type PatientMutationKind,
} from "@/lib/api/patient-schema";

export type PatientIntention = {
  [K in PatientMutationKind]: { kind: K; data: PatientMutationData<K> };
}[PatientMutationKind];
export type PatientSaveStatus = "saving" | "saved" | "failed" | "conflict" | "invalid" | "expired" | null;

function reflects(journey: PatientJourney, intention: PatientIntention): boolean {
  switch (intention.kind) {
    case "teach_back_completed":
      return journey.teach_back.status === "completed";
    case "contact_permission_changed":
      return journey.contact_permission.status === intention.data.permission;
    case "slot_selected":
      return journey.slot_selection.status === "selected" && journey.slot_selection.slot_id === intention.data.slot_id;
    case "preparation_changed":
      return journey.preparation.items.some((item) => item.item_id === intention.data.item_id && item.completed === intention.data.completed);
  }
}

/** Server-confirmed patient state. A proposed click is never folded into the UI before the API acknowledges it. */
export function usePatientJourney(initial: PatientJourney) {
  const [journey, setJourney] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [status, setStatus] = useState<PatientSaveStatus>(null);
  const [pending, setPending] = useState<PatientMutation | null>(null);
  const [mustRefetch, setMustRefetch] = useState(false);

  const refresh = useCallback(async (): Promise<PatientJourney | null> => {
    try {
      const res = await fetch("/patient/journey", { cache: "no-store" });
      if (res.status === 401 || res.status === 410) {
        setStatus(res.status === 410 ? "expired" : "invalid");
        return null;
      }
      if (!res.ok) {
        setStatus("failed");
        return null;
      }
      const parsed = PatientJourneyResponse.safeParse(await res.json());
      if (!parsed.success) {
        setStatus("failed");
        return null;
      }
      setJourney(parsed.data.journey);
      return parsed.data.journey;
    } catch {
      setStatus("failed");
      return null;
    }
  }, []);

  const send = useCallback(
    async (request: PatientMutation): Promise<PatientJourney | null> => {
      const intention = { kind: request.kind, data: request.data } as PatientIntention;
      setBusy(true);
      setStatus("saving");
      try {
        const res = await fetch("/patient/journey", {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify(request),
          cache: "no-store",
        });
        if (res.status === 200 || res.status === 201) {
          const parsed = PatientMutationResponse.safeParse(await res.json().catch(() => undefined));
          if (parsed.success) {
            setJourney(parsed.data.journey);
            setPending(null);
            setStatus("saved");
            return parsed.data.journey;
          }
          // The append may already be durable. Read server truth before allowing
          // this intention to be shown as saved or retried.
          setMustRefetch(true);
          const current = await refresh();
          if (current) setMustRefetch(false);
          if (current && reflects(current, intention)) {
            setPending(null);
            setStatus("saved");
            return current;
          }
          setPending(request);
          setStatus("failed");
          return null;
        }
        if (res.status === 409) {
          await refresh();
          setPending(null);
          setStatus("conflict");
          return null;
        }
        if (res.status === 401 || res.status === 410) {
          setPending(null);
          setStatus(res.status === 410 ? "expired" : "invalid");
          return null;
        }
        if (res.status === 400) {
          setPending(null);
          setStatus("failed");
          return null;
        }
        setPending(request);
        setStatus("failed");
        return null;
      } catch {
        // Preserve exactly this request for a deliberate network retry. A
        // refetch may reveal that the original append had in fact succeeded.
        setMustRefetch(true);
        const current = await refresh();
        if (current) setMustRefetch(false);
        if (current && reflects(current, intention)) {
          setPending(null);
          setStatus("saved");
          return current;
        }
        setPending(request);
        setStatus("failed");
        return null;
      } finally {
        setBusy(false);
      }
    },
    [refresh],
  );

  const save = useCallback(
    async (intention: PatientIntention) => {
      if (busy || mustRefetch) return null;
      setPending(null);
      return send({ ...intention, expected_head: journey.head, request_id: crypto.randomUUID() } as PatientMutation);
    },
    [busy, journey.head, mustRefetch, send],
  );

  const retry = useCallback(async () => {
    if (busy || !pending) return null;
    if (mustRefetch) {
      setBusy(true);
      const current = await refresh();
      if (!current) {
        setBusy(false);
        return null;
      }
      setMustRefetch(false);
      if (reflects(current, pending)) {
        setPending(null);
        setStatus("saved");
        setBusy(false);
        return current;
      }
      setBusy(false);
    }
    return send(pending);
  }, [busy, mustRefetch, pending, refresh, send]);

  return { journey, busy: busy || mustRefetch, status, save, retry, canRetry: pending !== null, refresh };
}
