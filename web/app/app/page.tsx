import Link from "next/link";
import { pct, points, programmeView } from "@/components/programme/model";
import { SourceHeading } from "@/components/programme/source-toggle";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { getDashboard } from "@/lib/api/source";
import { result } from "@/lib/results";

export const metadata = { title: "Programme" };

const card = "rounded-card border border-line bg-surface p-pad";

function KpiTrack({ eid, wasl, target }: { eid: number; wasl: number; target: number | null }) {
  const at = (v: number) => ({ left: `${v}%` });
  const label = `On a 0 to 100% scale: reported ${pct(eid)}, measured by Wasl ${pct(wasl)}${target === null ? "" : `, target ${target}%`}.`;
  return (
    <figure role="img" aria-label={label} className="m-0 mt-7">
      <div aria-hidden className="relative h-5 text-small leading-5">
        <span className="absolute top-0 -translate-x-1/2 whitespace-nowrap text-muted" style={at(eid)}>
          Reported
        </span>
        <span className="absolute top-0 -translate-x-1/2 whitespace-nowrap font-medium text-hue" style={at(wasl)}>
          Wasl
        </span>
        {target !== null && (
          <span className="absolute top-0 -translate-x-1/2 whitespace-nowrap" style={at(target)}>
            Target {target}%
          </span>
        )}
      </div>
      <div aria-hidden className="relative mt-1.5 h-2 rounded-[4px] bg-raised">
        <div className="absolute left-0 top-0 h-2 rounded-[4px] bg-hue" style={{ width: `${wasl}%` }} />
        <div className="absolute -top-1.5 h-5 w-0.5 -translate-x-1/2 bg-muted" style={at(eid)} />
        {target !== null && <div className="absolute -top-1.5 h-5 w-0 -translate-x-px border-l-2 border-dashed border-ink" style={at(target)} />}
      </div>
      <div aria-hidden className="mt-1.5 flex justify-between text-small leading-5 text-muted">
        <span>0%</span>
        <span>100%</span>
      </div>
    </figure>
  );
}

export default async function ProgrammePage() {
  const view = programmeView(await getDashboard());
  const { rate } = view;
  const safety = result("v2.safety.crc_false_closures");
  const maxStall = view.stalls[0]?.count ?? 1;

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
          </SourceHeading>

          <div className="mt-5 flex items-end gap-12">
            <div data-testid="kpi-eid">
              <div className="text-small text-muted">Reported, Emirates ID only</div>
              <div className="mt-1 text-lead font-medium tracking-[-0.02em] text-muted">{pct(rate.eid)}</div>
            </div>
            <div data-testid="kpi-wasl">
              <div className="text-small text-muted">Measured by Wasl</div>
              <div className="mt-1 text-lead font-bold tracking-[-0.02em] text-hue">{pct(rate.wasl)}</div>
            </div>
          </div>

          <KpiTrack eid={rate.eid} wasl={rate.wasl} target={view.target} />
          <p data-testid="kpi-truth" className="m-0 mt-2 text-small text-muted">
            Within {points(rate.truth, rate.wasl)} of the true rate ({pct(rate.truth)}) in testing
          </p>

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

        <section aria-labelledby="stall-h" className={card}>
          <SourceHeading id="stall-h" title="Where patients stall">
            Simulated run of {view.cases.toLocaleString("en-US")} cases, colorectal and cervical together. The three largest stalls are shown.
          </SourceHeading>
          <ol className="m-0 mt-3 list-none p-0">
            {view.stalls.map((s) => (
              <li key={s.dropoff}>
                <Link
                  href={`/app/patients?dropoff=${s.dropoff}`}
                  className="grid h-14 grid-cols-[minmax(0,1fr)_auto] content-center gap-x-3 text-ink no-underline hover:text-ink [&:hover_.stall-label]:underline"
                >
                  <span className="stall-label text-body">{s.label}</span>
                  <svg aria-hidden width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--color-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="row-span-2 self-center">
                    <path d="M6 3.5l4.5 4.5L6 12.5" />
                  </svg>
                  <span className="mt-1.5 flex items-center gap-1.5">
                    <span aria-hidden className="block h-2.5 flex-none rounded-[5px] bg-cta" style={{ width: `${((s.count / maxStall) * 88).toFixed(2)}%` }} />
                    <span className="text-small font-medium">{s.count.toLocaleString("en-US")}</span>
                  </span>
                </Link>
              </li>
            ))}
          </ol>
        </section>
      </div>

      <section aria-label="Safety" className="mt-gap flex items-center gap-3 rounded-card border border-line bg-surface px-pad py-4">
        <svg aria-hidden width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--color-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="flex-none">
          <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
          <path d="m9 12 2 2 4-4" />
        </svg>
        <p className="m-0 text-body">
          {safety.value} false closures in {safety.denominator?.toLocaleString("en-US")} tested. Every closure has evidence or two signatures.{" "}
          <Link href="/app/results" className="font-medium no-underline">
            See proof
          </Link>
        </p>
      </section>
    </>
  );
}
