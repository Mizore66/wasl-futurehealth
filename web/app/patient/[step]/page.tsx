import type { Metadata } from "next";
import { cookies } from "next/headers";
import { notFound, redirect } from "next/navigation";
import { PatientAccessError, patientAccessFailure } from "@/components/patient/access-error";
import { BookFlow } from "@/components/patient/book-flow";
import { CheckFlow } from "@/components/patient/check-flow";
import { isStep, parsePhoneQuery, STEPS } from "@/components/patient/journey";
import { NoticeScreen } from "@/components/patient/notice-screen";
import { PhoneFrame } from "@/components/patient/phone-frame";
import { getPatientJourney, PATIENT_COOKIE } from "@/lib/api/patient-source";

const TITLES = { notice: "Patient phone: notice", check: "Patient phone: check", book: "Patient phone: book" } as const;

export function generateStaticParams() {
  return STEPS.map((step) => ({ step }));
}

export async function generateMetadata({ params }: PageProps<"/patient/[step]">): Promise<Metadata> {
  const { step } = await params;
  return { title: isStep(step) ? TITLES[step] : "Patient phone" };
}

export default async function Page({ params, searchParams }: PageProps<"/patient/[step]">) {
  const { step } = await params;
  if (!isStep(step)) notFound();
  const search = await searchParams;
  const query = parsePhoneQuery(search);
  const accessFailure = patientAccessFailure(search.error);
  if (accessFailure) return <PatientAccessError query={query} reason={accessFailure} />;

  const token = (await cookies()).get(PATIENT_COOKIE)?.value;
  if (!token) return <PatientAccessError query={query} reason="invalid" />;
  let loaded: Awaited<ReturnType<typeof getPatientJourney>>;
  try {
    loaded = await getPatientJourney(token);
  } catch {
    redirect(`/patient/notice?lang=${query.lang}&error=unavailable`);
  }
  if (!loaded.ok) redirect(`/patient/notice?lang=${query.lang}&error=${loaded.status === 410 ? "expired" : "invalid"}`);
  const initial = loaded.response;

  return (
    <PhoneFrame step={step} query={query} live>
      {step === "notice" && <NoticeScreen lang={query.lang} />}
      {step === "check" && <CheckFlow query={query} initial={initial.journey} />}
      {step === "book" && <BookFlow query={query} initial={initial.journey} offers={initial.offers} />}
    </PhoneFrame>
  );
}
