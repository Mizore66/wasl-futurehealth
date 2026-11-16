import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { BookFlow } from "@/components/patient/book-flow";
import { CheckFlow } from "@/components/patient/check-flow";
import { patientApproved, type Lang } from "@/components/patient/i18n";
import { NoticeScreen } from "@/components/patient/notice-screen";
import { patientScript } from "@/components/patient/programme";
import { PatientJourneyResponse, type PatientJourney, type PatientOffers, type PatientProgramme } from "@/lib/api/patient-schema";
import { patientFixture, patientOffers } from "./patient-fixture";

/** Representative synthetic cases: the programme, its served next step, and the wrong answer the API offers (pipeline/api_patient.go). */
const CASES: { programme: PatientProgramme; next: string; distractor: string; served: string; answer: string }[] = [
  { programme: "crc", next: "colonoscopy", distractor: "blood_test", served: "colonoscopy", answer: "small camera" },
  { programme: "cervical", next: "colposcopy", distractor: "wait_for_symptoms", served: "colposcopy", answer: "colposcopy" },
  { programme: "lung_nodule", next: "ct", distractor: "wait_for_symptoms", served: "CT scan of the chest", answer: "CT scan of the chest" },
  { programme: "diabetic_retinopathy", next: "refer", distractor: "wait_for_symptoms", served: "eye clinic", answer: "eye clinic" },
  { programme: "breast", next: "recall", distractor: "wait_for_symptoms", served: "breast clinic", answer: "breast clinic" },
  { programme: "lung_ldct", next: "annual", distractor: "urgent_visit", served: "yearly lung screening", answer: "yearly lung screening" },
  { programme: "ifhas", next: "follow_up", distractor: "wait_for_symptoms", served: "follow-up visit with your doctor", answer: "follow-up visit with my doctor" },
];

/** Words that belong to exactly one programme's patient copy. */
const VOCABULARY: Record<PatientProgramme, string[]> = {
  crc: ["colonoscopy", "bowel", "blood you cannot see", "Endoscopy", "SKMC", "Hessa", "Fatima"],
  cervical: ["cervi", "colposcopy"],
  lung_nodule: ["spot", "chest"],
  diabetic_retinopathy: ["eye"],
  breast: ["mammogram", "breast"],
  lung_ldct: ["low-dose", "lung screening"],
  ifhas: ["blood pressure", "cholesterol", "health check"],
};

const NO_OFFERS: PatientOffers = { version: "", procedure: "", slots: [] };

function journeyFor(programme: PatientProgramme, next: string, distractor: string): { journey: PatientJourney; offers: PatientOffers } {
  const base = patientFixture();
  const crc = programme === "crc";
  const raw = {
    ...base,
    journey: {
      ...base.journey,
      programme,
      next_step: next,
      teach_back: { ...base.journey.teach_back, question_id: crc ? "crc-next-step-v1" : `${programme}-next-step-v1`, answers: [distractor, next, "not_sure"] },
      slot_selection: { ...base.journey.slot_selection, offers_version: crc ? patientOffers.version : "" },
      preparation: crc ? base.journey.preparation : { items: [], completed: 0, total: 0 },
    },
    offers: crc ? patientOffers : NO_OFFERS,
  };
  const parsed = PatientJourneyResponse.parse(JSON.parse(JSON.stringify(raw)));
  return { journey: parsed.journey, offers: parsed.offers };
}

function render(journey: PatientJourney, offers: PatientOffers, lang: Lang = "en") {
  // Draft languages render only on the internal review surface (#133).
  const query = { lang, surface: patientApproved(lang) ? ("patient" as const) : ("review" as const) };
  return {
    notice: renderToStaticMarkup(createElement(NoticeScreen, { lang, journey })),
    check: renderToStaticMarkup(createElement(CheckFlow, { query, initial: journey })),
    book: renderToStaticMarkup(createElement(BookFlow, { query, initial: journey, offers })),
  };
}

