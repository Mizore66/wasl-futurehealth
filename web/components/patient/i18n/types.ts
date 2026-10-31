export const LANGS = ["ar", "en", "ur", "fil"] as const;
export type Lang = (typeof LANGS)[number];

type Three = readonly [string, string, string];
/** Sunday first, matching Date#getUTCDay. */
type Week = readonly [string, string, string, string, string, string, string];

/** Every phone string in one language. A missing or extra key fails `tsc`. */
export interface PatientDict {
  dir: "rtl" | "ltr";
  langName: string;
  langTitle: string;
  clinic: string;
  call: string;
  close: string;
  notice: {
    greet: string;
    title: string;
    body: string;
    nextHead: string;
    next: string;
    nurse: string;
    alsoCall: string;
    rights: string;
    rightsItems: Three;
    rightsNote: string;
  };
  check: {
    steps: Three;
    /** `{n}` and `{name}` are replaced. */
    stepFmt: string;
    intro: string;
    question: string;
    groupLabel: string;
    letters: readonly [string, string];
    wrong: string;
    right: string;
    notTest: string;
    explainTitle: string;
    explain: Three;
    retry: string;
    correct: string;
    consentTitle: string;
    consentBody: string;
    allow: string;
    decline: string;
    thanks: string;
    willCall: string;
    alsoCall: string;
    smsYes: string;
    smsNo: string;
  };
  book: {
    title: string;
    proposed: string;
    confirmed: string;
    weekdays: Week;
    october: string;
    sep: string;
    at: string;
    am: string;
    pm: string;
    mapLabel: string;
    place: string;
    address: string;
    confirm: string;
    another: string;
    pick: string;
    othersLabel: string;
    smsYes: string;
    smsNo: string;
    change: string;
    prepHead: string;
    warn: string;
    prep: readonly [string, string];
  };
}
