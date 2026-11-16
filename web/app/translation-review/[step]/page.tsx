import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { isStep } from "@/components/patient/journey";
import { PhonePage } from "@/components/patient/phone-page";
import { translationReviewEnabled } from "@/components/patient/translation-review";

export const metadata: Metadata = {
  title: "Internal translation review preview (not for patients)",
  robots: { index: false, follow: false },
};

/**
 * Internal preview of unreviewed draft dictionaries for design and translation review (#133). Off unless the
 * deployment sets WASL_TRANSLATION_REVIEW_PREVIEW=1; every step carries the unreviewed-translation warning.
 */
export default async function Page({ params, searchParams }: PageProps<"/translation-review/[step]">) {
  const { step } = await params;
  if (!translationReviewEnabled() || !isStep(step)) notFound();
  return <PhonePage step={step} search={await searchParams} surface="review" />;
}
