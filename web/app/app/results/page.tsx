import { HonestyLabel } from "@/components/wasl/honesty-label";
import { result, results } from "@/lib/results";

export const metadata = { title: "Results" };

const card = "rounded-card border border-line bg-surface px-pad py-5";
const fmt = (n: number) => n.toLocaleString("en-US");

function CardHead({ id, title, sub }: { id: string; title: string; sub: string }) {
  return (
    <>
      <h2 id={id} className="m-0 text-section font-medium">
        {title}
      </h2>
      <p className="m-0 text-small text-muted">{sub}</p>
    </>
  );
}

function FalseClosures() {
  const crc = result("v2.safety.crc_false_closures");
  const recall = result("v2.crc_recall");
  const wrongPatient = result("v2.safety.wrong_patient_released");
  const noConsent = result("v2.safety.released_without_granted_consent");
  const upper = parseFloat(crc.upper95 ?? "0");
  const SCALE = 0.5;
  const ubAt = `${(upper / SCALE) * 100}%`;
  const ci = recall.ci95?.map((v) => v.toFixed(3)).join("–");

  return (
    <section aria-labelledby="safe-h" className={card}>
      <CardHead id="safe-h" title="False closures" sub="A case Wasl closed that was not really finished. CRC, counted after review." />
      <div className="mt-3 flex flex-wrap items-baseline gap-3">
        <span data-testid="lead-figure" className="whitespace-nowrap text-lead font-bold tracking-[-0.02em]">
          {crc.value} of {fmt(crc.denominator ?? 0)}
        </span>
        <span className="text-body text-muted">closures were wrong, across 5 seeds</span>
      </div>

      <div className="mt-5">
        <div className="text-small text-muted">How high could the true rate be? 95% upper bound, all seeds pooled</div>
        <div
          role="img"
          aria-label={`Observed false-closure rate 0%. 95% upper bound ${upper.toFixed(2)}%, on a scale from 0% to ${SCALE}%.`}
          className="relative mt-1.5 h-[60px]"
        >
          <div className="absolute inset-x-0 top-[9px] h-0.5 bg-rule" />
          <div className="absolute left-0 top-2 h-1 bg-ink" style={{ width: ubAt }} />
          <div className="absolute top-[3px] -ml-px h-3.5 w-0.5 bg-ink" style={{ left: ubAt }} />
          <span className="absolute left-0 top-1 box-border size-3 rounded-full border-2 border-surface bg-ink" />
          <span className="absolute left-0 top-[22px] text-small text-muted">0% observed</span>
          <span className="absolute top-[42px] -ml-px whitespace-nowrap text-small font-medium" style={{ left: ubAt }}>
            {upper.toFixed(2)}% upper bound
          </span>
          <span className="absolute right-0 top-[22px] text-small text-muted">{SCALE}%</span>
        </div>
      </div>

      <p className="m-0 mt-4 text-body">
        <span className="font-medium">
          Recall {recall.value.toFixed(3)} ({ci}).
        </span>{" "}
        About 1 in {Math.round(1 / (1 - recall.value))} finished colonoscopies stays open, because the gate refuses to guess when a report
        doesn&apos;t grade the prep. It waits for the unit&apos;s grade.
      </p>
      <p className="m-0 mt-1 text-small text-muted">
        Also {wrongPatient.value} of {fmt(wrongPatient.denominator ?? 0)} wrong-patient messages and {noConsent.value} of{" "}
        {fmt(noConsent.denominator ?? 0)} sent without consent, pooled.
      </p>
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
  const AXIS = 0.4;
  const x = (v: number) => `${(v / AXIS) * 100}%`;
  const ticks = [0, 0.1, 0.2, 0.3, 0.4];
  const rows = [
    { label: "Emirates ID only", e: eid, bar: "bg-unknown", top: 30 },
    { label: "Wasl", e: wasl, bar: "bg-hue", top: 72 },
  ];

  return (
    <section aria-labelledby="kpi-h" className={card}>
      <CardHead id="kpi-h" title="The 31-day number" sub="Share of referrals with a colonoscopy within 31 days. The cohort is synthetic, so the truth is known." />
      <h3 data-testid="kpi-heldout-h" className="m-0 mt-4 text-body font-medium">
        Five-seed held-out mean
      </h3>
      <div className="mt-1 flex flex-wrap items-baseline gap-3">
        <span data-testid="kpi-error" className="whitespace-nowrap text-[30px] font-medium leading-10">
          {errWasl} points off
        </span>
        <span className="text-body text-muted">Emirates ID only: {errEid} points off · mean of 5 seeds</span>
      </div>

      <div
        role="img"
        aria-label={`Within 31 days, held-out noise: Emirates ID only ${eid.display}, Wasl ${wasl.display}, truth ${truth.display}.`}
        className="relative mt-5 h-[136px]"
      >
        {ticks.map((t) => (
          <div key={t}>
            <div className={`absolute bottom-5 top-6 w-px ${t === 0 ? "bg-control" : "bg-rule"}`} style={{ left: x(t) }} />
            <span
              className={`absolute bottom-0 text-small text-muted ${t === 0 ? "" : t === AXIS ? "" : "-translate-x-1/2"}`}
              style={t === AXIS ? { right: 0 } : { left: x(t) }}
            >
              {Math.round(t * 100)}%
            </span>
          </div>
        ))}
        <div className="absolute bottom-5 top-[18px] w-0 border-l border-dashed border-ink" style={{ left: x(truth.value) }} />
        <span className="absolute top-0 -translate-x-1/2 whitespace-nowrap text-small font-medium" style={{ left: x(truth.value) }}>
          Truth {truth.display}
        </span>
        {rows.map((r) => (
          <div key={r.label} className="absolute inset-x-0" style={{ top: r.top }}>
            <span className="block text-small text-muted">
              {r.label} <span className="font-medium text-ink">{r.e.display}</span>
            </span>
            <span className={`mt-1 block h-3 ${r.bar}`} style={{ width: x(r.e.value) }} />
          </div>
        ))}
      </div>
      <div className="mt-5 border-t border-rule pt-4">
        <h3 className="m-0 text-body font-medium">This demo run</h3>
        <p className="m-0 text-small text-muted">One seed, on the noise the system was tuned on. This is the run the dashboard shows.</p>
        <p data-testid="demo-cohort" className="m-0 mt-2 text-body">
          <span className="font-medium text-ink-soft">Demo cohort</span>, one run on training-time noise: Emirates ID only {demo.eid.display}, Wasl{" "}
          {demo.wasl.display}, truth {demo.truth.display}.
        </p>
      </div>
      <p data-testid="kpi-why" className="m-0 mt-4 rounded-control bg-ground px-3 py-2 text-body">
        <span className="font-medium">Why two sets of figures?</span> The demo run is a single seed on noise the system was tuned on, so Wasl reads {demo.wasl.display} there
        and {wasl.display} on the five-seed held-out mean. The held-out mean is the number to quote.
      </p>
    </section>
  );
}

function Readers() {
  const corpora = result("reader.corpora");
  return (
    <section aria-labelledby="read-h" className={card}>
      <CardHead
        id="read-h"
        title="Readers on reports they had never seen"
        sub={`${corpora.value} corpora of 300 reports by 4 model families, each frozen before any reader ran. Each one tested the previous fix.`}
      />
      <div className="mt-3 flex flex-wrap items-baseline gap-3">
        <span className="whitespace-nowrap text-[30px] font-medium leading-10">3 of {corpora.value}</span>
        <span className="text-body text-muted">found an unsafe phrasing; each was fixed by a general rule</span>
      </div>
      <p className="m-0 mt-4 text-body">The last 2 fair tests passed with both readers. Current reader v7.2: 0 unsafe on every corpus.</p>
      <p className="m-0 mt-1 text-small text-muted">
        Over all {corpora.value}: the larger reader closed 6 of 211 wrongly, the local reader 10 of 137.
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
      <CardHead
        id="human-h"
        title="When the gate says no, two people sign"
        sub="False closures after human review. Offline sweep: 5 seeds, 3 reviewer error rates, simulated reviewers."
      />
      <div
        role="img"
        aria-label={`Two reviewers: 0 false closures in ${double.display}. One reviewer: ${single.display}.`}
        className="mt-4 grid grid-cols-[104px_minmax(0,1fr)] items-center gap-y-2.5"
      >
        <span className="text-body">Two reviewers</span>
        <span data-testid="double-review" className="flex items-center gap-2">
          <span className="text-[24px] font-medium leading-8">0</span>
          <span className="text-small text-muted">in {double.display}</span>
        </span>
        <span className="text-body">One reviewer</span>
        <span data-testid="single-review" className="flex items-center gap-2">
          <span className="block h-2.5 w-[84%] flex-none bg-unknown" />
          <span className="text-[24px] font-medium leading-8">{single.display}</span>
        </span>
      </div>
      <p className="m-0 mt-2 text-small text-muted">So double review is mandatory.</p>
      <div className="mt-4 flex items-start gap-2.5 border-t border-rule pt-4">
        <svg aria-hidden width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--color-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="mt-0.5 flex-none">
          <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
          <path d="m9 12 2 2 4-4" />
        </svg>
        <div>
          <p className="m-0 text-body">
            The 0.99 auto-close line is certified. On {ltt.display} calibration cases it made 0 false closures and 0 wrong-patient messages: with 95%
            confidence, both rates stay at or below 0.1%.
          </p>
          <p className="m-0 mt-1 text-small text-muted">Learn-then-Test, for cases like the synthetic calibration set. A pilot recalibrates on local cases.</p>
        </div>
      </div>
    </section>
  );
}

export default function ResultsPage() {
  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="m-0 text-title font-semibold">Results</h1>
          <p className="m-0 mt-1 text-body text-muted">
            Frozen evaluation at code {results.frozen_revision.slice(0, 7)} · 5 seeds · noise it never trained on · synthetic patients, real report
            texts
          </p>
        </div>
        <HonestyLabel kind="simulated">Simulated evaluation</HonestyLabel>
      </header>
      <div className="mt-6 grid gap-gap lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)]">
        <FalseClosures />
        <Kpi />
        <Readers />
        <Review />
      </div>
    </>
  );
}
