import Link from "next/link";
import { BenchBullets, BenchPairs, GapChart, RateChart, SafetyChart, ScenarioRanges, TornadoChart, WorstStrip } from "@/components/evidence/charts";
import {
  HOSTED,
  REPO,
  asPercent,
  calibrationPer1000,
  claimCounts,
  evidenceHref,
  firstUrl,
  frozenWords,
  gapRuns,
  loadEvidence,
  num,
  ratePanels,
  safetyPairs,
  scenarioOrder,
  withInterval,
  worstOthers,
  worstRanges,
} from "@/components/evidence/model";
import { PageHeader } from "@/components/shell/page-header";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { HONESTY } from "@/lib/honesty";
import type { Evidence, EvidenceClaim } from "@/lib/api/evidence-schema";

export const metadata = { title: "Evidence" };

const card = "min-w-0 rounded-card border border-line bg-surface p-pad";
const h2 =
  "staff-display m-0 text-[clamp(23px,2.2vw,32px)] leading-[1.1] text-jade-900";
const h3 = "m-0 text-body font-medium";
const th =
  "border-b border-line px-1.5 py-2 text-left align-bottom text-small font-medium text-muted";
const td = "border-b border-rule px-1.5 py-2 align-top";
const link = "text-jade-600 underline underline-offset-2 hover:text-ink";

function Ext({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <a href={href} target="_blank" rel="noreferrer" className={link}>
      {children}
    </a>
  );
}

function Section({
  id,
  title,
  aside,
  children,
}: {
  id: string;
  title: string;
  aside?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <section
      aria-labelledby={`${id}-h`}
      data-testid={`evidence-${id}`}
      className={card}
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <h2 id={`${id}-h`} className={h2}>
          {title}
        </h2>
        {aside}
      </div>
      {children}
    </section>
  );
}

function Headline({ ev }: { ev: Evidence }) {
  const { per_1000: rows, calibration } = ev.impact;
  const seha = {
    per1000: calibrationPer1000(calibration.value),
    short: `SEHA ${num(asPercent(calibration.value))}%`,
    caveat: calibration.caveat,
  };
  return (
    <Section
      id="gap"
      title="Wasl raises the measured rate toward the true rate"
    >
      <GapChart
        runs={gapRuns(rows)}
        calibration={seha}
        source={
          <p data-testid="calibration" className="m-0">
            For comparison: {seha.short}, {calibration.label} (<Ext href={calibration.url}>{calibration.source}</Ext>). A sense check, not proof.
          </p>
        }
      />
    </Section>
  );
}

function Terms({ ev }: { ev: Evidence }) {
  return (
    <details data-testid="terms" className="group mt-4">
      <summary className="inline-flex min-h-11 cursor-pointer items-center gap-1 text-small font-medium text-jade-600 md:min-h-8">
        <span aria-hidden className="inline-block transition-transform group-open:rotate-90">
          ›
        </span>
        What the terms mean
      </summary>
      <dl className="m-0 mt-2 grid grid-cols-[minmax(0,1fr)] gap-x-6 gap-y-2 text-small [overflow-wrap:anywhere] sm:grid-cols-2">
        {ev.accuracy.definitions.map((d) => (
          <div key={d.term}>
            <dt className="font-medium text-ink">{d.term}</dt>
            <dd className="m-0 text-ink-soft">{d.meaning}</dd>
          </div>
        ))}
      </dl>
    </details>
  );
}

function StatTile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="min-w-0 rounded-control bg-raised px-3 py-2">
      <dt className="text-small text-ink-soft">{label}</dt>
      <dd className="m-0 text-title font-semibold text-ink">{value}</dd>
      {sub && <dd className="m-0 text-small text-muted">{sub}</dd>}
    </div>
  );
}

