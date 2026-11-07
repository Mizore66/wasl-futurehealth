import Link from "next/link";
import { programmeView } from "@/components/programme/model";
import { exactlyOnce, fixed, missedByIdOnly, published, QUOTED, upperBound } from "@/components/site/figures";
import { LTT_SOURCE } from "@/components/site/ltt";
import { Reveal } from "@/components/site/reveal";
import { wrapExpert } from "@/components/site/styles";
import { TauPicker } from "@/components/site/tau-picker";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { getDashboard } from "@/lib/api/source";
import { result, results } from "@/lib/results";
import { cn } from "@/lib/utils";

export const metadata = { title: "For experts" };

const h2x = "m-0 text-[clamp(2rem,1.5rem_+_1.5vw,2.5rem)] font-bold leading-[1.1] tracking-[-0.025em] text-balance";
const h3x = "m-0 text-[20px] font-semibold leading-[1.3] tracking-[-0.01em]";
const muted = "text-(--muted)";
const sectionX = "py-20 md:py-[120px]";
const big = "text-[clamp(2.75rem,2rem_+_2.5vw,4rem)] font-semibold leading-[1.05] tracking-[-0.035em] tabular-nums";
const ctaX =
  "inline-block whitespace-nowrap rounded bg-(--ink) px-6 py-4 text-[16px] font-semibold text-(--paper) no-underline hover:bg-(--teal) hover:text-(--paper)";

/** A responsive table: real table from md up, stacked cards below, each cell labelled from `data-label`. */
const tbl = {
  table: "mt-10 w-full border-collapse text-left text-[16px] leading-[1.45] max-md:block",
  thead: "max-md:sr-only",
  headRow: "border-b-2 border-(--ink)",
  th: "px-4 pb-3 align-bottom text-[14px] font-semibold first:pl-0 last:pr-0",
  tbody: "max-md:block",
  row: (i: number) => cn("align-top max-md:mb-2 max-md:block max-md:p-4", i % 2 === 0 && "bg-(--card)"),
  cell: "p-4 max-md:block max-md:px-0 max-md:py-1.5 max-md:before:block max-md:before:text-[14px] max-md:before:font-semibold max-md:before:text-(--muted) max-md:before:content-[attr(data-label)]",
};

type Mark = "built" | "simulated" | "designed";

/** Status as shape plus word: filled square built, half square simulated, dashed outline designed. */
function StatusLine({ mark, children }: { mark: Mark; children: React.ReactNode }) {
  return (
    <span className="flex items-center gap-2 [&+&]:mt-1.5" data-status={mark}>
      <svg aria-hidden width="12" height="12" viewBox="0 0 12 12" className="flex-none">
        {mark === "built" && <rect x="1" y="1" width="10" height="10" fill="var(--teal)" />}
        {mark === "simulated" && (
          <>
            <rect x="1.5" y="1.5" width="9" height="9" fill="none" stroke="var(--teal)" strokeWidth="1.5" />
            <rect x="1.5" y="1.5" width="4.5" height="9" fill="var(--teal)" />
          </>
        )}
        {mark === "designed" && <rect x="1.5" y="1.5" width="9" height="9" fill="none" stroke="var(--muted)" strokeWidth="1.5" strokeDasharray="2 2" />}
      </svg>
      {children}
    </span>
  );
}

const B = ({ children }: { children: React.ReactNode }) => <strong className="font-semibold">{children}</strong>;

function Bar({ label, value, width, tone, strong }: { label: string; value: string; width: number; tone: string; strong?: boolean }) {
  return (
    <div className="mt-5 first:mt-7">
      <div className="flex justify-between text-[16px]">
        <span className={cn(strong && "font-semibold")}>{label}</span>
        <span className={cn("font-semibold tabular-nums", strong && "text-(--teal)")}>{value}</span>
      </div>
      <div className="mt-2 h-7 rounded-xs bg-(--sand)">
        <div className={cn("h-7 rounded-xs", tone)} style={{ width: `${width}%` }} />
      </div>
    </div>
  );
}

