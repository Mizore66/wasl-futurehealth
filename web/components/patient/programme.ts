import type { PatientJourney, PatientOffer, PatientOffers, PatientProgramme } from "@/lib/api/patient-schema";
import { EN_DICT, type Lang, type PatientDict } from "./i18n";

type OtherProgramme = Exclude<PatientProgramme, "crc">;

/** Patient-safe words for one served recommendation. They name the next step only: no deadline, preparation or new advice. */
interface StepCopy {
  title: string;
  body: string;
  next: string;
  right: string;
}

const CARE_TEAM: StepCopy = {
  title: "Your result is being reviewed",
  body: "This is not a diagnosis. A person on the care team is reviewing your result.",
  next: "The care team will contact you about the next step.",
  right: "The care team will contact me about the next step",
};

/** Keyed by programme, then by the canonical next-step key the API serves (pipeline/api_patient.go patientNextStep). */
const STEPS: Record<OtherProgramme, Record<string, StepCopy>> = {
  cervical: {
    colposcopy: {
      title: "Your cervical screening result needs one more check",
      body: "This is not a diagnosis. The screening test found changes that need a closer look.",
      next: "The next step is a colposcopy, where a doctor looks closely at the cervix. The care team will contact you to arrange it.",
      right: "A closer look at the cervix (colposcopy)",
    },
  },
  lung_nodule: {
    ct: {
      title: "Your scan found a small spot on the lung that needs a follow-up scan",
      body: "This is not a diagnosis. A follow-up scan checks whether the spot changes.",
      next: "The next step is another CT scan of the chest. The care team will contact you to arrange it.",
      right: "Another CT scan of the chest",
    },
    ct_or_pet_or_tissue: {
      title: "Your scan found a spot on the lung that needs more tests",
      body: "This is not a diagnosis. More tests show what the spot is.",
      next: "The next step is more tests, such as another scan or a small sample. The care team will contact you to explain and arrange them.",
      right: "More tests, such as another scan or a small sample",
    },
    none: {
      title: "The spot on your scan needs no follow-up scan",
      body: "Your scan found a small spot on the lung. The rule for spots like this one does not ask for another scan.",
      next: "No follow-up scan is needed for this spot. Call the care team if you have questions.",
      right: "No follow-up scan is needed for this spot",
    },
  },
  diabetic_retinopathy: {
    refer: {
      title: "Your eye screening found changes that need an eye clinic visit",
      body: "This is not a diagnosis. Diabetes can change the back of the eye, and an eye doctor needs to check it.",
      next: "The next step is a full eye examination at the eye clinic. The care team will contact you to arrange it.",
      right: "A full eye examination at the eye clinic",
    },
    rescreen: {
      title: "Your eye screening needs no extra visit now",
      body: "Your eye photos did not show changes that need the eye clinic.",
      next: "The next step is your routine eye screening at the usual time.",
      right: "My routine eye screening at the usual time",
    },
    repeat_photo: {
      title: "Your eye photos need to be taken again",
      body: "This is not a diagnosis. The photos could not be read clearly.",
      next: "The next step is to take the eye photos again. The care team will contact you to arrange it.",
      right: "Taking the eye photos again",
    },
  },
  breast: {
    recall: {
      title: "Your mammogram needs a further check",
      body: "This is not a diagnosis. Most people who are asked back do not have cancer.",
      next: "The next step is an assessment appointment at the breast clinic. The care team will contact you to arrange it.",
      right: "An assessment appointment at the breast clinic",
    },
    routine: {
      title: "Your mammogram needs no extra check now",
      body: "Your mammogram did not show anything that needs a further check.",
      next: "The next step is your routine breast screening at the usual time.",
      right: "My routine breast screening at the usual time",
    },
    repeat_mammogram: {
      title: "Your mammogram needs to be repeated",
      body: "This is not a diagnosis. The images could not be read clearly.",
      next: "The next step is to repeat the mammogram. The care team will contact you to arrange it.",
      right: "Repeating the mammogram",
    },
  },
  lung_ldct: {
    annual: {
      title: "Your lung screening scan needs no extra check now",
      body: "Your low-dose scan did not show anything that needs a check before your next screening.",
      next: "The next step is your next yearly lung screening scan.",
      right: "My next yearly lung screening scan",
    },
    complete_ldct: {
      title: "Your lung screening scan needs to be completed",
      body: "This is not a diagnosis. Part of the scan could not be assessed.",
      next: "The next step is another low-dose scan to complete it. The care team will contact you to arrange it.",
      right: "Another low-dose scan to complete it",
    },
    followup_ldct: {
      title: "Your lung screening scan needs a follow-up scan",
      body: "This is not a diagnosis. A follow-up scan checks whether what was seen changes.",
      next: "The next step is another low-dose scan. The care team will contact you to arrange it.",
      right: "Another low-dose scan",
    },
    workup: {
      title: "Your lung screening scan needs more tests",
      body: "This is not a diagnosis. More tests show what the scan found.",
      next: "The next step is more tests with a specialist team. The care team will contact you to explain and arrange them.",
      right: "More tests with a specialist team",
    },
  },
  ifhas: {
    follow_up: {
      title: "Your health check results need a follow-up",
      body: "This is not a diagnosis. Some results, such as blood pressure, blood sugar, cholesterol or weight, were outside the healthy range.",
      next: "The next step is a follow-up visit with your doctor about these results. The care team will contact you to arrange it.",
      right: "A follow-up visit with my doctor about these results",
    },
    routine: {
      title: "Your health check results need no follow-up now",
      body: "Your results were in the healthy range.",
      next: "The next step is your next routine health check at the usual time.",
      right: "My next routine health check at the usual time",
    },
  },
};

