import { notFound } from "next/navigation";
import { PipelineHeading } from "@/components/pipeline/heading";
import { MatchingReplay, type MatchCase } from "@/components/pipeline/matching-replay";
import { getCase, getDashboard, getFixtures } from "@/lib/api/source";
import { CRC_FIXTURES } from "@/lib/api/schema";
import { linkScore } from "@/components/worklist/model";
import { personFor } from "@/lib/people";

export const metadata = { title: "Identity matching" };

const PROVIDER: Record<string, string> = { "ahs-mushrif": "Al Mushrif", "skmc-endo": "SKMC endoscopy", "noor-endo": "Noor endoscopy" };
const providerName = (id: string) => PROVIDER[id] ?? id;

/**
 * Runner-up referrals the search would also return. The API keeps only the chosen link, so these are
 * the design's illustrative near-misses (gap G7), labelled as such on screen.
 */
type CrcFixture = (typeof CRC_FIXTURES)[number];

const RUNNERS_UP: Record<CrcFixture, MatchCase["candidates"]> = {
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
const TOP_SIMILARITY: Record<CrcFixture, number> = { evidence_closure: 0.97, false_closure_trap: 0.95, review_hold: 0.96 };

/** What the weakest edge of the chosen link rests on, in words. A `human` edge was a below-line name match two reviewers confirmed. */
const BASIS: Record<string, string> = { eid: "Emirates ID", human: "name", wasl: "name and details", p_cal: "name and details" };

async function matchCase(fixture: CrcFixture, loop: string): Promise<MatchCase | null> {
  const detail = await getCase(loop);
  const link = detail?.links[0];
  if (!detail || !link) return null;
  const referral = personFor(link.weakest_edge.a);
  const report = personFor(link.weakest_edge.b);
  if (!referral || !report) return null;

  const reportId = link.downstream_event.split("/").at(-1)!;
  const byEid = link.weakest_edge.basis === "eid";
  const basis = BASIS[link.weakest_edge.basis] ?? link.weakest_edge.basis;
  const score = linkScore(detail, link);
  const above = score >= link.tau;
  const p = score.toFixed(3);
  const tech = `link_decision · basis ${link.weakest_edge.basis} · path_p_cal ${link.path_p_cal}${score !== link.path_p_cal ? ` · review p ${score}` : ""} · tau ${link.tau}`;
  const noDob = detail.review_items.some((r) => r.kinds.includes("link") && r.reason === "second_identifier");
  const confirmed = detail.signoffs.some((s) => s.kinds.includes("link") && s.link === "same");
  const reviewed = confirmed ? " Two reviewers later confirmed the same patient." : "";
  const short = referral.display_name.split(" ")[0];
  const sr = loop.split("-").at(-1);

  return {
    tab: `${short} · ${sr}`,
    report: reportId,
    referralName: referral.recorded_name,
    referralMeta: `${providerName(referral.provider)} · stool test referral ${loop.split("/").at(-1)}`,
    reportName: report.recorded_name,
    reportMeta: `${providerName(report.provider)} · colonoscopy report ${reportId}`,
    p: score,
    tau: link.tau,
    basis,
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
    outcome: !above
      ? {
          held: true,
          pill: "Two reviewers",
          text: `A name alone stops at ${p}, under the ${link.tau} line, so it never closes by itself.${reviewed}`,
          tech: `${tech}${confirmed ? " · human_signoff same" : ""}`,
        }
      : noDob
        ? {
            held: true,
            pill: "Second identifier",
            text: `Matched on the ${basis}, ${p} against the ${link.tau} line, but the report has no date of birth. Messages wait for a second identifier.${reviewed}`,
            tech: `${tech} · outreach_guard second_identifier${confirmed ? " · human_signoff same" : ""}`,
          }
        : { held: false, pill: "Matched", text: `Matched on the ${basis}, ${p} against the ${link.tau} line.`, tech },
  };
}

export default async function Page() {
  const [fx, dashboard] = await Promise.all([getFixtures(), getDashboard()]);
  const cases = (await Promise.all(CRC_FIXTURES.map((name) => matchCase(name, fx.fixtures[name].id)))).filter((c): c is MatchCase => c !== null);
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
