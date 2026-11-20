import Link from "next/link";
import { PoolingChart, RegionCapacityChart } from "@/components/programme/capacity-chart";
import { KpiChart } from "@/components/programme/kpi-chart";
import { byOverdueShare, poolingRuns, poolingVerb, programmeView } from "@/components/programme/model";
import { PatternLinks } from "@/components/programme/patterns";
import { ProgrammeChart } from "@/components/programme/programme-chart";
import { SourceHeading } from "@/components/programme/source-toggle";
import { StallsChart } from "@/components/programme/stalls-chart";
import { Workload } from "@/components/programme/workload";
import { PageHeader } from "@/components/shell/page-header";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { getDashboard } from "@/lib/api/source";
import { result } from "@/lib/results";

export const metadata = { title: "Programme" };

const NUMBER_WORD: Record<number, string> = { 3: "three", 4: "four", 5: "five", 6: "six", 7: "seven", 8: "eight", 9: "nine" };

const card = "rounded-card border border-line bg-surface p-pad";

export default async function ProgrammePage() {
  const dash = await getDashboard();
  const view = programmeView(dash);
  const { capacity } = view;
  const safety = result("v2.safety.crc_false_closures");
  const runs = poolingRuns();
  const [enough] = runs;
  const noPooling = enough.rows.find((r) => r.id.endsWith("skewed_no_travel"));
  const pooled = enough.rows.find((r) => r.id.endsWith("skewed_pool_60"));
  const programmeCases = view.programmes.reduce((t, p) => t + p.cases, 0);
  const crc = view.programmes.find((p) => p.key === "crc");
  const programmeCount = NUMBER_WORD[view.programmes.length] ?? String(view.programmes.length);
  const word = (k: number) => NUMBER_WORD[k] ?? String(k);

  // Takeaway lines, computed from the numbers the charts draw so they stay true for any run.
  const [first, second] = byOverdueShare(view.programmes);
  const withOnTime = view.programmes.filter((p) => p.onTime);
  const underHalf = withOnTime.filter((p) => p.onTime!.share < 50).length;
  const progTakeaway = first && second
    ? `${first.name} and ${second.name.charAt(0).toLowerCase()}${second.name.slice(1)} carry the largest overdue shares${
        view.onTimeMeasured && withOnTime.length ? `; on time is under half in ${word(underHalf)} of ${word(withOnTime.length)}` : ""
      }.`
    : null;
  const stallGroup = view.allStalls[0];

  return (
    <>
      <article data-testid="programme-rail" className="@container overflow-hidden rounded-[8px] border border-rule bg-surface">
        <PageHeader
          attached
          title="Colorectal screening"
          compact
          className="border-rule px-4 py-[17px] @min-[701px]:px-[clamp(19px,2.65cqi,42px)] @min-[701px]:py-[clamp(17px,2.2cqi,32px)]"
        />

        <section aria-labelledby="kpi-h" className="p-4 @min-[701px]:p-[clamp(19px,2.65cqi,42px)]">
          <SourceHeading id="kpi-h" title="Colonoscopy within 31 days" sub="Share of completed colonoscopies each method can see." display>
            Simulated colorectal data, positive FIT referrals. &quot;Emirates ID only&quot; counts a colonoscopy only when the report carries the
            matching Emirates ID. Wasl also matches reports by name and date of birth. Each bar is that method&apos;s 31-day rate as a share of the
            simulated truth, what really happened to each simulated patient. The target is the DoH 31-day standard.
          </SourceHeading>

          <KpiChart kpi={view.kpi} target={view.target} />
        </section>

        <Workload
          row={crc}
          note={view.chaseOneIn !== null && <p className="m-0 text-small text-ink-soft">1 in {view.chaseOneIn} on your chase list is already done.</p>}
        />
      </article>

      <div className="mt-gap grid items-start gap-gap">
        <section aria-labelledby="prog-h" className={card}>
          <SourceHeading id="prog-h" title="By programme" sub={progTakeaway && <span data-testid="prog-takeaway" className="text-body font-medium text-ink">{progTakeaway}</span>}>
            Simulated data: {programmeCases.toLocaleString("en-US")} cases across {programmeCount} programmes. Finished means closed with evidence,
            or no follow-up, referral or recall needed. Still open is the programme&apos;s cases that are neither finished nor overdue.
            {view.onTimeMeasured && (
              <>
                {" "}
                On time is the share of referrals past their deadline whose follow-up step was done by the deadline. Each programme has its
                own step and deadline (colonoscopy 31 days; colposcopy 14, 28 or 56 days by priority; follow-up CT within the Fleischner window;
                eye-clinic exam for a referable grade; breast assessment outcome; follow-up LDCT or work-up; IFHAS follow-up for each abnormal
                result). There is no target line: no official target exists for these.
              </>
            )}
          </SourceHeading>
          {view.onTimeMeasured ? (
            <div className="mt-2">
              <HonestyLabel plain kind="simulated">On time: Wasl-measured, simulated</HonestyLabel>
            </div>
          ) : (
            <p className="m-0 mt-2 text-small text-muted">On-time follow-up by programme is not available yet.</p>
          )}
          <div className="mt-3">
            <ProgrammeChart rows={view.programmes} onTimeMeasured={view.onTimeMeasured} />
          </div>
        </section>
      </div>

      <div className="mt-gap grid items-start gap-gap lg:grid-cols-[minmax(0,5fr)_minmax(0,7fr)]">
        <div className="grid min-w-0 gap-gap">
          <section aria-labelledby="stall-h" className={card}>
          <SourceHeading
            id="stall-h"
            title="Where patients stall"
            sub={
              stallGroup && (
                <span data-testid="stall-scope" className="text-body font-medium text-ink">
                  {stallGroup.pathway}: the {word(view.stalls.length)} largest of {word(stallGroup.stalls.length)} stalls
                </span>
              )
            }
          >
            Simulated data: {view.cases.toLocaleString("en-US")} colorectal and cervical cases. The three largest stalls are shown.
            &quot;Show all stalls&quot; adds the lung, eye, breast, lung screening and IFHAS programmes.
          </SourceHeading>
          <div className="mt-3">
            <StallsChart top={view.stalls} groups={view.allStalls} />
          </div>
        </section>

          <PatternLinks patterns={view.patterns} className={card} />
        </div>

        {capacity && (
          <section aria-labelledby="cap-h" className={card}>
            <SourceHeading id="cap-h" title="Booking capacity">
              {capacity.note} Slots are suggested one patient at a time, not as a best overall plan. The figures on sharing slots come from a
              separate simulation with slots placed in the wrong regions.
            </SourceHeading>
            <div className="mt-2">
              <HonestyLabel plain kind="simulated">Simulated decision support</HonestyLabel>
            </div>

            <p data-testid="capacity-condition" className="m-0 mt-3 text-body font-medium">
              {capacity.conditionWords}
            </p>

            <div className="mt-5">
              <RegionCapacityChart regions={capacity.regions} />
            </div>

            <div className="mt-6 border-t border-rule pt-5">
              <h3 className="m-0 text-body font-medium">Sharing slots between regions</h3>
              {noPooling && pooled && (
                <p data-testid="pooling-result" className="m-0 mt-1 text-body text-ink-soft">
                  Sharing slots between regions {poolingVerb(noPooling.share, pooled.share)} on-time colonoscopy — but only when there are enough
                  slots overall.
                </p>
              )}
              <div className="mt-3">
                <PoolingChart runs={runs} />
              </div>
            </div>
          </section>
        )}
      </div>

      {/* Static supporting copy with no object state, so it sits on the page grid unframed (DEC-3); the chart panels above keep their surfaces. */}
      <section aria-labelledby="safety-h" className="mt-8 flex flex-wrap items-center gap-x-5 gap-y-2">
        <h2 id="safety-h" className="sr-only">
          Safety
        </h2>
        {/* The zero is the good news: a positive check badge, not a muted glyph. */}
        <span aria-hidden className="inline-flex size-10 flex-none items-center justify-center rounded-full bg-pill-positive-bg text-positive">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
            <path d="m9 12 2 2 4-4" />
          </svg>
        </span>
        <p data-testid="safety-figure" className="m-0 text-[48px] font-semibold leading-none tracking-[-0.02em] text-ink">
          {safety.value}
        </p>
        <div className="min-w-0">
          <p className="m-0 text-body font-semibold">false closures</p>
          <p className="m-0 text-small text-ink-soft">
            in {safety.denominator?.toLocaleString("en-US")} tested · every closure has evidence or two signatures ·{" "}
            <Link href="/app/results" className="font-medium no-underline">
              See proof
            </Link>
          </p>
          {safety.upper95 && (
            <p data-testid="safety-bound" className="m-0 text-small text-muted">
              The true rate is likely at most {Number(parseFloat(safety.upper95).toPrecision(2))}%
            </p>
          )}
        </div>
      </section>
    </>
  );
}