const WRONG: Record<string, string> = {
  wait_for_symptoms: "Nothing, unless I feel unwell",
  urgent_visit: "An urgent hospital visit this week",
  not_sure: "I am not sure",
};

/** Everything the phone says about this case's programme. Non-CRC copy is English until a qualified translation review (#133). */
export interface PatientScript {
  lang: Lang;
  gated: boolean;
  notice: { greet: string; title: string; body: string; next: string; contact: string; alsoCall: string };
  check: {
    question: string;
    answers: { id: string; text: string }[];
    explain: readonly string[];
    callbackOpen: string;
    thanks: string;
    willCall: string;
  };
  book: { title: string; noOffer: string };
}

/**
 * `dict` is the dictionary for `lang` from the surface's dictionary set (English on patient routes). A closed case
 * (`case_closed`) gets the finished-follow-up wording instead of the booking next step.
 */
export function patientScript(
  journey: Pick<PatientJourney, "programme" | "next_step" | "teach_back" | "case_closed">,
  lang: Lang,
  dict: PatientDict = EN_DICT,
): PatientScript {
  const script = openCaseScript(journey, lang, dict);
  if (!journey.case_closed) return script;
  const n = journey.programme === "crc" ? dict.notice : EN_DICT.notice;
  return {
    ...script,
    notice: { ...script.notice, title: n.closedTitle, body: n.closedBody, next: n.closedNext },
    book: { ...script.book, noOffer: n.closedNext },
  };
}

function openCaseScript(journey: Pick<PatientJourney, "programme" | "next_step" | "teach_back">, lang: Lang, dict: PatientDict): PatientScript {
  const ids = journey.teach_back.answers;
  if (journey.programme === "crc") {
    const { notice: n, check: c, book: b } = dict;
    const crcAnswers: Record<string, string> = { blood_test: c.wrong, colonoscopy: c.right, not_sure: c.notSure };
    return {
      lang,
      gated: false,
      notice: { greet: n.greet, title: n.title, body: n.body, next: n.next, contact: n.nurse, alsoCall: n.alsoCall },
      check: {
        question: c.question,
        answers: ids.map((id) => ({ id, text: crcAnswers[id] ?? "" })),
        explain: c.explain,
        callbackOpen: c.callbackOpen,
        thanks: c.thanks,
        willCall: `${c.willCall} ${c.alsoCall}`,
      },
      book: { title: b.title, noOffer: b.noOffer },
    };
  }
  const step = STEPS[journey.programme][journey.next_step] ?? CARE_TEAM;
  return {
    lang: "en",
    gated: lang !== "en",
    notice: { greet: "Hello", title: step.title, body: step.body, next: step.next, contact: "Your care team", alsoCall: "You can also call:" },
    check: {
      question: "What is the next step after your result?",
      answers: ids.map((id) => ({ id, text: id === journey.next_step ? step.right : (WRONG[id] ?? "") })),
      explain: ["The fault is in our explanation, not in you.", step.body, step.next],
      callbackOpen: "The care team will call you by {date} to go through it with you. This is a demo: no call is placed.",
      thanks: "Thank you",
      willCall: `${step.next} You can also call:`,
    },
    book: {
      title: "Your next step",
      noOffer: "No appointment time is offered on this phone. The care coordinator will contact you about what comes next, or you can call:",
    },
  };
}

/** Simulated booking regions (booking/booking/model.py REGIONS). Place names only, in English until translation review (#133). */
const REGIONS: Record<string, string> = { AD: "Abu Dhabi city", AA: "Al Ain", DH: "Al Dhafra" };

export interface OfferPlace {
  place: string;
  address: string;
  abbr?: string;
  /** English copy shown inside a translated phone; render with lang="en". */
  gated: boolean;
}

/**
 * Patient-facing place for one simulated offer. The fixed CRC demo offers (`source: "demo_fixed"`) keep their reviewed
 * SKMC wording; a booking catalogue slot names only its region (the payload carries no unit code). An unknown region
 * shows no place.
 */
export function offerPlace(slot: Pick<PatientOffer, "region"> | undefined, source: PatientOffers["source"], lang: Lang, dict: PatientDict = EN_DICT): OfferPlace | null {
  if (!slot) return null;
  if (source === "demo_fixed") {
    const b = dict.book;
    return { place: b.place, address: b.address, abbr: "SKMC", gated: false };
  }
  const region = REGIONS[slot.region ?? ""];
  if (!region) return null;
  return { place: `${region} region`, address: "The care coordinator confirms the exact clinic with you.", gated: lang !== "en" };
}

export interface OfferCopy {
  title: string;
  /** What happens when the patient does not allow SMS. */
  smsNo: string;
  gated: boolean;
}

/** Patient-facing words for a simulated offer's procedure. Colposcopy copy is English until translation review (#133). */
export function offerCopy(procedure: string, lang: Lang, dict: PatientDict = EN_DICT): OfferCopy | null {
  if (procedure === "colonoscopy") return { title: dict.book.title, smsNo: dict.book.smsNo, gated: false };
  if (procedure === "colposcopy") {
    return {
      title: "Your colposcopy appointment",
      smsNo: "The care coordinator will call you before the appointment to remind you.",
      gated: lang !== "en",
    };
  }
  return null;
}