describe("programme-aware patient phone", () => {
  it.each(CASES)("$programme: notice and teach-back carry only the served recommendation", ({ programme, next, distractor, served, answer }) => {
    const { journey, offers } = journeyFor(programme, next, distractor);
    const { notice, check } = render(journey, offers);
    expect(notice.toLowerCase()).toContain(served.toLowerCase());
    expect(check.toLowerCase()).toContain(answer.toLowerCase());
    for (const [other, words] of Object.entries(VOCABULARY)) {
      if (other === programme) continue;
      for (const word of words) {
        expect(notice, `${programme} notice shows ${other} copy`).not.toContain(word);
        expect(check, `${programme} teach-back shows ${other} copy`).not.toContain(word);
      }
    }
  });

  it.each(CASES.filter((c) => c.programme !== "crc"))("$programme: no offer means no slot, confirmation or preparation, and a coordinator contact", ({ programme, next, distractor }) => {
    const { journey, offers } = journeyFor(programme, next, distractor);
    const { book } = render(journey, offers);
    const t = patientScript(journey, "en");
    expect(book).toContain('data-testid="no-offer"');
    expect(book).toContain("The care coordinator will contact you");
    expect(book).toContain(t.notice.next);
    for (const absent of ["slot-date", "Save this preferred time", "Before your appointment", "October", "SKMC", "colonoscopy"]) {
      expect(book).not.toContain(absent);
    }
  });

  it("asks the server-offered teach-back answers in its order", () => {
    const { journey } = journeyFor("lung_ldct", "annual", "urgent_visit");
    expect(patientScript(journey, "en").check.answers.map((a) => a.id)).toEqual(["urgent_visit", "annual", "not_sure"]);
  });

  it("keeps non-CRC medical copy in English, marked as awaiting translation review, in every other language", () => {
    const { journey, offers } = journeyFor("breast", "recall", "wait_for_symptoms");
    const { notice } = render(journey, offers, "ar");
    expect(notice).toContain("until a reviewed translation is ready");
    expect(notice).toContain('lang="en"');
    expect(notice).toContain("breast clinic");
  });

  it("falls back to a care-team step for a recommendation the copy table does not know", () => {
    const { journey } = journeyFor("breast", "care_team", "wait_for_symptoms");
    const script = patientScript(journey, "en");
    expect(script.notice.next).toBe("The care team will contact you about the next step.");
    expect(script.check.answers.find((a) => a.id === "care_team")?.text).toBe("The care team will contact me about the next step");
  });
});

describe("CRC booking reads the served offer", () => {
  it("keeps the CRC journey: colonoscopy, SKMC endoscopy and its two preparation checks", () => {
    const { journey, offers } = journeyFor("crc", "colonoscopy", "blood_test");
    const { book } = render(journey, offers);
    expect(book).toContain("Your colonoscopy appointment");
    expect(book).toContain("Sheikh Khalifa Medical City");
    expect(book).toContain("Friday");
    expect(book).toContain("October");
    expect(book).toContain("Before your appointment");
  });

  it("takes the month and place from the offer, never the October or SKMC constants", () => {
    const { journey } = journeyFor("crc", "colonoscopy", "blood_test");
    const offers: PatientOffers = {
      version: "v2",
      procedure: "colonoscopy",
      slots: [{ slot_id: "other-unit:2026-11-17:10:00", unit: "other-unit", starts_at: "2026-11-17T10:00:00+04:00" }],
    };
    const { book } = render(journey, offers);
    expect(book).toContain("Tuesday");
    expect(book).toContain("November");
    expect(book).not.toContain("October");
    expect(book).not.toContain("SKMC");
    expect(book).not.toContain("Sheikh Khalifa");
  });

  it("shows no preparation checklist when the served journey has none", () => {
    const { journey, offers } = journeyFor("crc", "colonoscopy", "blood_test");
    const { book } = render({ ...journey, preparation: { items: [], completed: 0, total: 0 } }, offers);
    expect(book).not.toContain("Before your appointment");
    expect(book).toContain("Save this preferred time");
  });
});
