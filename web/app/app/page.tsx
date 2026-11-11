import Link from "next/link";
import { PoolingChart, RegionCapacityChart } from "@/components/programme/capacity-chart";
import { KpiChart } from "@/components/programme/kpi-chart";
import { cellText, pct, poolingRuns, programmeView } from "@/components/programme/model";
import { PatternLinks } from "@/components/programme/patterns";
import { ProgrammeChart } from "@/components/programme/programme-chart";
import { SourceHeading } from "@/components/programme/source-toggle";
import { StallsChart } from "@/components/programme/stalls-chart";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { getDashboard } from "@/lib/api/source";
import { result } from "@/lib/results";

export const metadata = { title: "Programme" };

const NUMBER_WORD: Record<number, string> = { 3: "three", 4: "four", 5: "five", 6: "six", 7: "seven", 8: "eight", 9: "nine" };

const card = "rounded-card border border-line bg-surface p-pad";

export default async function ProgrammePage() {
  const dash = await getDashboard();
  const view = programmeView(dash);
  const { capacity, levers } = view;
  const safety = result("v2.safety.crc_false_closures");
  const runs = poolingRuns();
  const [enough] = runs;
  const noPooling = enough.rows.find((r) => r.id.endsWith("skewed_no_travel"));
  const pooled = enough.rows.find((r) => r.id.endsWith("skewed_pool_60"));
  const programmeCases = view.programmes.reduce((t, p) => t + p.cases, 0);
  const programmeCount = NUMBER_WORD[view.programmes.length] ?? String(view.programmes.length);

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="m-0 text-title font-semibold">Programme</h1>
          <p className="m-0 mt-1 text-body text-muted">Your 31-day rate is under-reported. Wasl finds the patients the Emirates ID misses.</p>
        </div>
        <HonestyLabel kind="simulated">Simulated cohort</HonestyLabel>
      </header>

      <div className="mt-6 grid items-start gap-gap lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)]">
        <section aria-labelledby="kpi-h" className={card}>
          <SourceHeading id="kpi-h" title="Colonoscopy within 31 days of a positive FIT">
            Simulated colorectal cohort. The reported rate counts a colonoscopy only when the Emirates ID on the report matches. Wasl also links
            reports by name and date of birth. The true rate comes from the cohort&apos;s known outcomes. The target is the DoH 31-day standard.
            The demo run is the run this dashboard serves; the five-seed mean is the frozen evaluation in results.json.
          </SourceHeading>

          <KpiChart kpi={view.kpi} target={view.target} />

          <div className="mt-6 flex items-center justify-between gap-3 border-t border-rule pt-5">
            <p className="m-0 text-body">{view.chaseOneIn !== null && <>1 in {view.chaseOneIn} on your chase list is already done.</>}</p>
            <Link
              href="/app/today"
              className="inline-flex h-control flex-none items-center rounded-control bg-cta px-4 font-medium text-white no-underline hover:bg-ink hover:text-white"
            >
              Open today&apos;s list
            </Link>
          </div>
        </section>

        <section aria-labelledby="prog-h" className={card}>
          <SourceHeading id="prog-h" title="By programme">
            Simulated run of {programmeCases.toLocaleString("en-US")} cases across {programmeCount} programmes. Still open is the programme&apos;s cases minus
            those closed and those overdue.
          </SourceHeading>
          <div className="mt-3">
            <ProgrammeChart rows={view.programmes} />
          </div>
        </section>
      </div>

      <div className="mt-gap grid items-start gap-gap lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
        <section aria-labelledby="stall-h" className={card}>
          <SourceHeading id="stall-h" title="Where patients stall">
            Simulated run of {view.cases.toLocaleString("en-US")} cases, colorectal and cervical together (run {dash.meta.run}). The three largest stalls are shown.
            The run&apos;s drop-off counts cover all {programmeCount} programmes; these stall labels are the colorectal and cervical pathway&apos;s.
            &quot;Show all stalls&quot; adds the lung, eye, breast, lung screening and IFHAS pathways.
          </SourceHeading>
          <div className="mt-3">
            <StallsChart top={view.stalls} groups={view.allStalls} />
          </div>
        </section>

        {capacity && (
          <section aria-labelledby="cap-h" className={card}>
            <SourceHeading id="cap-h" title="Booking capacity">
              {capacity.note} The optimiser is greedy: it suggests a slot for each case in turn and does not search for a best plan. The pooling
              test comes from booking/sim/pool_test.json, a separate simulation of 30 draws with capacity placed in the wrong regions.
            </SourceHeading>
            <div className="mt-2">
              <HonestyLabel kind="simulated">Simulated decision support</HonestyLabel>
            </div>

            <p data-testid="capacity-condition" className="m-0 mt-3 text-body font-medium">
              {capacity.conditionWords}
            </p>
            <p className="m-0 mt-1 text-body text-ink-soft">
              {capacity.poolingNote.charAt(0).toUpperCase() + capacity.poolingNote.slice(1)}. {capacity.slots.toLocaleString("en-US")} slots for{" "}
              {capacity.openCases.toLocaleString("en-US")} open cases.
            </p>

            <dl data-testid="capacity-summary" className="m-0 mt-4 grid grid-cols-2 gap-x-4 gap-y-3 tabular-nums sm:grid-cols-3">
              {(
                [
                  ["Within 31 days", pct(capacity.summary.within31dShare * 100)],
                  ["Median wait", `${capacity.summary.medianWaitDays} days`],
                  ["Cross-region bookings", String(capacity.summary.crossRegion)],
                  ["Nurse calls", String(capacity.summary.nurseCalls)],
                  ["No slot found", String(capacity.summary.noSlot)],
                ] as const
              ).map(([term, value]) => (
                <div key={term}>
                  <dt className="text-small text-muted">{term}</dt>
                  <dd className="m-0 mt-0.5 text-section font-medium">{value}</dd>
                </div>
              ))}
            </dl>

            {levers && (
              <p data-testid="capacity-levers" className="m-0 mt-4 text-body text-ink-soft tabular-nums">
                <span className="font-medium text-ink">Queue levers:</span>{" "}
                {[
                  levers.direct && `${cellText(levers.direct.cell, levers.direct.of)} suggested direct to colonoscopy`,
                  levers.highPrepRisk && `${cellText(levers.highPrepRisk.cell, levers.highPrepRisk.of)} at high risk of failed bowel prep`,
                ]
                  .filter(Boolean)
                  .join("; ")}
                . Counted over the{" "}
                {[levers.direct, levers.highPrepRisk].every((l) => !l || l.of === capacity.openCases) && `${capacity.openCases.toLocaleString("en-US")} `}
                open colorectal cases needing a colonoscopy slot. The patient list cannot filter by pathway or prep risk, so these counts do not link.
                {levers.requiresClinicianOrder && " A prep regimen change needs a clinician’s order."}
              </p>
            )}

            <div className="mt-5">
              <RegionCapacityChart regions={capacity.regions} />
            </div>

            <div className="mt-6 border-t border-rule pt-5">
              <h3 className="m-0 text-body font-medium">What pooling changes</h3>
              <p className="m-0 mt-1 text-body text-ink-soft">
                Pooling only moves existing slots between regions. It cannot create slots, so it does not help when the whole network is short.
              </p>
              {noPooling && pooled && (
                <p data-testid="pooling-result" className="m-0 mt-2 text-body">
                  Measured in the simulated booking test, with {enough.slotsPerCase} slots per FIT+ in total: {noPooling.display} within 31 days
                  without cross-site booking, {pooled.display} with network pooling.
                </p>
              )}
              <div className="mt-3">
                <PoolingChart runs={runs} />
              </div>
            </div>
          </section>
        )}
      </div>

      <PatternLinks patterns={view.patterns} />

      {/* Static supporting copy with no object state, so it sits on the page grid unframed (DEC-3); the chart panels above keep their surfaces. */}
      <section aria-labelledby="safety-h" className="mt-8">
        <h2 id="safety-h" className="m-0 flex items-center gap-2 text-section font-medium">
          <svg aria-hidden width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--color-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="flex-none">
            <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
            <path d="m9 12 2 2 4-4" />
          </svg>
          Safety
        </h2>
        <p className="m-0 mt-1 text-body">
          {safety.value} false closures in {safety.denominator?.toLocaleString("en-US")} tested. Every closure has evidence or two signatures.{" "}
          <Link href="/app/results" className="font-medium no-underline">
            See proof
          </Link>
        </p>
      </section>
    </>
  );
}
