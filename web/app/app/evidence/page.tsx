import Link from "next/link";
import { GapChart, TornadoChart } from "@/components/evidence/charts";
import {
  HOSTED,
  REPO,
  asPercent,
  calibrationPer1000,
  evidenceHref,
  fmt,
  frozenWords,
  headline,
  humanise,
  linkageRows,
  loadEvidence,
  num,
  safetyOrder,
  safetyWords,
  scenarioOrder,
  withInterval,
} from "@/components/evidence/model";
import { PageHeader } from "@/components/shell/page-header";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { Evidence, EvidenceClaim } from "@/lib/api/evidence-schema";

export const metadata = { title: "Evidence" };

const card = "rounded-card border border-line bg-surface p-pad";
const h2 = "staff-display m-0 text-[clamp(23px,2.2vw,32px)] leading-[1.1] text-jade-900";
const h3 = "m-0 text-body font-medium";
const th = "border-b border-line px-1.5 py-2 text-left align-bottom text-small font-medium text-muted";
const td = "border-b border-rule px-1.5 py-2 align-top";
const link = "text-jade-600 underline underline-offset-2 hover:text-ink";

function Ext({ href, children }: { href: string; children: React.ReactNode }) {
  return (
    <a href={href} target="_blank" rel="noreferrer" className={link}>
      {children}
    </a>
  );
}

