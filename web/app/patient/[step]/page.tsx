import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { BookFlow } from "@/components/patient/book-flow";
import { CheckFlow } from "@/components/patient/check-flow";
import { isStep, parsePhoneQuery, STEPS } from "@/components/patient/journey";
import { NoticeScreen } from "@/components/patient/notice-screen";
import { PhoneFrame } from "@/components/patient/phone-frame";

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
  const query = parsePhoneQuery(await searchParams);
  return (
    <PhoneFrame step={step} query={query}>
      {step === "notice" && <NoticeScreen lang={query.lang} />}
      {step === "check" && <CheckFlow query={query} />}
      {step === "book" && <BookFlow query={query} />}
    </PhoneFrame>
  );
}