function Accuracy({ ev }: { ev: Evidence }) {
  const { linkage, safety, worst } = ev.accuracy;
  const { closures, other } = safetyPairs(safety);
  const rates = ratePanels(linkage);
  const ranges = worstRanges(worst);
  const tiles = worstOthers(worst);
  const anyClosure = closures.some((p) => (p.v1?.events ?? 0) + (p.v2?.events ?? 0) > 0);
  const extras = rates.other.filter((p) => p.v2 || p.v1);
  const tileValue = (l: (typeof extras)[number]["v2"], metric: string) => (l ? withInterval(l, metric).split(" ")[0] : "–");
  const tileName = (m: string) => {
    const t = m.replace(/^Review band:\s*/i, "").replace(/\s*\(not comparable across linkers\)/i, "");
    return t.charAt(0).toUpperCase() + t.slice(1);
  };
  return (
    <Section
      id="accuracy"
      title={anyClosure ? "False closures were seen in the counts below" : "No false closures in any programme, and misses stay visible"}
      aside={<HonestyLabel plain kind="simulated">Simulated patients, 5 runs</HonestyLabel>}
    >
      <Terms ev={ev} />
      <div className="mt-5 grid items-start gap-x-gap gap-y-8 lg:grid-cols-2">
        <div className="min-w-0">
          <h3 className={h3}>False closures: 0 seen, and how high the rate could still be</h3>
          <div className="mt-2">
            <SafetyChart closures={closures} other={other} />
          </div>
        </div>
        <div className="min-w-0">
          <h3 className={h3}>Wrong matches are rare, and missed matches stay open for review</h3>
          <div className="mt-2">
            <RateChart falseMatch={rates.falseMatch} missed={rates.missed} />
          </div>
          {extras.length > 0 && (
            <dl data-testid="linkage-tiles" className="m-0 mt-4 grid grid-cols-[repeat(auto-fit,minmax(9rem,1fr))] gap-2">
              {extras.map((p) => (
                <StatTile
                  key={p.metric}
                  label={tileName(p.metric)}
                  value={tileValue(p.v2, p.metric)}
                  sub={p.v1 ? `Tuning data ${tileValue(p.v1, p.metric)}` : undefined}
                />
              ))}
            </dl>
          )}
        </div>
      </div>
      {(ranges.length > 0 || tiles.length > 0) && (
        <div className="mt-8">
          <h3 className={h3}>Even the worst run stays close to the average</h3>
          {ranges.length > 0 && (
            <div className="mt-2">
              <WorstStrip ranges={ranges} />
            </div>
          )}
          {tiles.length > 0 && (
            <dl data-testid="worst-tiles" className="m-0 mt-4 grid grid-cols-[repeat(auto-fit,minmax(12rem,1fr))] gap-2">
              {[...new Set(tiles.map((t) => t.name))].map((name) => {
                const v2 = tiles.find((t) => t.name === name && t.noise !== "v1");
                const v1 = tiles.find((t) => t.name === name && t.noise === "v1");
                const lead = v2 ?? v1!;
                return (
                  <StatTile
                    key={name}
                    label={name}
                    value={lead.lead}
                    sub={[lead.who, v1 && v1 !== lead ? `Tuning data ${v1.who ? `${v1.who} ` : ""}${v1.lead}` : null].filter(Boolean).join(" · ") || undefined}
                  />
                );
              })}
            </dl>
          )}
        </div>
      )}
    </Section>
  );
}