function Section({ id, title, aside, children }: { id: string; title: string; aside?: React.ReactNode; children: React.ReactNode }) {
  return (
    <section aria-labelledby={`${id}-h`} data-testid={`evidence-${id}`} className={card}>
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
  const { chart, flagged } = headline(rows);
  const seha = { per1000: calibrationPer1000(calibration.value), short: `SEHA ${num(asPercent(calibration.value))}%` };
  return (
    <Section id="gap" title="The measured gap" aside={<HonestyLabel kind="simulated">Synthetic cohort, measured</HonestyLabel>}>
      <p className="m-0 mt-1 max-w-[68ch] text-body text-ink-soft">
        Per 1,000 positive FIT results, 31-day colonoscopy. Matching on Emirates ID alone misses completions that happened elsewhere, so
        navigators chase people who already had the test.
      </p>

      {flagged && flagged.eid_only !== null && flagged.wasl !== null && (
        <div data-testid="flagged" className="mt-4 flex flex-wrap items-baseline gap-x-3 gap-y-1">
          <span className="whitespace-nowrap text-lead font-bold tracking-[-0.02em] tabular-nums">
            {fmt(flagged.eid_only)} <span aria-hidden>→</span>
            <span className="sr-only">to</span> {fmt(flagged.wasl)}
          </span>
          <span className="text-body text-muted">
            {flagged.row.replace(/^./, (c) => c.toLowerCase())} per 1,000: Emirates ID only, then Wasl
          </span>
        </div>
      )}

      <div className="mt-5 grid items-start gap-gap lg:grid-cols-[minmax(0,7fr)_minmax(0,5fr)]">
        <div>{chart ? <GapChart row={chart} calibration={seha} /> : <p className="m-0 text-small text-muted">No completions row in this evidence file.</p>}</div>
        <div className="rounded-control bg-ochre-wash px-4 py-3 text-small text-ochre-ink">
          <p className="m-0 font-medium">Calibration point</p>
          <p data-testid="calibration" className="m-0 mt-1">
            {calibration.label}: {num(asPercent(calibration.value))}% (<Ext href={calibration.url}>{calibration.source}</Ext>).
          </p>
          <p className="m-0 mt-1">{calibration.caveat}</p>
        </div>
      </div>

      <table className="mt-5 w-full border-collapse text-body tabular-nums">
        <caption className="sr-only">Per 1,000 positive FITs, 31-day window</caption>
        <thead>
          <tr>
            <th scope="col" className={th}>
              Per 1,000 positive FITs
            </th>
            <th scope="col" className={`${th} text-right`}>
              Emirates ID only
            </th>
            <th scope="col" className={`${th} text-right`}>
              Wasl
            </th>
            <th scope="col" className={`${th} text-right`}>
              Truth
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const cell = (v: number | null) => (v === null ? <span aria-label="not applicable">–</span> : `${fmt(v)}${r.unit === "%" ? "%" : ""}`);
            return (
              <tr key={r.row}>
                <th scope="row" className={`${td} text-left font-normal`}>
                  {r.row}
                </th>
                <td className={`${td} text-right`}>{cell(r.eid_only)}</td>
                <td className={`${td} text-right font-medium`}>{cell(r.wasl)}</td>
                <td className={`${td} text-right`}>{cell(r.truth)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
      <p className="m-0 mt-3 text-small text-muted">
        Wasl raises the measured rate toward the true rate. The real rate moves only if someone acts on the corrected list.
      </p>
    </Section>
  );
}

function Accuracy({ ev }: { ev: Evidence }) {
  const { definitions, linkage, safety, worst } = ev.accuracy;
  const pairs = linkageRows(linkage);
  return (
    <Section id="accuracy" title="How accurate it is" aside={<HonestyLabel kind="simulated">Held-out persons, 5 seeds</HonestyLabel>}>
      <dl className="m-0 mt-3 grid gap-x-6 gap-y-2 text-small sm:grid-cols-2">
        {definitions.map((d) => (
          <div key={d.term}>
            <dt className="font-medium text-ink">{d.term}</dt>
            <dd className="m-0 text-ink-soft">{d.meaning}</dd>
          </div>
        ))}
      </dl>

      <div className="mt-5 grid items-start gap-gap lg:grid-cols-2">
        <div>
          <h3 className={h3}>Safety: errors that close a loop</h3>
          <table data-testid="safety" className="mt-2 w-full border-collapse text-body tabular-nums">
            <caption className="sr-only">Safety events, pooled over seeds, with a 95% upper bound where none were seen</caption>
            <thead>
              <tr>
                <th scope="col" className={th}>
                  Error
                </th>
                <th scope="col" className={`${th} text-right`}>
                  Seen
                </th>
                <th scope="col" className={`${th} text-right`}>
                  95% upper bound
                </th>
              </tr>
            </thead>
            <tbody>
              {safetyOrder(safety).map((s) => {
                const w = safetyWords(s);
                return (
                  <tr key={s.metric}>
                    <th scope="row" className={`${td} text-left font-normal`}>
                      {humanise(s.metric)}
                    </th>
                    <td className={`${td} whitespace-nowrap text-right font-medium`}>{w.count}</td>
                    <td className={`${td} text-right`}>{w.bound ? w.bound.replace("95% upper bound ", "") : <span aria-label="none">–</span>}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          <p className="m-0 mt-2 text-small text-muted">Zero events is never 100%: the bound is how high the true rate could still be.</p>
        </div>

        <div>
          <h3 className={h3}>Linkage error rates, mean and 95% interval</h3>
          <table data-testid="linkage" className="mt-2 w-full border-collapse text-body tabular-nums">
            <caption className="sr-only">Linkage error rates by noise model, mean over 5 seeds with a 95% t-interval</caption>
            <thead>
              <tr>
                <th scope="col" className={th}>
                  Rate
                </th>
                <th scope="col" className={`${th} text-right`}>
                  v1 noise <span className="block font-normal">tuned on</span>
                </th>
                <th scope="col" className={`${th} text-right`}>
                  v2 noise <span className="block font-normal">independent</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {pairs.map((p) => (
                <tr key={p.metric}>
                  <th scope="row" className={`${td} text-left font-normal`}>
                    {humanise(p.metric)}
                  </th>
                  {[p.v1, p.v2].map((l, i) => {
                    if (!l) return <td key={i} className={`${td} text-right`}><span aria-label="not run">–</span></td>;
                    const [value, interval] = withInterval(l).split(" ");
                    return (
                      <td key={i} className={`${td} text-right`}>
                        <span className="font-medium">{value}</span>
                        <span className="block text-small text-muted">{interval}</span>
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {worst.length > 0 && (
        <div className="mt-5">
          <h3 className={h3}>Worst cases, not just the mean</h3>
          <dl data-testid="worst" className="m-0 mt-2 grid gap-x-6 gap-y-2 sm:grid-cols-3">
            {worst.map((w) => (
              <div key={w.label}>
                <dt className="text-small text-muted">{w.label}</dt>
                <dd className="m-0 text-body font-medium tabular-nums">{w.value}</dd>
              </div>
            ))}
          </dl>
        </div>
      )}
    </Section>
  );
}

function Benchmarks({ ev }: { ev: Evidence }) {
  const rows = ev.benchmarks;
  return (
    <Section id="benchmarks" title="On data the team did not generate" aside={rows.length > 0 ? <HonestyLabel kind="built">Public datasets</HonestyLabel> : undefined}>
      {rows.length === 0 ? (
        <p data-testid="benchmarks-none" className="m-0 mt-2 text-body text-ink-soft">
          Not run yet. FEBRL4 and Arabic–English name-pair benchmarks are planned; no results are shown until they exist.
        </p>
      ) : (
        <table className="mt-3 w-full border-collapse text-body tabular-nums">
          <caption className="sr-only">Out-of-generator benchmarks</caption>
          <thead>
            <tr>
              <th scope="col" className={th}>
                Benchmark
              </th>
              <th scope="col" className={`${th} text-right`}>
                Wasl
              </th>
              <th scope="col" className={`${th} text-right`}>
                Baseline
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((b) => (
              <tr key={b.name}>
                <th scope="row" className={`${td} text-left font-normal`}>
                  <span className="font-medium">{b.name}</span>
                  <span className="block text-small text-muted">
                    {b.url ? <Ext href={b.url}>{b.dataset}</Ext> : b.dataset} · {b.metric}
                  </span>
                  {b.note && <span className="block text-small text-ink-soft">{b.note}</span>}
                </th>
                <td className={`${td} text-right font-medium`}>{num(b.wasl)}</td>
                <td className={`${td} text-right`}>
                  {b.baseline === null ? <span aria-label="none">–</span> : num(b.baseline)}
                  {b.baseline_source && <span className="block text-small text-muted">{b.baseline_source}</span>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Section>
  );
}

const SCENARIO_WORD = { worst: "Worst", base: "Base", best: "Best" } as const;

function Modelled({ ev }: { ev: Evidence }) {
  const m = ev.impact.modelled;
  const scenarios = scenarioOrder(m.scenarios);
  const measures: [string, (s: (typeof scenarios)[number]) => string][] = [
    ["Uplift in completion", (s) => `${num(s.uplift_pp)} pp`],
    ["CRC yield per colonoscopy", (s) => `${num(asPercent(s.ppv))}%`],
    ["Extra colonoscopies per 1,000", (s) => num(s.extra_colonoscopies_per_1000)],
    ["Cancers found per 1,000", (s) => num(s.cancers_found_per_1000)],
  ];
  return (
    <Section id="modelled" title="Modelled benefit" aside={<HonestyLabel kind="illustrative">Modelled, not measured</HonestyLabel>}>
      <p data-testid="modelled-label" className="m-0 mt-1 max-w-[68ch] text-body text-ink-soft">
        {m.label} The worst case is the measured gap alone. None of this is Wasl&apos;s direct effect.
      </p>
      <div className="mt-4 grid items-start gap-gap lg:grid-cols-2">
        <table data-testid="scenarios" className="w-full border-collapse text-body tabular-nums">
          <caption className="sr-only">Modelled scenarios per 1,000 positive FITs</caption>
          <thead>
            <tr>
              <th scope="col" className={th}>
                Scenario
              </th>
              {scenarios.map((s) => (
                <th key={s.name} scope="col" className={`${th} text-right`}>
                  {SCENARIO_WORD[s.name]}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {measures.map(([label, value]) => (
              <tr key={label}>
                <th scope="row" className={`${td} text-left font-normal`}>
                  {label}
                </th>
                {scenarios.map((s) => (
                  <td key={s.name} className={`${td} text-right`}>
                    {value(s)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
        {m.tornado.length > 0 && <TornadoChart rows={m.tornado} output="Cancers found per 1,000" />}
      </div>

      {m.parameters.length > 0 && (
        <details className="group mt-4">
          <summary className="inline-flex min-h-11 cursor-pointer items-center gap-1 text-small font-medium text-jade-600 md:min-h-8">
            <span aria-hidden className="inline-block transition-transform group-open:rotate-90">
              ›
            </span>
            Parameters and sources ({m.parameters.length})
          </summary>
          <ul className="m-0 mt-2 grid list-none gap-2 p-0 text-small">
            {m.parameters.map((p) => (
              <li key={p.name} className="flex flex-wrap items-baseline gap-x-2 gap-y-1 border-b border-rule pb-2">
                <span className="font-medium text-ink">{p.name}</span>
                <span className="tabular-nums">{p.value}</span>
                <span className="rounded-chip bg-raised px-1.5 text-ink-soft">{p.scope === "ASSUMED" ? "Assumed" : p.scope === "UAE" ? "UAE source" : "International source"}</span>
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
  const label = <span className="[overflow-wrap:anywhere]">{claim.evidence}</span>;
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
  return (
    <Section id="claims" title="Claim ledger">
      <p className="m-0 mt-1 text-body text-ink-soft">Every claim, how true it is, where the evidence is and how to rerun it.</p>
      {/* Desktop: a table. Phone: the same rows as a list, so nothing scrolls sideways. */}
      <table data-testid="claims-table" className="mt-3 hidden w-full border-collapse text-body md:table">
        <caption className="sr-only">Claim ledger</caption>
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
              Reproduce
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
              <span className="text-muted">Reproduce: </span>
              <Reproduce text={c.reproduce} />
            </p>
          </li>
        ))}
      </ul>
    </Section>
  );
}

function Verify() {
  return (
    <Section id="verify" title="Verify it yourself in two minutes">
      <div className="mt-3 grid items-start gap-gap lg:grid-cols-2">
        <div>
          <h3 className={h3}>In the hosted app</h3>
          <ol className="m-0 mt-2 grid list-decimal gap-1.5 pl-5 text-body">
            <li>
              This page: the measured gap, error rates and claims, from <span className="whitespace-nowrap">evidence.json</span>.
            </li>
            <li>
              <Link href="/app/results" className={link}>
                Results
              </Link>
              : the frozen held-out evaluation behind these numbers.
            </li>
            <li>
              <Link href="/app/pipeline" className={link}>
                Pipeline
              </Link>
              : how a record is matched, read and closed.
            </li>
            <li>
              <Link href="/app/audit" className={link}>
                Audit trail
              </Link>
              : the hash-chained record of every decision.
            </li>
          </ol>
          <p className="m-0 mt-2 text-small text-muted">
            Hosted at <Ext href={HOSTED}>{HOSTED.replace(/^https:\/\//, "")}</Ext>.
          </p>
        </div>
        <div>
          <h3 className={h3}>On your machine</h3>
          <p className="m-0 mt-2 text-small text-ink-soft">Regenerate the held-out seeds and check every number against the committed hashes:</p>
          <pre data-testid="verify-cmd" className="m-0 mt-1.5 whitespace-pre-wrap break-all rounded-control bg-raised px-3 py-2 text-small">
            <code className="font-mono">git clone {REPO} && cd wasl-futurehealth && make verify</code>
          </pre>
          <p className="m-0 mt-3 text-small text-ink-soft">Pick any seed nobody has seen, so the team cannot have tuned for it:</p>
          <pre className="m-0 mt-1.5 whitespace-pre-wrap break-all rounded-control bg-raised px-3 py-2 text-small">
            <code className="font-mono">make verify SEED=&lt;n&gt;</code>
          </pre>
          <p className="m-0 mt-3 text-small">
            <Ext href={REPO}>GitHub repository</Ext> · <Ext href={`${REPO}/blob/main/docs/EVIDENCE.md`}>docs/EVIDENCE.md</Ext>
          </p>
        </div>
      </div>
    </Section>
  );
}

export default function EvidencePage() {
  const ev = loadEvidence();
  return (
    <>
      <PageHeader
        title="Evidence"
        sub={`What Wasl measured, how sure we are, and how to check it. Frozen at code ${ev.generated.revision.slice(0, 7)} on ${frozenWords(ev.generated.frozen_at)}, before the held-out runs.`}
        aside={<HonestyLabel kind="simulated">Synthetic patients</HonestyLabel>}
      />
      <div className="grid gap-gap">
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
