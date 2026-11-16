import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { isStep, STEPS } from "@/components/patient/journey";
import { PhonePage } from "@/components/patient/phone-page";

const TITLES = { notice: "Patient phone: notice", check: "Patient phone: check", book: "Patient phone: book", outcome: "Patient phone: outcome" } as const;

export function generateStaticParams() {
  return STEPS.map((step) => ({ step }));
}

export async function generateMetadata({ params }: PageProps<"/patient/[step]">): Promise<Metadata> {
  const { step } = await params;
  return { title: isStep(step) ? TITLES[step] : "Patient phone" };
}

/** The patient-facing journey: patient-approved dictionaries only (#133). */
export default async function Page({ params, searchParams }: PageProps<"/patient/[step]">) {
  const { step } = await params;
  if (!isStep(step)) notFound();
  return <PhonePage step={step} search={await searchParams} surface="patient" />;
}
