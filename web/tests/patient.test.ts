import { afterEach, describe, expect, it, vi } from "vitest";
import { DICTS, LANGS } from "@/components/patient/i18n";
import { parsePhoneQuery, phoneHref, slotLabel } from "@/components/patient/journey";
import { getPatientJourney, postPatientJourney } from "@/lib/api/patient-source";
import { PatientJourneyResponse, PatientMutation } from "@/lib/api/patient-schema";
import { patientFixture, patientOffers } from "./patient-fixture";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});
describe("phone query", () => {
  it("defaults to Arabic without browser-held consent", () => {
    expect(parsePhoneQuery({})).toEqual({ lang: "ar" });
  });

  it("rejects an unknown language instead of rendering a missing dictionary", () => {
    expect(parsePhoneQuery({ lang: "fr" }).lang).toBe("ar");
  });

  it("round-trips only language through step links, never consent", () => {
    for (const lang of LANGS) {
      const href = phoneHref("book", { lang });
      const search = Object.fromEntries(new URL(href, "http://x").searchParams);
      expect(parsePhoneQuery(search)).toEqual({ lang });
      expect(href).not.toContain("consent");
    }
  });
});

describe("booking slots", () => {
  it("formats the server October offers with the right weekdays", () => {
    const t = DICTS.en.book;
    expect(patientOffers.slots.map((slot) => slotLabel(slot, t))).toEqual([
      { weekday: "Friday", date: "9", time: "9:30", period: "am" },
      { weekday: "Thursday", date: "8", time: "11:00", period: "am" },
      { weekday: "Monday", date: "12", time: "1:00", period: "pm" },
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
      { kind: "slot_selected", data: { slot_id: patientOffers.slots[0].slot_id, offers_version: patientOffers.version } },
      { kind: "preparation_changed", data: { item_id: "escort_home", completed: false } },
    ];
    for (const variant of variants) expect(PatientMutation.parse({ ...base, ...variant })).toEqual({ ...base, ...variant });
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
