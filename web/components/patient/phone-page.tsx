import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { getPatientJourney, PATIENT_COOKIE } from "@/lib/api/patient-source";
import { PatientAccessError, patientAccessFailure } from "./access-error";
import { BookFlow } from "./book-flow";
import { CheckFlow } from "./check-flow";
import { parsePhoneQuery, phoneHref, type Step, type Surface } from "./journey";
import { NoticeScreen } from "./notice-screen";
import { OutcomeHandoff } from "./outcome-handoff";
import { PhoneFrame } from "./phone-frame";
import { DictsProvider } from "./i18n/context";
import { dictsForSurface } from "./i18n/drafts";

type Search = Record<string, string | string[] | undefined>;

/**
 * One phone step on a surface. The surface comes from the route, so a patient URL can never select a draft
 * dictionary: `parsePhoneQuery` replaces an unreviewed `lang` with the patient default before anything renders.
 */
export async function PhonePage({ step, search, surface }: { step: Step; search: Search; surface: Surface }) {
  const query = parsePhoneQuery(search, surface);
  const accessFailure = patientAccessFailure(search.error);
  if (accessFailure) return <PatientAccessError query={query} reason={accessFailure} />;

  const token = (await cookies()).get(PATIENT_COOKIE)?.value;
  if (!token) return <PatientAccessError query={query} reason="invalid" />;
  const notice = phoneHref("notice", query);
  let loaded: Awaited<ReturnType<typeof getPatientJourney>>;
  try {
    loaded = await getPatientJourney(token);
  } catch {
    redirect(`${notice}&error=unavailable`);
  }
  if (!loaded.ok) redirect(`${notice}&error=${loaded.status === 410 ? "expired" : "invalid"}`);
  const initial = loaded.response;

  return (
    <PhoneFrame step={step} query={query} live>
      {/* Draft dictionaries reach client components only through this provider, and only on the review surface. */}
      <DictsProvider dicts={dictsForSurface(surface)}>
        {step === "notice" && <NoticeScreen lang={query.lang} journey={initial.journey} />}
        {step === "check" && <CheckFlow query={query} initial={initial.journey} />}
        {step === "book" && <BookFlow query={query} initial={initial.journey} offers={initial.offers} preview={surface === "review"} />}
        {step === "outcome" && <OutcomeHandoff lang={query.lang} />}
      </DictsProvider>
    </PhoneFrame>
  );
}