function Benchmarks({ ev }: { ev: Evidence }) {
  const rows = ev.benchmarks;
  const paired = rows.filter((b) => b.baseline !== null);
  const alone = rows.filter((b) => b.baseline === null);
  const wins = paired.filter((b) => (/false_match/i.test(b.metric) ? b.wasl < b.baseline! : b.wasl > b.baseline!)).length;
  const links = new Map<string, string>();
  for (const b of rows) {
    if (b.url) links.set(b.dataset, b.url);
    const u = firstUrl(b.baseline_source);
    if (u && b.baseline_source) links.set(b.baseline_source.split(/[(:]/)[0].trim(), u);
  }
  return (
    <Section
      id="benchmarks"
      title={rows.length === 0 ? "On data the team did not generate" : wins === paired.length && paired.length > 0 ? "Ahead of the published baseline on public data" : "On data the team did not generate"}
      aside={rows.length > 0 ? <HonestyLabel plain kind="built">Public datasets</HonestyLabel> : undefined}
    >
      {rows.length === 0 ? (
        <p data-testid="benchmarks-none" className="m-0 mt-2 text-body text-ink-soft">
          Not run yet.
        </p>
      ) : (
        <>
          <div className="mt-4 grid items-start gap-x-gap gap-y-8 xl:grid-cols-2">
            {paired.length > 0 && (
              <div className="min-w-0">
                <BenchPairs rows={paired} />
              </div>
            )}
            {alone.length > 0 && (
              <div className="min-w-0">
                <BenchBullets rows={alone} />
              </div>
            )}
          </div>
          <p data-testid="bench-sources" className="m-0 mt-4 flex flex-wrap gap-x-3 gap-y-1 text-small text-muted">
            <span>Sources</span>
            {[...links].map(([name, url]) => (
              <Ext key={url} href={url}>
                {name}
              </Ext>
            ))}
          </p>
        </>
      )}
    </Section>
  );
}

function Modelled({ ev }: { ev: Evidence }) {
  const m = ev.impact.modelled;
  const scenarios = scenarioOrder(m.scenarios);
  const cancers = scenarios.map((s) => s.cancers_found_per_1000);
  const base = scenarios.find((s) => s.name === "base")?.cancers_found_per_1000 ?? null;
  return (
    <Section
      id="modelled"
      title={`${num(Math.min(...cancers))} to ${num(Math.max(...cancers))} more cancers found per 1,000 if navigators act on the list`}
      aside={<HonestyLabel plain kind="illustrative">Modelled, not measured</HonestyLabel>}
    >
      <p data-testid="modelled-label" className="m-0 mt-2 flex flex-wrap gap-2 text-small">
        <span className="rounded-chip bg-ochre-wash px-2 py-0.5 text-ochre-ink">Only if navigators act on the corrected list</span>
        <span className="rounded-chip bg-ochre-wash px-2 py-0.5 text-ochre-ink">Worst case is the measured gap alone</span>
        <span className="rounded-chip bg-ochre-wash px-2 py-0.5 text-ochre-ink">Not Wasl&apos;s direct effect</span>
      </p>
      <div className="mt-4 grid items-start gap-x-gap gap-y-8 lg:grid-cols-2">
        <div className="min-w-0">
          <ScenarioRanges scenarios={scenarios} />
        </div>
        {m.tornado.length > 0 && (
          <div className="min-w-0">
            <TornadoChart rows={m.tornado} output="Cancers found per 1,000" base={base} />
          </div>
        )}
      </div>

      {m.parameters.length > 0 && (
        <details className="group mt-4">
          <summary className="inline-flex min-h-11 cursor-pointer items-center gap-1 text-small font-medium text-jade-600 md:min-h-8">
            <span aria-hidden className="inline-block transition-transform group-open:rotate-90">
              ›
            </span>
            Assumptions and sources ({m.parameters.length})
          </summary>
          <p className="m-0 mt-2 text-small text-ink-soft">{m.label}</p>
          <ul className="m-0 mt-2 grid list-none gap-2 p-0 text-small">
            {m.parameters.map((p) => (
              <li key={p.name} className="flex flex-wrap items-baseline gap-x-2 gap-y-1 border-b border-rule pb-2">
                <span className="font-medium text-ink">{p.name}</span>
                <span className="tabular-nums">{p.value}</span>
                <span className="rounded-chip bg-raised px-1.5 text-ink-soft">
                  {p.scope === "ASSUMED" ? "Assumed" : p.scope === "UAE" ? "UAE source" : "International source"}
                </span>
                <span className="text-muted">{p.url ? <Ext href={p.url}>{p.source}</Ext> : p.source}</span>
              </li>
            ))}
          </ul>
        </details>
      )}
    </Section>
  );
}

function EvidenceLink({ claim }: { claim: EvidenceClaim }) {
  const target = evidenceHref(claim.evidence);
  if (!target) return <span className="text-muted">–</span>;
  const label = (
    <span className="[overflow-wrap:anywhere]">{claim.evidence}</span>
  );
  return target.external ? (
    <Ext href={target.href}>{label}</Ext>
  ) : (
    <Link href={target.href} className={link}>
      {label}
    </Link>
  );
}

/** A command in code type; a click path or a "not reproducible" note (starts with a capital) as plain words. */
function Reproduce({ text }: { text: string }) {
  if (/^[A-Z]/.test(text)) return <span className="text-ink-soft">{text}</span>;
  return <code className="break-all font-mono text-[13px]">{text}</code>;
}

function Ledger({ ev }: { ev: Evidence }) {
  const counts = claimCounts(ev.claims);
  return (
    <Section id="claims" title={`${ev.claims.length} claims, each with its evidence and a rerun`}>
      <ul data-testid="claim-counts" className="m-0 mt-3 flex list-none flex-wrap gap-2 p-0">
        {counts
          .filter((c) => c.count > 0)
          .map((c) => (
            <li key={c.status} data-status={c.status}>
              <HonestyLabel kind={c.status}>
                <span className="font-semibold tabular-nums">{c.count}</span>&nbsp;{HONESTY[c.status].toLowerCase()}
              </HonestyLabel>
            </li>
          ))}
      </ul>
      <details data-testid="claims" className="group mt-3">
        <summary className="inline-flex min-h-11 cursor-pointer items-center gap-1 text-small font-medium text-jade-600 md:min-h-8">
          <span aria-hidden className="inline-block transition-transform group-open:rotate-90">
            ›
          </span>
          All {ev.claims.length} claims
        </summary>
        {/* Desktop: a table. Phone: the same rows as a list, so nothing scrolls sideways. */}
        <table data-testid="claims-table" className="mt-3 hidden w-full border-collapse text-body md:table">
          <caption className="sr-only">Claims</caption>
          <thead>
            <tr>
              <th scope="col" className={th}>
                Claim
              </th>
              <th scope="col" className={th}>
                Status
              </th>
              <th scope="col" className={th}>
                Evidence
              </th>
              <th scope="col" className={th}>
                How to check
              </th>
            </tr>
          </thead>
          <tbody>
            {ev.claims.map((c) => (
              <tr key={c.id}>
                <th scope="row" className={`${td} text-left font-normal`}>
                  {c.claim}
                </th>
                <td className={td}>
                  <HonestyLabel kind={c.status} />
                </td>
                <td className={`${td} text-small`}>
                  <EvidenceLink claim={c} />
                </td>
                <td className={`${td} text-small`}>
                  <Reproduce text={c.reproduce} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <ul data-testid="claims-list" className="m-0 mt-3 grid list-none gap-3 p-0 md:hidden">
          {ev.claims.map((c) => (
            <li key={c.id} className="border-b border-rule pb-3">
              <p className="m-0 text-body">{c.claim}</p>
              <div className="mt-1.5">
                <HonestyLabel kind={c.status} />
              </div>
              <p className="m-0 mt-1.5 text-small">
                <span className="text-muted">Evidence: </span>
                <EvidenceLink claim={c} />
              </p>
              <p className="m-0 mt-1 text-small">
                <span className="text-muted">How to check: </span>
                <Reproduce text={c.reproduce} />
              </p>
            </li>
          ))}
        </ul>
      </details>
    </Section>
  );
}

function Verify() {
  return (
    <Section id="verify" title="Verify it yourself in two minutes">
      <pre data-testid="verify-cmd" className="m-0 mt-3 whitespace-pre-wrap break-all rounded-control bg-raised px-3 py-2 text-small">
        <code className="font-mono">git clone {REPO} && cd wasl-futurehealth && make verify</code>
      </pre>
      <pre className="m-0 mt-2 whitespace-pre-wrap break-all rounded-control bg-raised px-3 py-2 text-small">
        <code className="font-mono">make verify SEED=&lt;n&gt;</code>
      </pre>
      <p className="m-0 mt-3 flex flex-wrap gap-x-3 gap-y-1 text-small">
        <Ext href={REPO}>GitHub repository</Ext>
        <Ext href={`${REPO}/blob/main/docs/EVIDENCE.md`}>docs/EVIDENCE.md</Ext>
        <Link href="/app/results" className={link}>
          Results
        </Link>
        <Ext href={HOSTED}>{HOSTED.replace(/^https:\/\//, "")}</Ext>
      </p>
    </Section>
  );
}

export default function EvidencePage() {
  const ev = loadEvidence();
  return (
    <>
      <PageHeader
        title="Evidence"
        sub={`What Wasl measured, how sure we are, and how to check it. Locked on ${frozenWords(ev.generated.frozen_at)}, before the final test runs.`}
      />
      <div className="grid grid-cols-[minmax(0,1fr)] gap-gap">
        <Headline ev={ev} />
        <Accuracy ev={ev} />
        <Benchmarks ev={ev} />
        <Modelled ev={ev} />
        <Ledger ev={ev} />
        <Verify />
      </div>
    </>
  );
}
