import { afterEach, describe, expect, it, vi } from "vitest";
import { LANGS } from "@/components/patient/i18n";
import { ALL_DICTS as DICTS } from "@/components/patient/i18n/all";
import { parsePhoneQuery, phoneHref, slotLabel, surfaceLangs } from "@/components/patient/journey";
import { getPatientJourney, postPatientJourney } from "@/lib/api/patient-source";
import { PatientJourneyResponse, PatientMutation, PatientOffer } from "@/lib/api/patient-schema";
import { patientFixture, patientOffers } from "./patient-fixture";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});
describe("phone query", () => {
  it("defaults to the reviewed English source without browser-held consent", () => {
    expect(parsePhoneQuery({})).toEqual({ lang: "en", surface: "patient" });
  });

  it("rejects an unknown language instead of rendering a missing dictionary", () => {
    expect(parsePhoneQuery({ lang: "fr" }).lang).toBe("en");
  });

  it("round-trips only language through step links, never consent", () => {
    for (const surface of ["patient", "review"] as const) {
      for (const lang of surfaceLangs(surface)) {
        const href = phoneHref("book", { lang, surface });
        const search = Object.fromEntries(new URL(href, "http://x").searchParams);
        expect(parsePhoneQuery(search, surface)).toEqual({ lang, surface });
        expect(href).not.toContain("consent");
      }
    }
  });
});

describe("booking slots", () => {
  it("formats the server October offers with the right weekdays", () => {
    const t = DICTS.en.book;
    expect(patientOffers.slots.map((slot) => slotLabel(slot, t, "en"))).toEqual([
      { weekday: "Friday", date: "9", month: "October", time: "9:30", period: "am" },
      { weekday: "Thursday", date: "8", month: "October", time: "11:00", period: "am" },
      { weekday: "Monday", date: "12", month: "October", time: "1:00", period: "pm" },
    ]);
  });

  it("uses pending-review wording, never a confirmed reservation", () => {
    for (const lang of LANGS) {
      expect(DICTS[lang].book.confirmed.toLowerCase()).not.toContain("your appointment is confirmed");
    }
    expect(DICTS.en.book.confirmed).toBe("Your preferred time is saved for coordinator review — simulated only; no appointment is reserved.");
  });
});

describe("patient API contract", () => {
  it("parses a server projection with no case id, staff timeline or token", () => {
    const raw = JSON.stringify(patientFixture());
    expect(PatientJourneyResponse.parse(JSON.parse(raw)).journey.head).toBe("canonical-signature");
    expect(raw).not.toContain("token_sha256");
    expect(raw).not.toContain("timeline");
  });

  it("accepts the four exact mutation variants", () => {
    const base = { expected_head: "canonical-signature", request_id: crypto.randomUUID() };
    const variants = [
      { kind: "teach_back_completed", data: { question_id: "crc-next-step-v1", answer_id: "colonoscopy" } },
      { kind: "contact_permission_changed", data: { permission: "opted_out" } },
      { kind: "slot_selected", data: { offer_id: patientOffers.slots[0].offer_id, offers_version: patientOffers.version } },
      { kind: "preparation_changed", data: { item_id: "escort_home", completed: false } },
    ];
    for (const variant of variants) expect(PatientMutation.parse({ ...base, ...variant })).toEqual({ ...base, ...variant });
  });

  it("offers carry only an offer id, the date and the region: no unit code or slot id (#165)", () => {
    const offer = { offer_id: "offer-cx-0625", region: "AD", starts_at: "2026-06-25" };
    expect(PatientOffer.parse(offer)).toEqual(offer);
    expect(PatientOffer.safeParse({ ...offer, unit: "AD-2" }).success).toBe(false);
    expect(PatientOffer.safeParse({ ...offer, slot_id: "sim:colposcopy:2:1" }).success).toBe(false);
    const base = { expected_head: "canonical-signature", request_id: crypto.randomUUID(), kind: "slot_selected" };
    expect(PatientMutation.safeParse({ ...base, data: { slot_id: "sim:colposcopy:2:1", offers_version: "v" } }).success).toBe(false);
  });

  it("rejects case, role, actor and unknown data assertions", () => {
    const base = { kind: "contact_permission_changed", expected_head: "head", request_id: crypto.randomUUID(), data: { permission: "granted" } };
    for (const extra of [{ case: "other" }, { role: "coordinator" }, { actor: "other" }]) {
      expect(PatientMutation.safeParse({ ...base, ...extra }).success).toBe(false);
    }
    expect(PatientMutation.safeParse({ ...base, data: { permission: "granted", case: "other" } }).success).toBe(false);
  });
});

describe("server-only patient proxy", () => {
  it("forwards the raw body and patient credential, never a staff identity", async () => {
    vi.stubEnv("WASL_API_URL", "http://127.0.0.1:8088");
    const fixture = patientFixture();
    const fetchMock = vi.fn().mockResolvedValueOnce(new Response(JSON.stringify(fixture), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    await getPatientJourney("opaque-patient-token");
    expect(fetchMock.mock.calls[0][0]).toBe("http://127.0.0.1:8088/api/patient/journey");
    const readHeaders = new Headers(fetchMock.mock.calls[0][1].headers);
    expect(readHeaders.get("authorization")).toBe("Bearer opaque-patient-token");
    expect(readHeaders.get("x-wasl-role")).toBeNull();
    expect(readHeaders.get("x-wasl-case")).toBeNull();

    const raw = JSON.stringify({
      kind: "preparation_changed", expected_head: fixture.journey.head, request_id: crypto.randomUUID(), data: { item_id: "escort_home", completed: false },
    });
    fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ event: null, journey: fixture.journey, replayed: true }), { status: 200 }));
    const result = await postPatientJourney("opaque-patient-token", raw);
    expect(result).toMatchObject({ ok: true, status: 200 });
    expect(fetchMock.mock.calls[1][1].body).toBe(raw);
    expect(new Headers(fetchMock.mock.calls[1][1].headers).get("authorization")).toBe("Bearer opaque-patient-token");
  });
});
