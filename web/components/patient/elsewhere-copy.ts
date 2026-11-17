import type { Lang } from "./i18n";

/*
 * Copy for the "already done elsewhere" report (issue #177). Kept beside its component rather than in the shared
 * patient dictionaries. It says plainly that the report is checked, not accepted as a result.
 */
export type ElsewhereCopy = {
  open: string;
  title: string;
  intro: string;
  procedure: string;
  performed: string;
  performedHint: string;
  provider: string;
  reportRef: string;
  optional: string;
  submit: string;
  sending: string;
  cancel: string;
  failed: string;
  pending: string;
  overdue: string;
  resumed: Record<"not_found" | "not_matching" | "non_closing", string>;
  evidenceClosed: string;
  byCareTeam: string;
  receipt: string;
  notResult: string;
};

export const ELSEWHERE_COPY: Record<Lang, ElsewhereCopy> = {
  en: {
    open: "I already had this done somewhere else",
    title: "Done somewhere else",
    intro: "Tell us where and roughly when. Your care team will ask that place for the report. Until they have it, we will not ask you to book.",
    procedure: "What was done",
    performed: "Roughly when",
    performedHint: "The month is enough.",
    provider: "Where (hospital or clinic)",
    reportRef: "Report or reference number",
    optional: "optional",
    submit: "Send to my care team",
    sending: "Sending…",
    cancel: "Cancel",
    failed: "This could not be sent. Nothing was saved. Please try again or call us.",
    pending: "We are asking {provider} for the report. Your care team aims to have it by {due}.",
    overdue: "We are still waiting for the report from {provider}. Your care team will call you.",
    resumed: {
      not_found: "{provider} could not find the report, so your care team will contact you about the next step.",
      not_matching: "The report we received was for a different test, so your care team will contact you about the next step.",
      non_closing: "The report we received was not enough to finish this check, so your care team will contact you about the next step.",
    },
    evidenceClosed: "Your care team has the report and has finished this check.",
    byCareTeam: "Your care team recorded this from a call with you.",
    receipt: "Reference",
    notResult: "This is a report for your care team to check, not a result. This is a demo: no hospital is contacted.",
  },
  ar: {
    open: "أجريتُ هذا الفحص في مكان آخر",
    title: "أُجري في مكان آخر",
    intro: "أخبرنا أين ومتى تقريباً. سيطلب فريق الرعاية التقرير من ذلك المكان. ولن نطلب منك الحجز حتى يصلهم.",
    procedure: "ما الذي أُجري",
    performed: "متى تقريباً",
    performedHint: "يكفي ذكر الشهر.",
    provider: "أين (مستشفى أو عيادة)",
    reportRef: "رقم التقرير أو المرجع",
    optional: "اختياري",
    submit: "أرسل إلى فريق الرعاية",
    sending: "جارٍ الإرسال…",
    cancel: "إلغاء",
    failed: "تعذّر الإرسال ولم يُحفظ شيء. حاول مرة أخرى أو اتصل بنا.",
    pending: "نطلب التقرير من {provider}. يسعى فريق الرعاية للحصول عليه بحلول {due}.",
    overdue: "ما زلنا ننتظر التقرير من {provider}. سيتصل بك فريق الرعاية.",
    resumed: {
      not_found: "لم يجد {provider} التقرير، لذلك سيتواصل معك فريق الرعاية بشأن الخطوة التالية.",
      not_matching: "التقرير الذي وصلنا كان لفحص مختلف، لذلك سيتواصل معك فريق الرعاية بشأن الخطوة التالية.",
      non_closing: "التقرير الذي وصلنا لا يكفي لإنهاء هذا الفحص، لذلك سيتواصل معك فريق الرعاية بشأن الخطوة التالية.",
    },
    evidenceClosed: "وصل التقرير إلى فريق الرعاية وأنهى هذا الفحص.",
    byCareTeam: "سجّل فريق الرعاية هذا من مكالمة معك.",
    receipt: "المرجع",
    notResult: "هذا بلاغ يتحقق منه فريق الرعاية، وليس نتيجة. هذا عرض تجريبي: لا يتم التواصل مع أي مستشفى.",
  },
  ur: {
    open: "میں یہ کہیں اور کروا چکا ہوں",
    title: "کہیں اور ہو چکا ہے",
    intro: "ہمیں بتائیں کہاں اور تقریباً کب۔ آپ کی نگہداشت ٹیم وہاں سے رپورٹ منگوائے گی۔ جب تک رپورٹ نہ ملے، ہم آپ سے بکنگ کا نہیں کہیں گے۔",
    procedure: "کیا کروایا گیا",
    performed: "تقریباً کب",
    performedHint: "مہینہ کافی ہے۔",
    provider: "کہاں (ہسپتال یا کلینک)",
    reportRef: "رپورٹ یا حوالہ نمبر",
    optional: "اختیاری",
    submit: "میری نگہداشت ٹیم کو بھیجیں",
    sending: "بھیجا جا رہا ہے…",
    cancel: "منسوخ کریں",
    failed: "یہ نہیں بھیجا جا سکا اور کچھ محفوظ نہیں ہوا۔ دوبارہ کوشش کریں یا ہمیں کال کریں۔",
    pending: "ہم {provider} سے رپورٹ منگوا رہے ہیں۔ آپ کی ٹیم اسے {due} تک حاصل کرنے کی کوشش کرے گی۔",
    overdue: "ہم ابھی تک {provider} کی رپورٹ کا انتظار کر رہے ہیں۔ آپ کی ٹیم آپ کو کال کرے گی۔",
    resumed: {
      not_found: "{provider} کو رپورٹ نہیں ملی، اس لیے آپ کی ٹیم اگلے قدم کے بارے میں آپ سے رابطہ کرے گی۔",
      not_matching: "ملنے والی رپورٹ کسی اور ٹیسٹ کی تھی، اس لیے آپ کی ٹیم اگلے قدم کے بارے میں آپ سے رابطہ کرے گی۔",
      non_closing: "ملنے والی رپورٹ یہ جانچ مکمل کرنے کے لیے کافی نہیں تھی، اس لیے آپ کی ٹیم آپ سے رابطہ کرے گی۔",
    },
    evidenceClosed: "آپ کی ٹیم کو رپورٹ مل گئی ہے اور یہ جانچ مکمل ہو گئی ہے۔",
    byCareTeam: "آپ کی ٹیم نے یہ آپ کے ساتھ کال سے درج کیا۔",
    receipt: "حوالہ",
    notResult: "یہ آپ کی ٹیم کے جانچنے کے لیے ایک اطلاع ہے، نتیجہ نہیں۔ یہ ڈیمو ہے: کسی ہسپتال سے رابطہ نہیں کیا جاتا۔",
  },
  fil: {
    open: "Nagawa ko na ito sa ibang lugar",
    title: "Nagawa na sa ibang lugar",
    intro: "Sabihin sa amin kung saan at kailan, kahit tantiya lang. Hihingin ng inyong care team ang report doon. Hangga't wala pa ito, hindi namin kayo hihilingang mag-book.",
    procedure: "Ano ang ginawa",
    performed: "Tantiyang kailan",
    performedHint: "Sapat na ang buwan.",
    provider: "Saan (ospital o klinika)",
    reportRef: "Numero ng report o reference",
    optional: "opsyonal",
    submit: "Ipadala sa aking care team",
    sending: "Ipinapadala…",
    cancel: "Kanselahin",
    failed: "Hindi ito naipadala at walang na-save. Subukan muli o tawagan kami.",
    pending: "Hinihingi namin ang report sa {provider}. Layunin ng inyong care team na makuha ito bago ang {due}.",
    overdue: "Hinihintay pa namin ang report mula sa {provider}. Tatawagan kayo ng inyong care team.",
    resumed: {
      not_found: "Hindi nahanap ng {provider} ang report, kaya kokontakin kayo ng inyong care team tungkol sa susunod na hakbang.",
      not_matching: "Para sa ibang test ang report na natanggap, kaya kokontakin kayo ng inyong care team tungkol sa susunod na hakbang.",
      non_closing: "Hindi sapat ang report na natanggap para matapos ang check na ito, kaya kokontakin kayo ng inyong care team.",
    },
    evidenceClosed: "Nasa care team na ang report at tapos na ang check na ito.",
    byCareTeam: "Itinala ito ng inyong care team mula sa tawag sa inyo.",
    receipt: "Reference",
    notResult: "Ito ay ulat na susuriin ng inyong care team, hindi resulta. Demo ito: walang ospital na kinokontak.",
  },
};
