import type { PatientJourneyResponse } from "@/lib/api/patient-schema";

export const patientOffers = {
  version: "offer-version-1",
  slots: [
    { slot_id: "skmc-endo:2026-10-09:09:30", unit: "skmc-endo", starts_at: "2026-10-09T09:30:00+04:00" },
    { slot_id: "skmc-endo:2026-10-08:11:00", unit: "skmc-endo", starts_at: "2026-10-08T11:00:00+04:00" },
    { slot_id: "skmc-endo:2026-10-12:13:00", unit: "skmc-endo", starts_at: "2026-10-12T13:00:00+04:00" },
  ],
};

export function patientFixture(): PatientJourneyResponse {
  return {
    meta: { api: "wasl-api/1", data_mode: "simulated" },
    journey: {
      schema: "wasl-patient-journey/1",
      head: "canonical-signature",
      seq: 0,
      ledger_status: "ok",
      teach_back: { question_id: "crc-next-step-v1", status: "pending", completed_at: "", event: "" },
      contact_permission: { status: "unknown", updated_at: "", event: "" },
      slot_selection: { status: "unselected", offers_version: patientOffers.version, slot_id: "", selected_at: "", event: "" },
      preparation: {
        items: [
          { item_id: "clear_fluids_and_prep", completed: false, updated_at: "", event: "" },
          { item_id: "escort_home", completed: false, updated_at: "", event: "" },
        ],
        completed: 0,
        total: 2,
      },
      last_action_at: "",
    },
    offers: patientOffers,
    expires_at: "2026-11-13T12:00:00Z",
  };
}
