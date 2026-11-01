import { notFound } from "next/navigation";
import { PipelineHeading } from "@/components/pipeline/heading";
import { MatchingReplay, type MatchCase } from "@/components/pipeline/matching-replay";
import { getCase, getDashboard, getFixtures } from "@/lib/api/source";
import type { FixtureName } from "@/lib/api/schema";
import { personFor } from "@/lib/people";

export const metadata = { title: "Identity matching" };

const PROVIDER: Record<string, string> = { "ahs-mushrif": "Al Mushrif", "skmc-endo": "SKMC endoscopy", "noor-endo": "Noor endoscopy" };
const providerName = (id: string) => PROVIDER[id] ?? id;

/**
 * Runner-up referrals the search would also return. The API keeps only the chosen link, so these are
 * the design's illustrative near-misses (gap G7), labelled as such on screen.
 */
const RUNNERS_UP: Record<FixtureName, MatchCase["candidates"]> = {
  evidence_closure: [
    { ar: "فاطمة الزعبي", meta: "Al Bateen · 61", similarity: 0.91 },
    { ar: "فطيم الزعابي", meta: "Al Mushrif · 47", similarity: 0.86 },
    { ar: "فاطمة الزبيدي", meta: "Al Khalidiya · 52", similarity: 0.78 },
  ],
  false_closure_trap: [
    { ar: "عيشة المنصوري", meta: "Al Bateen · 66", similarity: 0.88 },
    { ar: "عائشة المزروعي", meta: "Al Mushrif · 58", similarity: 0.79 },
    { ar: "عائشة المنهالي", meta: "Mussafah · 49", similarity: 0.74 },
  ],
  review_hold: [
    { ar: "مريم الشمسي", meta: "Al Bateen · 39", similarity: 0.9 },
    { ar: "ميرا الشامسي", meta: "Al Mushrif · 33", similarity: 0.82 },
    { ar: "مريم السامعي", meta: "Mussafah · 58", similarity: 0.71 },
  ],
};
/** Illustrative too: the top candidate's bar. The linker's real output is the calibrated probability below it. */
const TOP_SIMILARITY: Record<FixtureName, number> = { evidence_closure: 0.97, false_closure_trap: 0.95, review_hold: 0.96 };

const ORDER: FixtureName[] = ["evidence_closure", "false_closure_trap", "review_hold"];

async function matchCase(fixture: FixtureName, loop: string): Promise<MatchCase | null> {
  const detail = await getCase(loop);
  const link = detail?.links[0];
  if (!detail || !link) return null;
  const referral = personFor(link.weakest_edge.a);
  const report = personFor(link.weakest_edge.b);
  if (!referral || !report) return null;

  const reportId = link.downstream_event.split("/").at(-1)!;
  const byEid = link.weakest_edge.basis === "eid";
  const above = link.path_p_cal >= link.tau;
  const short = referral.display_name.split(" ")[0];
  const sr = loop.split("-").at(-1);

  return {
    tab: `${short} · ${sr}`,
    report: reportId,
    referralName: referral.recorded_name,
    referralMeta: `${providerName(referral.provider)} · stool test referral ${loop.split("/").at(-1)}`,
    reportName: report.recorded_name,
    reportMeta: `${providerName(report.provider)} · colonoscopy report ${reportId}`,
    p: link.path_p_cal,
    tau: link.tau,
    basis: byEid ? "Emirates ID" : "name",
    linker: link.linker,
    fields: [
      { name: "Name", referral: referral.recorded_name, report: report.recorded_name, verdict: "agrees" },
      byEid
        ? { name: "Emirates ID", referral: "on record", report: "on record", verdict: "exact" }
        : { name: "Emirates ID", referral: "on record", report: "not on report", verdict: "missing" },
      { name: "Provider", referral: providerName(referral.provider), report: providerName(report.provider), verdict: "differs" },
    ],
    candidates: [
      { ar: referral.recorded_name, meta: `${providerName(referral.provider)} · ${referral.age}`, similarity: TOP_SIMILARITY[fixture] },
      ...RUNNERS_UP[fixture],
    ],
    outcome: above
      ? {
          held: false,
          pill: "Matched",
          text: `Matched on the ${byEid ? "Emirates ID" : "name"}, ${link.path_p_cal.toFixed(2)} against the ${link.tau} line.`,
          tech: `link_decision · basis ${link.weakest_edge.basis} · path_p_cal ${link.path_p_cal} · tau ${link.tau}`,
        }
      : {
          held: false,
          pill: "Matched · second identifier",
          text: `A name alone stops at ${link.path_p_cal}, under the ${link.tau} line, so it never closes by itself. The date of birth agreed as the second identifier.`,
          tech: `link_decision · basis ${link.weakest_edge.basis} · path_p_cal ${link.path_p_cal} · tau ${link.tau} · second identifier: exact DOB`,
        },
  };
}

export default async function Page() {
  const [fx, dashboard] = await Promise.all([getFixtures(), getDashboard()]);
  const cases = (await Promise.all(ORDER.map((name) => matchCase(name, fx.fixtures[name].id)))).filter((c): c is MatchCase => c !== null);
  if (cases.length === 0) notFound();

  return (
    <MatchingReplay
      heading={
        <PipelineHeading
          title="Identity matching"
          question="Is this incoming report the same woman as one of our referrals?"
          current="/app/pipeline/matching"
        />
      }
      cases={cases}
      referrals={dashboard.by_programme.crc?.cases ?? 0}
    />
  );
}
