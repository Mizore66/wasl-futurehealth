import { PageHeader } from "@/components/shell/page-header";
import { SERIES } from "@/components/charts/palette";
import { BoundChart, CountBars, IntervalChart, KpiChart, UnitChart, type Bound, type Kpi } from "@/components/results/charts";
import { ConsentLifecycle } from "@/components/results/consent-lifecycle";
import { getDashboard } from "@/lib/api/source";
import { result } from "@/lib/results";

export const metadata = { title: "Results" };

const card = "bg-surface px-pad py-5";
const fmt = (n: number) => n.toLocaleString("en-US");

function CardHead({ id, title }: { id: string; title: string }) {
  return (
    <h2 id={id} className="m-0 text-section font-medium">
      {title}
    </h2>
  );
}

function FalseClosures() {
  const crc = result("v2.safety.crc_false_closures");
  const recall = result("v2.crc_recall");
  const wrongPatient = result("v2.safety.wrong_patient_released");
  const noConsent = result("v2.safety.released_without_granted_consent");
  const bound = (e: typeof crc, label: string, testId: string): Bound => ({
    label,
    events: e.value,
    denominator: e.denominator ?? 0,
    upper: parseFloat(e.upper95 ?? "0") / 100,
    testId,
  });
  const [low, high] = recall.ci95 ?? [recall.value, recall.value];
  return (
    <section aria-labelledby="safe-h" className={card}>
      <CardHead id="safe-h" title="No wrong closures, and the likely rate stays near zero" />
      <div className="mt-3 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span data-testid="lead-figure" className="whitespace-nowrap text-[52px] font-semibold leading-[56px] tracking-[-0.02em]">
          {crc.value} of {fmt(crc.denominator ?? 0)}
        </span>
        <span className="text-body text-ink-soft">CRC closures wrong after review, across 5 runs</span>
      </div>
      <div className="mt-4">
        <BoundChart
          rows={[
            bound(crc, "CRC false closures", "bound-crc"),
            bound(wrongPatient, "Wrong-patient messages", "bound-wrong-patient"),
            bound(noConsent, "Sent without consent", "bound-consent"),
          ]}
        />
      </div>
      <h3 className="m-0 mt-5 text-body font-medium">About 1 in {Math.round(1 / (1 - recall.value))} finished colonoscopies stays open for review</h3>
      <div className="mt-2">
        <IntervalChart label="Finished CRC cases closed" value={recall.value} low={low} high={high} />
      </div>
    </section>
  );
}

function Kpi() {
  const eid = result("kpi31.heldout.eid_only");
  const wasl = result("kpi31.heldout.wasl");
  const truth = result("kpi31.heldout.truth");
  const errWasl = (result("v2.kpi_abs_error_wasl").value * 100).toFixed(1);
  const errEid = (result("v2.kpi_abs_error_eid_only").value * 100).toFixed(1);
  const demo = { eid: result("kpi31.demo.eid_only"), wasl: result("kpi31.demo.wasl"), truth: result("kpi31.demo.truth") };
  const share = (e: typeof eid) => (e.value > 1 ? e.value / 100 : e.value);
  const groups: Kpi[] = [
    {
      label: "Unseen test data, average of 5 runs",
      eid: share(eid),
      wasl: share(wasl),
      truth: share(truth),
      eidText: eid.display,
      waslText: wasl.display,
      truthText: truth.display,
      eidCi: eid.ci95,
      waslCi: wasl.ci95,
    },
    {
      label: "This demo, 1 run",
      eid: share(demo.eid),
      wasl: share(demo.wasl),
      truth: share(demo.truth),
      eidText: demo.eid.display,
      waslText: demo.wasl.display,
      truthText: demo.truth.display,
    },
  ];
  return (
    <section aria-labelledby="kpi-h" className={card}>
      <CardHead id="kpi-h" title="The 31-day number lands near the truth" />
      <div className="mt-3 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span data-testid="kpi-error" className="whitespace-nowrap text-[30px] font-semibold leading-10">
          {errWasl} points off
        </span>
        <span data-testid="kpi-error-eid" className="text-body text-ink-soft">
          Emirates ID only: {errEid} points off
        </span>
      </div>
      <div className="mt-4">
        <KpiChart groups={groups} />
      </div>
      <p className="m-0 mt-3 flex flex-wrap gap-2 text-small">
        <span data-testid="kpi-why" className="rounded-chip bg-ochre-wash px-2 py-0.5 text-ochre-ink">
          The demo is one run on data used for tuning. Quote the unseen test average.
        </span>
      </p>
    </section>
  );
}

function Readers() {
  const corpora = result("reader.corpora");
  return (
    <section aria-labelledby="read-h" className={card}>
      <CardHead id="read-h" title={`3 of ${corpora.value} new report sets exposed an unsafe reading; each was fixed`} />
      <div className="mt-3">
        <UnitChart total={corpora.value} hit={3} label="Each set holds 300 test reports from 4 different AI writers, locked before any reader saw them" />
      </div>
      <h3 className="m-0 mt-5 text-body font-medium">Wrong automatic closures across all {corpora.value} report sets</h3>
      <div className="mt-2">
        <CountBars
          caption="Wrong closures out of automatic closures, by reader"
          tableHead={["Reader", "Wrong closures"]}
          rows={[
            { label: "Larger reader", count: 6, of: 211, color: SERIES.comparator },
            { label: "On-site reader", count: 10, of: 137, color: SERIES.comparator },
          ]}
        />
      </div>
      <p className="m-0 mt-2 flex flex-wrap gap-2 text-small">
        <span className="rounded-chip bg-jade-100 px-2 py-0.5 text-jade-800">Current reader: 0 unsafe on every set</span>
      </p>
    </section>
  );
}

function Review() {
  const double = result("double_review.false_closures_denominator");
  const single = result("single_review.false_closures");
  const ltt = result("ltt.calibration_loops");
  return (
    <section aria-labelledby="human-h" className={card}>
      <CardHead id="human-h" title="When the safety check says no, two people sign" />
      <div className="mt-3">
        <CountBars
          caption="False closures after human review, with simulated reviewers"
          tableHead={["Review", "False closures"]}
          rows={[
            { label: "Two reviewers", sub: `in ${double.display}`, count: 0, testId: "double-review" },
            { label: "One reviewer", count: single.value, testId: "single-review", color: SERIES.comparator },
          ]}
          axisMax={single.value * 1.25}
        />
      </div>
      <p className="m-0 mt-3 flex flex-wrap gap-2 text-small">
        <span className="rounded-chip bg-raised px-2 py-0.5 text-ink-soft">Double review is required</span>
        <span data-testid="ltt" className="rounded-chip bg-raised px-2 py-0.5 text-ink-soft">
          At the 0.99 auto-close line: 0 false closures in {ltt.display} test cases, at most 0.1% with 95% confidence
        </span>
      </p>
    </section>
  );
}

export default async function ResultsPage() {
  const dashboard = await getDashboard();
  return (
    <div className="overflow-clip rounded-card border border-line bg-surface">
      <PageHeader
        attached
        title="Results"
        sub="Measured on unseen test data across 5 runs · simulated patients, real report texts"
      />
      {/* The sections share the sheet; the 1px gaps show the line colour as rules between them. */}
      <div className="grid gap-px bg-line lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)]">
        <FalseClosures />
        <Kpi />
        <Readers />
        <Review />
        <ConsentLifecycle timing={dashboard.metrics.consent_timing} />
      </div>
    </div>
  );
}