export default async function Page() {
  const dash = await getDashboard();
  const kpi = dash.metrics.crc_kpi_31d;
  const drop = dash.dropoff;
  const n = kpi.truth.eligible_referrals.toLocaleString("en-US");
  const eid = result("kpi31.demo.eid_only");
  const wasl = result("kpi31.demo.wasl");
  const truth = result("kpi31.demo.truth");
  const pct = (v: number) => (v / 0.4) * 100;
  const fc = result("v2.safety.crc_false_closures");
  const once = exactlyOnce();
  const chaseOneIn = programmeView(dash).chaseOneIn;

  const OUTCOMES: { outcome: string; does: string; status: React.ReactNode; evidence: React.ReactNode; stops: string }[] = [
    {
      outcome: "End-to-end case visibility",
      does: "One case per FIT+ result across providers: referral, notice, acknowledgement, booking, colonoscopy, histology, end-state. Linked without the Emirates ID by an Arabic–Latin name encoder and a calibrated linker.",
      status: <StatusLine mark="built">Built</StatusLine>,
      evidence: (
        <>
          <B>
            31-day KPI error {fixed("v2.kpi_abs_error_wasl", 3)}
          </B>{" "}
          vs {fixed("v2.kpi_abs_error_eid_only", 3)} for ID-only tracking. Link pair F1 {fixed("v2.linkage_pair_f1", 3)}. Drop-off state
          matches hidden truth in 96.6% of CRC loops (integrated run).
        </>
      ),
      stops: "Treatment initiation is not a tracked stage. The loop ends at its end-state: normal, surveillance, MDT, specialist or repeat.",
    },
    {
      outcome: "Clear ownership and timely progression",
      does: "A named owner on every case. SLA clocks: referral to colonoscopy in 31 days (DoH spec), histology in 10. Overdue, rejected and unowned cases are filtered and escalate from coordinator to medical director.",
      status: <StatusLine mark="built">Built, in the API</StatusLine>,
      evidence: (
        <>
          Drop-off reason on every open case. One demo run: <B>{String(drop.no_acknowledgement)}</B> no acknowledgement,{" "}
          <B>{String(drop.not_booked)}</B> not booked, <B>{String(drop.referral_unanswered)}</B> referral unanswered,{" "}
          <B>{String(drop.referral_rejected)}</B> rejected.
        </>
      ),
      stops: "The 10-day histology, acknowledgement and reply deadlines are our configurable defaults. The coordinator worklist interface is in progress.",
    },
    {
      outcome: "Patient-centred communication and reassurance",
      does: "Owner-named notices in the patient’s language. A teach-back reply parsed for understanding, with a nurse callback when it fails. A consent hold: no message leaves until contact permission is granted.",
      status: (
        <>
          <StatusLine mark="built">Built</StatusLine>
          <StatusLine mark="designed">Patient measures designed</StatusLine>
        </>
      ),
      evidence: (
        <>
          <B>{result("v2.safety.released_without_granted_consent").display}</B> messages sent without granted consent. Teach-back parser 98 /
          100, 0 / 50 missed escalations (one non-clinician labeller).
        </>
      ),
      stops: "Non-English notices are machine-drafted. Understanding and reassurance are not yet measured in patients.",
    },
    {
      outcome: "Cross-provider consultation and coordination",
      does: "Structured e-consult between units. The receiving unit accepts, accepts with conditions, redirects or rejects; silence escalates. A rejection returns responsibility to the referrer with a hand-off packet and an act-by date (DoH PHC 4.9.5).",
      status: (
        <>
          <StatusLine mark="built">Built</StatusLine>
          <StatusLine mark="simulated">Referrers simulated</StatusLine>
        </>
      ),
      evidence: (
        <>
          <B>
            {result("v2.counts.handoff_packets_matched").display} of {result("v2.counts.handoff_truth_packets").display}
          </B>{" "}
          hand-off packets produced, 0 extra. {result("v2.safety.handoff_false_closures").display} false closures after re-referral.
        </>
      ),
      stops: "Patient-to-clinician consultation is mostly designed. There is no clinician messaging interface yet.",
    },
    {
      outcome: "Safe, human-governed escalation",
      does: "A loop closes automatically only on a trusted link and two agreeing readers. Everything else goes to double human review. Suspected malignancy never waits for histology.",
      status: <StatusLine mark="built">Built</StatusLine>,
      evidence: (
        <>
          <B>{fc.display}</B> false closures (95% upper bound {upperBound("v2.safety.crc_false_closures")}), precision{" "}
          {fixed("v2.crc_precision", 3)}, recall {fixed("v2.crc_recall", 3)}. Double review 0 /{" "}
          {result("double_review.false_closures_denominator").display}; single review gives {result("single_review.false_closures").display}.
        </>
      ),
      stops: "No urgent-case triage view yet. Urgency reaches staff through escalation states and SLA clocks.",
    },
    {
      outcome: "Closure and patient impact measures",
      does: "The 31-day KPI measured from linked records, with drop-off reasons by provider and by name origin, small cells suppressed. Queue levers and network pooling run as decision support.",
      status: (
        <>
          <StatusLine mark="built">Measurement built</StatusLine>
          <StatusLine mark="simulated">Uplift simulated</StatusLine>
        </>
      ),
      evidence: (
        <>
          <B>
            {eid.display} → {wasl.display}
          </B>{" "}
          (truth {truth.display}). At 1.2 slots per FIT+: queue levers +13 pp; pooling 22.2% → 42.1%, both simulated.
        </>
      ),
      stops: "Measurement alone does not raise attendance. Uplift sizes are borrowed from published trials until the stepped-wedge pilot.",
    },
  ];

  const DEPLOY: { component: string; measured: React.ReactNode; status: string }[] = [
    {
      component: "HL7 v2 over MLLP, with drops and re-sends",
      measured: `${result("hl7.resources_exactly_once").display} / ${result("hl7.resources_exactly_once").display} delivered exactly once; 888 re-sends absorbed; CRC 0 / 574 false closures`,
      status: "Built",
    },
    {
      component: "Streaming under process and broker kills",
      measured: (
        <span data-figure="exactly-once">
          {once.display} outputs duplicated or lost, held-out runs
        </span>
      ),
      status: "Built",
    },
    {
      component: "Hash-chained ledger",
      measured: `Tampering detected ${result("v2.safety.tamper_detected").display}; ${result("v2.safety.tamper_false_alarms").display} false alarms on clean checks`,
      status: "Built",
    },
    { component: "Role-based sign-off and consent", measured: `${result("v2.safety.forged_with_effect").display} forged events took effect`, status: "Built" },
    {
      component: "Arabic–Latin name encoder",
      measured: `2.3 MB, in-process, no external model call; R@10 ${result("encoder.recall_at_10").display} vs ${result("encoder.recall_at_10_best_single").display} best off-the-shelf`,
      status: "Built",
    },
    { component: "UAE-sovereign hosting, in-country keys, WORM anchor store", measured: "No production tenant exists yet", status: "Designed" },
    { component: "DoH Responsible AI control map", measured: "Self-classified High risk; hazard log; model cards", status: "Documented" },
  ];

  // One value per scored field, from docs/labelling/agreement.json via results.json; the claim is made only if they all agree.
  const KAPPAS = [...new Set(["caecum", "prep", "polyps", "histology", "state"].map((f) => result(`labelling.kappa.${f}`).display))];
  if (KAPPAS.length !== 1) throw new Error("labelling kappa differs between fields; reword the Experts limit");

  const LIMITS = [
    { limit: "Patients, reviewers, noise and capacity are simulated. Wasl has no clinical effectiveness result from real patients.", resolves: "Shadow pilot scored against the coordinator’s own list. Gate: 0 false closures, recall ≥ 0.95.", when: "Months 0–3" },
    {
      limit: `The ${result("labelling.reports").display} public report transcripts have clinically verified model-assisted labels: two model passes, then clinician adjudication. κ = ${KAPPAS[0]} on all five scored fields (caecum, prep, polyps, histology, state) is agreement between those two passes after adjudication. It is not independent human double-labelling or clinical ground truth, and 43 reports is a small sample.`,
      resolves: "Independent human labelling of a larger, locally sourced set, scored in the shadow pilot against the coordinator’s own list.",
      when: "Months 0–3",
    },
    { limit: "The Learn-then-Test certificate covers synthetic loops only.", resolves: "Recalibrate on about 3.7k locally adjudicated loops before any automatic closure.", when: "Months 0–3" },
    { limit: "Readers broke on 3 of 7 unseen corpora before the fixes.", resolves: "300 confirmed closures in a row, then a 10% audit; an error returns that class to shadow mode.", when: "Months 0–9" },
    { limit: "The image reader was trained on Norwegian video frames; its weights inherit ImageNet’s non-commercial terms.", resolves: "It stays hold-only. Local recalibration on UAE stills; the licence question stays open until then.", when: "Before production" },
    { limit: "Queue-lever effect sizes are borrowed from published trials; App 9 fields are synthetic.", resolves: "Stepped-wedge primary endpoint. Gate: lower bound of the uplift above 0.", when: "Months 4–9" },
    { limit: "Review load across name origins: 1.08 pooled and 1.21 per seed (1.09–1.34), against a 1.25 limit; two of five single seeds are above it.", resolves: "Reported in every run. Placeholder birth dates (25% of Arab vs 6% of Western persons) still send more cases to a person on purpose: reviewer time, not accuracy.", when: "Every run" },
    { limit: "Female-list access gap 12.6 pp, against a 5 pp limit (simulated).", resolves: "About 30–40% of lists must be female lists. Wasl reports the staffing shortfall; it cannot fix it.", when: "Reported to DoH" },
    { limit: "Below about 1 colonoscopy slot per FIT+, no software raises completion.", resolves: "Wasl quantifies the shortfall by region. Pooling helps only when capacity is misallocated.", when: "Reported to DoH" },
    { limit: "Non-English notices are machine-drafted.", resolves: "Professional translation and back-translation.", when: "Before live outreach" },
    { limit: "No real Malaffi feed has been accessed.", resolves: "DDGO assessment alongside the shadow pilot, against the adapter already built for HL7 v2.5.1.", when: "Months 0–3" },
  ];

  const PLAN = [
    {
      when: "Months 0–3",
      tone: "bg-(--mint) text-(--ink)",
      title: "Unpaid shadow pilot",
      text: "One PHC to one endoscopy unit, via the Future Health pathway. A clinician confirms every automatic closure. DDGO assessment runs in parallel.",
      gate: "0 false closures; recall ≥ 0.95 against the coordinator’s list.",
      gateLabel: "Gate",
    },
    {
      when: "Months 4–9",
      tone: "bg-(--teal) text-(--paper)",
      title: "Live outreach, stepped-wedge across 6–8 PHCs",
      text: `Power 0.81–0.94. Automatic closure resumes only after ${QUOTED.pilotStreak.value} confirmed in a row, then a 10% random clinician audit. Any confirmed error returns that phrasing class to shadow mode and restarts the count.`,
      gate: "lower bound of the uplift above 0; 0 audited false closures.",
      gateLabel: "Gate",
    },
    {
      when: "Months 9–12",
      tone: "bg-(--ink) text-(--paper)",
      title: "Contract",
      text: "Primary route an ADHDS vendor agreement; fallback a DoH limited tender. Second door: on-premise for one provider network (designed).",
      gate: "contract signed.",
      gateLabel: "Gate",
    },
    {
      when: "Month 12 on",
      tone: "bg-(--sand-deep) text-(--ink)",
      title: "Cervical, then breast",
      text: "Same engine, same gate. Each new loop type repeats shadow mode before it closes anything.",
      gate: "fixed cost of AED 0.54–1.39M in year 1 falls toward AED 5–28 per loop.",
      gateLabel: "Gate, projected",
    },
  ];

  return (
    <>
      <header id="top" className={cn(wrapExpert, "pb-20 pt-10 md:pb-[104px] md:pt-14")}>
        <h1 className="m-0 max-w-[16em] text-[clamp(2.25rem,4.6vw,4rem)] font-bold leading-[1.04] tracking-[-0.03em] text-balance">
          Every FIT-positive followed to documented closure
        </h1>
        <div className="mt-12 grid items-start gap-16 lg:grid-cols-2">
          <div>
            <p className="m-0 max-w-[30em] text-[20px] leading-normal">
              Wasl links the colonoscopy to its FIT+ referral across providers without the Emirates ID, reads the report with two independent
              readers, and closes the loop only under a certified gate. Every other loop goes to two human reviewers.
            </p>
            <div className="mt-8 flex flex-wrap items-center gap-7">
              <Link href="/app" className={ctaX}>
                Open the dashboard
              </Link>
              <a href="#safety" className="text-[16px] font-semibold text-(--teal) underline underline-offset-[0.18em] hover:text-(--ink)">
                Read the safety case
              </a>
            </div>
            <p className={cn("m-0 mt-10 max-w-[36em] text-[14px] leading-[1.55]", muted)}>
              <HonestyLabel kind="simulated" className="inline h-auto whitespace-normal bg-transparent px-0 text-[14px] font-normal text-(--muted) [&>[data-dot]]:hidden">
                Status words used on this page.
              </HonestyLabel>{" "}
              <strong className="font-semibold text-(--ink)">Built</strong>: ran in the integrated pipeline, on synthetic cohorts.{" "}
              <strong className="font-semibold text-(--ink)">Simulated</strong>: a modelled mechanism on assumed inputs.{" "}
              <strong className="font-semibold text-(--ink)">Projected</strong>: modelled from published effect sizes.{" "}
              <strong className="font-semibold text-(--ink)">Designed</strong>: specified, not built. Figures come from the frozen evaluation
              at commit {results.frozen_revision.slice(0, 7)} (5 seeds, held-out noise)
              unless named otherwise.
            </p>
          </div>
          <figure
            role="img"
            aria-label={`Colonoscopy within 31 days of referral, simulated cohort of ${n}: Emirates ID only tracking ${eid.display}, Wasl ${wasl.display}, truth ${truth.display}`}
            className="m-0 rounded bg-(--card) px-[clamp(1.25rem,2.5vw,2.25rem)] py-8"
          >
            <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
              <span className="text-[20px] font-semibold tracking-[-0.01em]">Colonoscopy within 31 days of referral</span>
              <span className={cn("whitespace-nowrap text-[14px]", muted)} data-figure="cohort">
                Simulated, n = {n}
              </span>
            </div>
            <Bar label="Tracked by Emirates ID only" value={eid.display} width={pct(eid.value)} tone="bg-(--stub)" />
            <Bar label="Measured by Wasl" value={wasl.display} width={pct(wasl.value)} tone="bg-(--teal)" strong />
            <Bar label="Truth, from the simulator" value={truth.display} width={pct(truth.value)} tone="bg-(--ink)" />
            <figcaption className={cn("mt-6 text-[14px] leading-[1.55]", muted)}>
              Bars run from 0 to 40%. Under ID-only tracking {Math.round(missedByIdOnly() * 100)}% of real completions look lost
              {chaseOneIn !== null && <>, and about 1 in {chaseOneIn} on the chase list is already done</>}. Published Abu Dhabi baseline: {QUOTED.baselineRate.value} of FIT+ patients had a
              colonoscopy, median {published("crc_followup_median_wait_days")} days, against a {published("crc_followup_standard_days")}-day standard and a target above 90%
              (SEHA, BMC Cancer 2021).
            </figcaption>
          </figure>
        </div>
      </header>

      <section id="outcomes" aria-labelledby="h-outcomes" className={cn(wrapExpert, "scroll-mt-4 pb-20 pt-4 md:pb-[120px]")}>
        <Reveal>
          <div className="grid items-end gap-x-16 gap-y-4 md:grid-cols-2">
            <h2 id="h-outcomes" className={h2x}>
              The six desired outcomes, answered
            </h2>
            <p className="m-0 max-w-[34em] text-[16px] leading-[1.55]">
              Challenge #5 names six outcomes. Each row says what Wasl does, its status, the measured figure, and where it stops today.
            </p>
          </div>
          <table className={tbl.table} data-testid="outcomes">
            <thead className={tbl.thead}>
              <tr className={tbl.headRow}>
                <th scope="col" className={cn(tbl.th, "w-[16%]")}>Desired outcome</th>
                <th scope="col" className={cn(tbl.th, "w-[29%]")}>What Wasl does</th>
                <th scope="col" className={cn(tbl.th, "w-[12%]")}>Status</th>
                <th scope="col" className={cn(tbl.th, "w-[22%]")}>Evidence</th>
                <th scope="col" className={cn(tbl.th, "w-[21%]")}>Where it stops</th>
              </tr>
            </thead>
            <tbody className={tbl.tbody}>
              {OUTCOMES.map((o, i) => (
                <tr key={o.outcome} className={tbl.row(i)}>
                  <th scope="row" className={cn(tbl.cell, "font-semibold max-md:text-[18px] max-md:before:hidden")}>
                    {o.outcome}
                  </th>
                  <td className={tbl.cell} data-label="What Wasl does">{o.does}</td>
                  <td className={cn(tbl.cell, "text-[14px]")} data-label="Status">{o.status}</td>
                  <td className={tbl.cell} data-label="Evidence">{o.evidence}</td>
                  <td className={cn(tbl.cell, "text-[14px]", muted)} data-label="Where it stops">{o.stops}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Reveal>
      </section>

      <section id="safety" aria-labelledby="h-safety" className="scroll-mt-4 bg-(--sand)">
        <Reveal className={cn(wrapExpert, sectionX)}>
          <h2 id="h-safety" className={h2x}>
            The gate refuses to guess
          </h2>
          <div className="mt-12 grid items-start gap-16 lg:grid-cols-[1.1fr_1fr] lg:gap-20">
            <div>
              <p className="m-0 max-w-[30em] text-[20px] leading-normal">
                Closure is the one action a programme cannot take back. Wasl takes it automatically only when every condition below holds,
                and sends every other loop to two people.
              </p>
              <ul className="m-0 mt-9 grid list-none gap-6 p-0 text-[16px] leading-[1.55] text-(--ink-soft)">
                <li>
                  <strong className="font-semibold text-(--ink)">A trusted link.</strong> Calibrated link probability at τ 0.99 or above, set
                  over the 0.989 cap on name-only links, so a name match alone never closes a loop.
                </li>
                <li>
                  <strong className="font-semibold text-(--ink)">Two readers agree, field by field.</strong> DoH rules and a Gemma reader each
                  read the report: caecum documented, preparation graded, histology back if tissue was taken. A reader failure or timeout
                  routes to review.
                </li>
                <li>
                  <strong className="font-semibold text-(--ink)">A second identifier before any message.</strong> An exact birth date, or a
                  matching Emirates ID with agreeing name, sex and birth date, or a human confirmation.
                </li>
                <li>
                  <strong className="font-semibold text-(--ink)">Images hold, never close.</strong> The image reader (caecum AUC 0.998, prep
                  AUC 0.991) can hold a loop or send it for a one-click endoscopist grade.
                </li>
                <li>
                  <strong className="font-semibold text-(--ink)">Suspected malignancy</strong> is an end-state that never waits for histology.
                </li>
              </ul>
            </div>
            <div>
              <div className="text-[16px] text-(--ink-soft)">False closures, held-out noise, 5 seeds</div>
              <div className={cn(big, "mt-2")} data-figure="false-closures">
                {fc.display}
              </div>
              <p className="m-0 mt-3 max-w-[30em] text-[16px] leading-[1.55] text-(--ink-soft)">
                95% upper bound {upperBound("v2.safety.crc_false_closures")}. Precision {fixed("v2.crc_precision", 3)}, recall{" "}
                {fixed("v2.crc_recall", 3)}. With no human review recall is {fixed("v2.crc_recall_no_review", 3)}: people sign the rest.
              </p>
              <div className="mt-12 text-[16px] text-(--ink-soft)">False closures in the offline gate sweep, 5 seeds × 3 reviewer error rates</div>
              <div className="mt-3 grid grid-cols-2 gap-6">
                <div className="rounded bg-(--card) px-6 py-5">
                  <div className={big}>0</div>
                  <div className="mt-1.5 text-[16px]">Double review, of {result("double_review.false_closures_denominator").display}</div>
                </div>
                <div className="rounded bg-(--card) px-6 py-5">
                  <div className={cn(big, "text-(--negative)")}>{result("single_review.false_closures").display}</div>
                  <div className="mt-1.5 text-[16px]">Single review</div>
                </div>
              </div>
              <p className="m-0 mt-3 text-[16px] leading-[1.55] text-(--ink-soft)">So double review is mandatory, not a setting.</p>
              <ul className="m-0 mt-10 grid list-none gap-2.5 p-0 text-[16px] tabular-nums">
                <li>
                  <B>{result("v2.safety.wrong_patient_released").display}</B> <span className="text-(--ink-soft)">wrong-patient messages</span>
                </li>
                <li>
                  <B>{result("v2.safety.lung_false_closures").display}</B>{" "}
                  <span className="text-(--ink-soft)">lung-nodule false closures, same engine</span>
                </li>
                <li>
                  <B>{result("v2.safety.cervical_false_closures").display}</B>{" "}
                  <span className="text-(--ink-soft)">cervical false closures, simulated programme</span>
                </li>
              </ul>
            </div>
          </div>
        </Reveal>
      </section>

      <section aria-labelledby="h-cert" className="bg-(--sand)">
        <Reveal className={cn(wrapExpert, "pb-20 md:pb-[120px]")}>
          <h2 id="h-cert" className={h2x}>
            Certified, then attacked
          </h2>
          <div className="mt-12 grid items-start gap-16 lg:grid-cols-[1.1fr_1fr] lg:gap-20">
            <div>
              <p className="m-0 max-w-[36em] text-[16px] leading-[1.55]">
                Learn-then-Test on {result("ltt.calibration_loops").display} calibration loops, disjoint from the evaluation cohort, held-out
                noise. False closures and wrong-patient messages are each bounded at 0.1% with probability at least 95%. Choose a closure tier.
              </p>
              <TauPicker />
              <p className={cn("m-0 mt-4 max-w-[40em] text-[14px] leading-[1.55]", muted)}>
                Scope: loops like the synthetic calibration set, whose CRC loops are 51.9% women against 69.6% in evaluation. It is not a
                guarantee on Abu Dhabi data; the pilot recalibrates on about 3.7k locally adjudicated loops. Source: {LTT_SOURCE}.
              </p>
            </div>
            <div>
              <h3 className={h3x}>Readers on text they had never seen</h3>
              <p className="m-0 mt-3 text-[16px] leading-[1.55]">
                {result("reader.corpora").display} more 300-report corpora, written by four model families and each frozen before any reader
                ran. Three of the seven found an unsafe phrasing class: 6 of 211 automatic closures by the 26B reader were wrong (2.8%, upper
                bound 6.1%), and 10 of 137 with the 4B. Each class was fixed by a generic rule. Reader v7.2 shows 0 unsafe closures on every
                corpus, and the last two fair tests passed with both readers.
              </p>
              <h3 className={cn(h3x, "mt-10")}>Changes made after a failure, disclosed</h3>
              <ul className="m-0 mt-3 grid gap-2.5 pl-5 text-[16px] leading-normal">
                <li>τ 0.98 → 0.99, after one wrong-patient closure at the name-only cap.</li>
                <li>The day-15 prep decision no longer skips pending histology, after 4 false closures in the first run at b53ce89.</li>
                <li>Short surnames corroborate a link only when the first name agrees, after 1 wrong-patient message in a run.</li>
                <li>
                  Reports that never grade prep now wait for the unit’s grade. Recall fell from 0.851 to {fixed("v2.crc_recall", 3)}.
                </li>
                <li>Surname-variant and Emirates ID identity rules: about 10% less reviewer time, safety unchanged.</li>
                <li>The MTSamples biopsy answer key was corrected and the held-out noise model fixed.</li>
              </ul>
            </div>
          </div>
        </Reveal>
      </section>

      <section id="deploy" aria-labelledby="h-deploy" className={cn(wrapExpert, sectionX, "scroll-mt-4")}>
        <Reveal>
          <h2 id="h-deploy" className={h2x}>
            Speaks Malaffi’s wire format
          </h2>
          <ol
            aria-label="Data path from provider systems to the worklist"
            className="m-0 mt-12 grid list-none gap-2 p-0 md:grid-cols-5 md:gap-9"
          >
            {[
              { t: "Provider systems", d: "PHC, endoscopy, pathology. Emirates ID optional.", hl: false },
              { t: "Malaffi", d: "HL7 v2.5.1 over MLLP. Tested against a mock feed.", hl: true },
              { t: "Ingest", d: "NATS JetStream, exactly-once decisions.", hl: false },
              { t: "Link, read, gate", d: "2.3 MB encoder in-process, two readers, v7strict.", hl: false },
              { t: "Ledger and worklist", d: "Hash-chained, role-gated sign-off.", hl: false },
            ].map((s, i, all) => (
              <li key={s.t} className={cn("relative rounded px-5 py-5", s.hl ? "bg-(--mint)" : "bg-(--sand)")}>
                <div className="text-[16px] font-semibold">{s.t}</div>
                <div className={cn("mt-1.5 text-[14px] leading-[1.45]", s.hl ? "text-(--ink-soft)" : muted)}>{s.d}</div>
                {i < all.length - 1 && (
                  <svg aria-hidden viewBox="0 0 36 12" className="absolute left-full top-1/2 h-3 w-9 -translate-y-1/2 max-md:hidden">
                    <path d="M2 6h28M25 1l6 5-6 5" fill="none" stroke="var(--teal)" strokeWidth="2" />
                  </svg>
                )}
              </li>
            ))}
          </ol>
          <div className="mt-14 grid items-start gap-12 lg:grid-cols-[1fr_1.25fr] lg:gap-[72px]">
            <div>
              <p className="m-0 text-[16px] leading-[1.55]">
                Malaffi carries HL7 v2, not FHIR. The MLLP adapter ingests ADT^A04, ORM^O01, ORU^R01, SIU^S12 and MDM^T02, acknowledges each
                message once and absorbs re-sends. A FHIR R4 Subscription path runs on the same pipeline.
              </p>
              <pre
                aria-label="Synthetic HL7 version 2 message and its acknowledgement"
                className="m-0 mt-6 whitespace-pre-wrap break-all rounded bg-(--card) px-6 py-6 font-mono text-[14px] leading-[1.6] text-(--ink)"
              >
                {`MSH|^~\\&|skmc-endo|skmc-endo|WASL|ADHIE|20261010093000||ORU^R01|SKM-dr-237|P|2.5.1
PID|1||SKM-48213^^^skmc-endo^MR||Fatima Al Zaabi||19720314|F|||||||ar
MSH|^~\\&|WASL|ADHIE|skmc-endo|skmc-endo|20261010093001||ACK|SKM-dr-237-ack|P|2.5.1
MSA|AA|SKM-dr-237|`}
              </pre>
              <p className={cn("m-0 mt-3 text-[14px] leading-[1.55]", muted)}>
                Synthetic report in the adapter’s segment layout: a Latin name, a local record number, no Emirates ID. Wasl answers MSA|AA
                once, however often it is re-sent.
              </p>
            </div>
            <table className={cn(tbl.table, "mt-0")}>
              <thead className={tbl.thead}>
                <tr className={tbl.headRow}>
                  <th scope="col" className={cn(tbl.th, "w-[34%]")}>Component</th>
                  <th scope="col" className={tbl.th}>Measured</th>
                  <th scope="col" className={cn(tbl.th, "w-[16%]")}>Status</th>
                </tr>
              </thead>
              <tbody className={tbl.tbody}>
                {DEPLOY.map((d, i) => (
                  <tr key={d.component} className={tbl.row(i)}>
                    <th scope="row" className={cn(tbl.cell, "py-3.5 font-semibold max-md:before:hidden")}>
                      {d.component}
                    </th>
                    <td className={cn(tbl.cell, "py-3.5 tabular-nums", d.status === "Designed" || d.status === "Documented" ? muted : "")} data-label="Measured">
                      {d.measured}
                    </td>
                    <td className={cn(tbl.cell, "py-3.5 text-[14px]")} data-label="Status">
                      {d.status}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {once.contested && (
            <p className={cn("m-0 mt-4 text-[14px] leading-[1.55]", muted)}>
              The exactly-once count is the frozen evaluation’s held-out figure; the project’s sources disagree on the total, so no larger
              number is shown.
            </p>
          )}
        </Reveal>
      </section>

      <section id="plan" aria-labelledby="h-plan" className="scroll-mt-4 bg-(--sand)">
        <Reveal className={cn(wrapExpert, sectionX)}>
          <div className="grid items-end gap-x-16 gap-y-4 md:grid-cols-2">
            <h2 id="h-plan" className={h2x}>
              Four stages, each with a numeric gate
            </h2>
            <p className="m-0 max-w-[34em] text-[16px] leading-[1.55]">
              The pilot switches already run in the pipeline. In a simulated shadow pilot the {QUOTED.pilotStreak.value}-in-a-row streak was
              reached on day 120 of the demo cohort.
            </p>
          </div>
          <ol aria-label="Pilot stages by month" className="m-0 mt-12 grid list-none gap-2 p-0 max-md:gap-8 md:grid-cols-[3fr_6fr_3fr_3fr]">
            {PLAN.map((p) => (
              <li key={p.when} data-stage={p.when}>
                <div className={cn("rounded px-[18px] py-4 text-[16px] font-semibold", p.tone)}>{p.when}</div>
                <div className="pr-[18px] pt-5">
                  <div className="text-[20px] font-semibold leading-[1.3]">{p.title}</div>
                  <p className="m-0 mt-2 max-w-[32em] text-[16px] leading-normal">{p.text}</p>
                  <p className="m-0 mt-4 text-[16px] leading-normal">
                    <strong className="font-semibold">{p.gateLabel}:</strong> {p.gate}
                  </p>
                </div>
              </li>
            ))}
          </ol>
        </Reveal>
      </section>

      <section id="limits" aria-labelledby="h-limits" className={cn(wrapExpert, sectionX, "scroll-mt-4")}>
        <Reveal>
          <div className="grid items-end gap-x-16 gap-y-4 md:grid-cols-2">
            <h2 id="h-limits" className={h2x}>
              Every limit, paired with its gate
            </h2>
            <p className="m-0 max-w-[34em] text-[16px] leading-[1.55]">
              What the evidence above does not show yet, what would settle each point, and when.
            </p>
          </div>
          <table className={tbl.table}>
            <thead className={tbl.thead}>
              <tr className={tbl.headRow}>
                <th scope="col" className={cn(tbl.th, "w-[40%]")}>Limit</th>
                <th scope="col" className={cn(tbl.th, "w-[42%]")}>What resolves it</th>
                <th scope="col" className={cn(tbl.th, "w-[18%]")}>When</th>
              </tr>
            </thead>
            <tbody className={tbl.tbody}>
              {LIMITS.map((l, i) => (
                <tr key={l.limit} className={tbl.row(i)}>
                  <th scope="row" className={cn(tbl.cell, "font-normal max-md:before:hidden")}>
                    {l.limit}
                  </th>
                  <td className={tbl.cell} data-label="What resolves it">{l.resolves}</td>
                  <td className={cn(tbl.cell, "font-semibold")} data-label="When">{l.when}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Reveal>
      </section>

      <section id="pilot" aria-labelledby="h-pilot" className="scroll-mt-4 bg-(--sand-deep)">
        <Reveal className={cn(wrapExpert, "grid items-start gap-16 py-20 md:py-28 lg:grid-cols-[1.2fr_1fr] lg:gap-20")}>
          <div>
            <h2 id="h-pilot" className={h2x}>
              One false closure ends the pilot
            </h2>
            <p className="m-0 mt-6 max-w-[28em] text-[20px] leading-normal">
              A free three-month shadow pilot: one primary care clinic, one endoscopy unit, a clinician confirming every closure, and your
              coordinator’s list as the reference.
            </p>
            <p className="m-0 mt-8 text-[20px] font-semibold leading-normal">To request one, write to [team email].</p>
          </div>
          <div>
            <h3 className={h3x}>What the pilot needs from you</h3>
            <ul className="m-0 mt-4 grid gap-3 pl-5 text-[16px] leading-normal">
              <li>One PHC and one endoscopy unit willing to run in shadow mode.</li>
              <li>The coordinator’s FIT+ follow-up list for months 0–3, as the reference.</li>
              <li>A route to Malaffi test messages and to the DDGO assessment.</li>
              <li>Two clinicians to sign held loops, because double review is mandatory.</li>
            </ul>
          </div>
        </Reveal>
      </section>

      <footer className={cn(wrapExpert, "grid items-start gap-6 pb-12 pt-10 md:grid-cols-[auto_1fr] md:gap-16")}>
        <div className="flex items-center gap-3">
          <span className="text-[20px] font-bold">Wasl</span>
          <span lang="ar" dir="rtl" className="font-arabic text-[19px] font-semibold text-(--teal)">
            وصل
          </span>
        </div>
        <p className={cn("m-0 max-w-[62em] text-[14px] leading-[1.6] md:justify-self-end", muted)}>
          Synthetic cohort. All patients, names, messages and cases shown are synthetic; no outcomes on real patients are claimed. Frozen
          evaluation: code {results.frozen_revision.slice(0, 7)}, policy v7strict, seeds 11, 23, 37, 41 and 59, held-out noise model v2, frozen 30 Oct 2026. The 31-day KPI,
          drop-off counts and pilot streak come from single integrated runs; the gate sweep, reader corpora and Learn-then-Test certificate
          are reported in the project’s results files.
        </p>
      </footer>
    </>
  );
}
