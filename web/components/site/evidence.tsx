import Link from "next/link";
import { programmeView, stallScope } from "@/components/programme/model";
import { ownerLabel } from "@/components/worklist/model";
import { getDashboard } from "@/lib/api/source";
import { result } from "@/lib/results";
import { cn } from "@/lib/utils";
import { exactlyOnce, fixed, frozenOn, published, QUOTED, upperBound } from "./figures";
import { GrowBar } from "./grow-bar";
import { Reveal } from "./reveal";
import { wrapEvidence } from "./styles";
import { TauPicker } from "./tau-picker";

const eyebrow = "m-0 text-[14px] font-semibold uppercase tracking-[0.1em] text-(--teal)";
const h2x = "m-0 mt-3 max-w-[22ch] text-[clamp(2rem,1.4rem_+_2vw,3.25rem)] font-bold leading-[1.05] tracking-[-0.03em] text-balance";
const big = "text-[clamp(2.5rem,1.8rem_+_2.4vw,4rem)] font-bold leading-none tracking-[-0.035em] tabular-nums";
const mid = "text-[clamp(1.75rem,1.3rem_+_1.2vw,2.5rem)] font-bold leading-none tracking-[-0.03em] tabular-nums";
const note = "m-0 text-[14px] leading-[1.5] text-(--muted)";
const card = "rounded bg-(--card) p-[clamp(1.25rem,2.2vw,2rem)]";
const sectionY = "py-20 md:py-28";

type Mark = "built" | "simulated" | "designed";

/** Status as shape plus word: filled square built, half square simulated, dashed outline designed. */
function ClaimMark({ mark, children }: { mark: Mark; children?: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5 text-[14px]" data-status={mark}>
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
      {children ?? mark[0].toUpperCase() + mark.slice(1)}
    </span>
  );
}

/** One labelled horizontal bar; `max` sets the scale shared by its siblings. */
function HBar({ label, value, n, max, tone, strong, delay }: { label: string; value: string; n: number; max: number; tone: string; strong?: boolean; delay?: number }) {
  return (
    <div>
      <div className="flex items-baseline justify-between gap-4 text-[16px]">
        <span className={cn(strong && "font-semibold")}>{label}</span>
        <span className={cn("font-semibold tabular-nums", strong && "text-(--teal)")}>{value}</span>
      </div>
      <div className="mt-1.5 h-6 rounded-xs bg-(--sand)">
        <GrowBar pct={(n / max) * 100} className={tone} delay={delay} />
      </div>
    </div>
  );
}

function Head({ kicker, title, id, children }: { kicker: string; title: string; id: string; children?: React.ReactNode }) {
  return (
    <div className="grid items-end gap-x-16 gap-y-4 md:grid-cols-[1.3fr_1fr]">
      <div>
        <p className={eyebrow}>{kicker}</p>
        <h2 id={id} className={h2x}>
          {title}
        </h2>
      </div>
      {children && <p className="m-0 max-w-[34em] text-[18px] leading-[1.5] text-(--ink-soft)">{children}</p>}
    </div>
  );
}

/** The expert evidence, condensed for a scroll-through pitch. It sits under the landing story, so it shows once the story ends. */
export async function Evidence() {
  const dash = await getDashboard();
  const kpi = dash.metrics.crc_kpi_31d;
  const view = programmeView(dash);
  const scope = stallScope(dash);
  const demo = view.kpi.demo.rates;
  const pct = (v: number) => `${v.toFixed(1)}%`;
  const missed = Math.round((1 - demo.eid / demo.truth) * 100);
  const fc = result("v2.safety.crc_false_closures");
  const once = exactlyOnce();
  const corpora = result("reader.corpora").value;
  const stalls = [...scope.top, ...scope.smaller];
  const stallMax = Math.max(...stalls.map((s) => s.count));
  // tests/site.test.ts checks all five scored fields share this kappa.
  const kappa = result("labelling.kappa.caecum").display;

  const OUTCOMES: { title: string; marks: [Mark, string?][]; figure: React.ReactNode; caption: React.ReactNode; gap: string }[] = [
    {
      title: "End-to-end visibility",
      marks: [["built"]],
      figure: (
        <div className="grid gap-3">
          <HBar label="KPI error, Wasl" value={fixed("v2.kpi_abs_error_wasl", 3)} n={result("v2.kpi_abs_error_wasl").value} max={result("v2.kpi_abs_error_eid_only").value} tone="bg-(--teal)" strong />
          <HBar label="KPI error, ID only" value={fixed("v2.kpi_abs_error_eid_only", 3)} n={result("v2.kpi_abs_error_eid_only").value} max={result("v2.kpi_abs_error_eid_only").value} tone="bg-(--stub)" delay={0.1} />
        </div>
      ),
      caption: "One case per FIT+, every provider, no Emirates ID needed.",
      gap: "Treatment start not tracked yet.",
    },
    {
      title: "Ownership, on time",
      marks: [["built"]],
      figure: (
        <div className="grid gap-2.5" data-testid="stalls">
          {stalls.map((s, i) => (
            <HBar key={s.dropoff} label={s.label} value={s.count.toLocaleString("en-US")} n={s.count} max={stallMax} tone={i < scope.top.length ? "bg-(--held)" : "bg-(--stub)"} delay={i * 0.06} />
          ))}
        </div>
      ),
      caption: (
        <>
          Open-case stalls, {scope.cases.toLocaleString("en-US")} simulated cases. Unowned cases show {ownerLabel("")}. Overdue escalates to the medical director.
        </>
      ),
      gap: "Some deadlines are our defaults.",
    },
    {
      title: "Patient communication",
      marks: [["built"], ["designed", "Patient measures"]],
      figure: (
        <>
          <div className={mid}>{result("v2.safety.released_without_granted_consent").display}</div>
          <div className="mt-2 text-[16px]">messages sent without consent</div>
        </>
      ),
      caption: "Owner-named notices in the patient's language. Teach-back parser 98 / 100; failed teach-back triggers a nurse callback.",
      gap: "Non-English notices machine-drafted.",
    },
    {
      title: "Cross-provider hand-off",
      marks: [["built"], ["simulated", "Referrers"]],
      figure: (
        <>
          <div className={mid}>
            {result("v2.counts.handoff_packets_matched").display} / {result("v2.counts.handoff_truth_packets").display}
          </div>
          <div className="mt-2 text-[16px]">hand-off packets, 0 extra</div>
        </>
      ),
      caption: <>Rejected referral returns to the referrer with an act-by date. {result("v2.safety.handoff_false_closures").display} false closures after re-referral.</>,
      gap: "No clinician messaging screen yet.",
    },
    {
      title: "Human-governed escalation",
      marks: [["built"]],
      figure: (
        <>
          <div className={mid}>{fc.display}</div>
          <div className="mt-2 text-[16px]">false closures</div>
        </>
      ),
      caption: "Unsure means two human reviewers. Suspected cancer never waits for histology.",
      gap: "No urgent-triage view yet.",
    },
    {
      title: "Closure, measured",
      marks: [["built"], ["simulated", "Uplift"]],
      figure: (
        <>
          <div className={mid}>
            {pct(demo.eid)} → {pct(demo.wasl)}
          </div>
          <div className="mt-2 text-[16px]">31-day rate, ID-only vs Wasl</div>
        </>
      ),
      caption: "At 1.2 slots per FIT+: queue levers +13 pp; pooling 21.5% → 41.7%. Both simulated.",
      gap: "Uplift sizes borrowed from trials.",
    },
  ];

  const HARDENED = [
    { n: once.display, what: "outputs lost or duplicated under crash tests", figure: "exactly-once" },
    { n: result("hl7.resources_exactly_once").display, what: "HL7 messages, each delivered once; 888 re-sends absorbed" },
    { n: result("v2.safety.tamper_detected").display, what: `ledger tampering caught; ${result("v2.safety.tamper_false_alarms").display} false alarms` },
    { n: result("v2.safety.forged_with_effect").display, what: "forged sign-offs took effect" },
  ];

  const PLAN = [
    { when: "Months 0–3", tone: "bg-(--mint) text-(--ink)", title: "Shadow pilot", text: "Unpaid. Coordinator’s list as reference.", gate: "0 false closures, recall ≥ 0.95" },
    { when: "Months 4–9", tone: "bg-(--teal) text-(--paper)", title: "Live outreach", text: "Stepped-wedge, 6–8 PHCs.", gate: "Uplift lower bound above 0" },
    { when: "Months 9–12", tone: "bg-(--ink) text-(--paper)", title: "Contract", text: "ADHDS vendor agreement; DoH tender fallback.", gate: "Signed" },
    { when: "Month 12 on", tone: "bg-(--sand-deep) text-(--ink)", title: "Cervical, breast", text: "Year-1 AED 0.54–1.39M falls toward AED 5–28 per loop.", gate: "Same gate, shadow first" },
  ];

  const LIMITS: { limit: string; fix: string; when: string }[] = [
    { limit: "Patients, reviewers, capacity all simulated. No real-patient result.", fix: "Shadow pilot vs coordinator's list. Gate: 0 false closures, recall ≥ 0.95.", when: "Months 0–3" },
    {
      limit: `${result("labelling.reports").display} clinically verified model-assisted labels: two model passes, clinician adjudication, κ = ${kappa} on all five scored fields. Not independent ground truth; small sample.`,
      fix: "Independent human labels, larger local set.",
      when: "Months 0–3",
    },
    { limit: "Certificate covers synthetic loops only.", fix: "Recalibrate on ~3.7k local loops.", when: "Months 0–3" },
    { limit: `Readers broke on 3 of ${corpora} unseen corpora before fixes.`, fix: `${QUOTED.pilotStreak.value} confirmed closures in a row, then 10% audit.`, when: "Months 0–9" },
    { limit: `Review load by name origin ${QUOTED.originPooled.value} vs 1.25 limit (p = 0.007); ${QUOTED.originStandardised.value} by programme (p = 0.185).`, fix: "Reported every run.", when: "Every run" },
    { limit: "Female-list access gap 12.6 pp vs 5 pp limit (simulated).", fix: "Needs 30–40% female lists. Wasl reports it; can't fix it.", when: "To DoH" },
    { limit: "Under ~1 colonoscopy slot per FIT+, no software helps.", fix: "Shortfall quantified by region.", when: "To DoH" },
    { limit: "No real Malaffi feed accessed yet.", fix: "DDGO assessment alongside pilot.", when: "Months 0–3" },
  ];

  return (
    <div id="evidence" className="scroll-mt-4" data-testid="evidence">
      {/* Opening: the claim, the status words, then the headline chart. */}
      <section aria-labelledby="ev-h" className={cn(wrapEvidence, sectionY)}>
        <Reveal>
          <Head kicker="The evidence" title="Every FIT-positive, followed to closure" id="ev-h">
            No Emirates ID needed. Auto-closes only when certain; the rest go to two humans. Simulated cohort, 5 seeds.
          </Head>
          <div className="mt-8 flex flex-wrap items-center gap-x-6 gap-y-2 text-(--ink-soft)">
            <ClaimMark mark="built">Built: ran end-to-end, synthetic cohorts</ClaimMark>
            <ClaimMark mark="simulated">Simulated: modelled, assumed inputs</ClaimMark>
            <ClaimMark mark="designed">Designed: specified, not built</ClaimMark>
          </div>
        </Reveal>

        <Reveal className="mt-14 grid items-stretch gap-6 lg:grid-cols-[1.5fr_1fr]">
          <figure
            role="img"
            aria-label={`Colonoscopy within 31 days of referral, simulated cohort of ${kpi.truth.eligible_referrals.toLocaleString("en-US")}: Emirates ID only ${pct(demo.eid)}, Wasl ${pct(demo.wasl)}, truth ${pct(demo.truth)}`}
            className={cn(card, "m-0")}
            data-testid="kpi"
          >
            <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
              <span className="text-[20px] font-semibold tracking-[-0.01em]">Colonoscopy within 31 days of referral</span>
              <span className={note}>Simulated, n = {kpi.truth.eligible_referrals.toLocaleString("en-US")}</span>
            </div>
            <div className="mt-7 grid gap-5">
              <HBar label="Seen by Emirates ID tracking" value={pct(demo.eid)} n={demo.eid} max={40} tone="bg-(--stub)" />
              <HBar label="Seen by Wasl" value={pct(demo.wasl)} n={demo.wasl} max={40} tone="bg-(--teal)" strong delay={0.15} />
              <HBar label="Truth" value={pct(demo.truth)} n={demo.truth} max={40} tone="bg-(--ink)" delay={0.3} />
            </div>
          </figure>
          <div className={cn(card, "flex flex-col justify-between gap-8 bg-(--mint)")}>
            <div>
              <div className={big}>{missed}%</div>
              <p className="m-0 mt-3 text-[18px] leading-[1.45]">
                of real completions look lost to ID-only tracking
                {view.chaseOneIn !== null && <>. About 1 in {view.chaseOneIn} on the chase list is already done</>}.
              </p>
            </div>
            <p className="m-0 text-[16px] leading-[1.5] text-(--ink-soft)">
              Abu Dhabi today: {QUOTED.baselineRate.value} of FIT+ get a colonoscopy; median wait {published("crc_followup_median_wait_days")} days vs a{" "}
              {published("crc_followup_standard_days")}-day standard. <span className="text-(--muted)">SEHA, BMC Cancer 2021.</span>
            </p>
          </div>
        </Reveal>
      </section>

      {/* Safety: the one irreversible action. */}
      <section id="safety" aria-labelledby="safety-h" className="scroll-mt-4 bg-(--sand)">
        <Reveal className={cn(wrapEvidence, sectionY)}>
          <Head kicker="Safety" title="The gate refuses to guess" id="safety-h">
            Closure is irreversible. Every check passes, or no auto-close.
          </Head>
          <div className="mt-12 grid items-start gap-6 lg:grid-cols-3">
            <div className={card}>
              <div className="text-[16px] text-(--ink-soft)">False closures, held-out noise, 5 seeds</div>
              <div className={cn(big, "mt-3")} data-figure="false-closures">
                {fc.display}
              </div>
              <p className={cn(note, "mt-3")}>
                95% upper bound {upperBound("v2.safety.crc_false_closures")}. Precision {fixed("v2.crc_precision", 3)}, recall {fixed("v2.crc_recall", 3)}.
              </p>
              <ul className="m-0 mt-6 grid list-none gap-2 p-0 text-[16px] tabular-nums">
                <li>
                  <strong className="font-semibold">{result("v2.safety.wrong_patient_released").display}</strong> <span className="text-(--ink-soft)">wrong-patient messages</span>
                </li>
                <li>
                  <strong className="font-semibold">{result("v2.safety.lung_false_closures").display}</strong> <span className="text-(--ink-soft)">lung, same engine</span>
                </li>
                <li>
                  <strong className="font-semibold">{result("v2.safety.cervical_false_closures").display}</strong> <span className="text-(--ink-soft)">cervical, simulated</span>
                </li>
              </ul>
            </div>
            <div className={card}>
              <div className="text-[16px] text-(--ink-soft)">Gate sweep: false closures by review mode</div>
              <div className="mt-5 grid grid-cols-2 gap-4">
                <div>
                  <div className={big}>0</div>
                  <div className="mt-2 text-[16px]">Double review, of {result("double_review.false_closures_denominator").display}</div>
                </div>
                <div>
                  <div className={cn(big, "text-(--negative)")}>{result("single_review.false_closures").display}</div>
                  <div className="mt-2 text-[16px]">Single review</div>
                </div>
              </div>
              <p className="m-0 mt-6 text-[16px] font-semibold">Double review: mandatory.</p>
            </div>
            <div className={card}>
              <div className="text-[16px] text-(--ink-soft)">Auto-closes only if all hold</div>
              <ul className="m-0 mt-4 grid list-none gap-3 p-0 text-[16px] leading-[1.4]">
                {[
                  "Trusted link, τ ≥ 0.99. A name alone never closes.",
                  "Two independent readers agree, field by field.",
                  "Second identifier before any message.",
                  "Images can hold a loop, never close one.",
                  "Suspected cancer never waits for histology.",
                ].map((t) => (
                  <li key={t} className="flex gap-2.5">
                    <svg aria-hidden viewBox="0 0 16 16" className="mt-1 size-4 flex-none">
                      <path d="m3 8.5 3 3 7-7" fill="none" stroke="var(--teal)" strokeWidth="2.2" />
                    </svg>
                    {t}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </Reveal>

        <Reveal className={cn(wrapEvidence, "pb-20 md:pb-28")}>
          <div className="grid items-start gap-6 lg:grid-cols-[1.4fr_1fr]">
            <div>
              <h3 className="m-0 text-[24px] font-bold tracking-[-0.01em]">Certified threshold. Pick a tier.</h3>
              <p className="m-0 mt-2 max-w-[40em] text-[16px] leading-[1.5] text-(--ink-soft)">
                {result("ltt.calibration_loops").display} calibration loops. False closures and wrong-patient messages each bounded at 0.1%, 95% confidence.
              </p>
              <TauPicker />
              <p className={cn(note, "mt-3")}>Synthetic calibration set only. Pilot recalibrates on ~3.7k local loops.</p>
            </div>
            <div className={card}>
              <h3 className="m-0 text-[20px] font-semibold">Attacked with unseen reports</h3>
              <div className="mt-5 grid grid-cols-7 gap-1.5" role="img" aria-label={`${corpora} unseen report corpora: 3 exposed an unsafe phrasing and were fixed, the rest passed`}>
                {Array.from({ length: corpora }, (_, i) => (
                  <div key={i} className={cn("aspect-square rounded-xs", i < 3 ? "bg-(--held)" : "bg-(--teal)")} />
                ))}
              </div>
              <div className="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-[14px]">
                <span className="inline-flex items-center gap-1.5">
                  <span aria-hidden className="size-3 rounded-xs bg-(--held)" /> Broke, then fixed
                </span>
                <span className="inline-flex items-center gap-1.5">
                  <span aria-hidden className="size-3 rounded-xs bg-(--teal)" /> Passed
                </span>
              </div>
              <p className="m-0 mt-5 text-[16px] leading-[1.5]">Each fix a generic rule. Now 0 unsafe closures on every corpus.</p>
              <h4 className="m-0 mt-6 text-[16px] font-semibold">Changed after failures, disclosed</h4>
              <ul className="m-0 mt-2 grid gap-1.5 pl-5 text-[16px] leading-[1.45] text-(--ink-soft)">
                <li>τ 0.98 → 0.99 after one wrong-patient closure.</li>
                <li>Pending histology no longer skipped at day 15.</li>
                <li>Short surnames need a matching first name.</li>
                <li>Ungraded prep waits for the unit; recall now {fixed("v2.crc_recall", 3)}.</li>
              </ul>
            </div>
          </div>
        </Reveal>
      </section>

      {/* The challenge's six outcomes, one card each. */}
      <section id="outcomes" aria-labelledby="outcomes-h" className={cn(wrapEvidence, sectionY, "scroll-mt-4")}>
        <Reveal>
          <Head kicker="Challenge #5" title="Six outcomes, answered" id="outcomes-h" />
          <ul className="m-0 mt-12 grid list-none gap-6 p-0 md:grid-cols-2 xl:grid-cols-3" data-testid="outcomes">
            {OUTCOMES.map((o) => (
              <li key={o.title} className={cn(card, "flex flex-col")}>
                <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
                  <h3 className="m-0 text-[20px] font-semibold leading-[1.25] tracking-[-0.01em]">{o.title}</h3>
                  <div className="flex flex-wrap gap-x-3 gap-y-1 text-(--ink-soft)">
                    {o.marks.map(([m, label], i) => (
                      <ClaimMark key={i} mark={m}>
                        {label}
                      </ClaimMark>
                    ))}
                  </div>
                </div>
                <div className="mt-6">{o.figure}</div>
                <p className="m-0 mt-5 text-[16px] leading-[1.5]">{o.caption}</p>
                <p className={cn(note, "mt-auto pt-4")}>
                  <span className="font-semibold text-(--ink-soft)">Gap:</span> {o.gap}
                </p>
              </li>
            ))}
          </ul>
          <p className={cn(note, "mt-6")}>Demo run {scope.run}.</p>
        </Reveal>
      </section>

      {/* Robustness figures, no architecture. */}
      <section aria-labelledby="hard-h" className="bg-(--ink) text-(--paper)">
        <Reveal className={cn(wrapEvidence, "py-16 md:py-20")}>
          <h2 id="hard-h" className="m-0 text-[24px] font-bold tracking-[-0.01em]">
            Broken on purpose, still exact
          </h2>
          <p className="m-0 mt-2 text-[14px] opacity-80">Built; synthetic loads.</p>
          <dl className="m-0 mt-8 grid gap-8 sm:grid-cols-2 lg:grid-cols-4">
            {HARDENED.map((h) => (
              <div key={h.what} className="flex flex-col-reverse">
                <dt className="mt-2 text-[16px] leading-[1.4] opacity-80">{h.what}</dt>
                <dd className={cn(mid, "m-0")} data-figure={h.figure}>
                  {h.n}
                </dd>
              </div>
            ))}
          </dl>
        </Reveal>
      </section>

      {/* Pilot plan. */}
      <section id="plan" aria-labelledby="plan-h" className={cn(wrapEvidence, sectionY, "scroll-mt-4")}>
        <Reveal>
          <Head kicker="Roadmap" title="Four stages, each gated" id="plan-h">
            Simulated shadow pilot: {QUOTED.pilotStreak.value} correct closures in a row by day 120.
          </Head>
          <ol aria-label="Pilot stages by month" className="m-0 mt-12 grid list-none gap-2 p-0 max-md:gap-8 md:grid-cols-[3fr_6fr_3fr_3fr]">
            {PLAN.map((p) => (
              <li key={p.when} data-stage={p.when}>
                <div className={cn("rounded px-4 py-3 text-[16px] font-semibold", p.tone)}>{p.when}</div>
                <div className="pr-4 pt-4">
                  <div className="text-[20px] font-semibold leading-[1.25]">{p.title}</div>
                  <p className="m-0 mt-1.5 text-[16px] leading-[1.45] text-(--ink-soft)">{p.text}</p>
                  <p className="m-0 mt-3 text-[16px] leading-[1.45]">
                    <span className="font-semibold text-(--teal)">Gate</span> {p.gate}
                  </p>
                </div>
              </li>
            ))}
          </ol>
        </Reveal>
      </section>

      {/* Limits, each paired with what settles it. */}
      <section id="limits" aria-labelledby="limits-h" className="scroll-mt-4 bg-(--sand)">
        <Reveal className={cn(wrapEvidence, sectionY)}>
          <Head kicker="Limits" title="Not proven yet, and what settles it" id="limits-h" />
          <ul className="m-0 mt-10 grid list-none gap-0 p-0">
            {LIMITS.map((l) => (
              <li key={l.limit} className="grid gap-x-8 gap-y-1.5 border-t border-(--line) py-4 md:grid-cols-[1.2fr_1fr_9rem]">
                <span className="text-[16px] leading-[1.45]">{l.limit}</span>
                <span className="text-[16px] leading-[1.45] text-(--ink-soft)">→ {l.fix}</span>
                <span className="text-[14px] font-semibold md:text-right">{l.when}</span>
              </li>
            ))}
          </ul>
        </Reveal>
      </section>

      {/* The ask. */}
      <section id="pilot" aria-labelledby="pilot-h" className="scroll-mt-4 bg-(--sand-deep)">
        <Reveal className={cn(wrapEvidence, "grid items-start gap-12 py-20 md:py-28 lg:grid-cols-[1.2fr_1fr] lg:gap-20")}>
          <div>
            <h2 id="pilot-h" className="m-0 text-[clamp(2rem,1.4rem_+_2vw,3.25rem)] font-bold leading-[1.05] tracking-[-0.03em] text-balance">
              One false closure ends the pilot
            </h2>
            <p className="m-0 mt-5 max-w-[28em] text-[20px] leading-normal">
              A free three-month shadow pilot. One clinic, one endoscopy unit, a clinician confirming every closure.
            </p>
            <Link
              href="/app"
              className="mt-8 inline-block rounded bg-(--ink) px-6 py-4 text-[16px] font-semibold text-(--paper) no-underline hover:bg-(--teal) hover:text-(--paper)"
            >
              Open the dashboard
            </Link>
          </div>
          <div>
            <h3 className="m-0 text-[20px] font-semibold">What we need</h3>
            <ul className="m-0 mt-3 grid gap-2.5 pl-5 text-[16px] leading-normal">
              <li>One PHC and one endoscopy unit, in shadow mode.</li>
              <li>The coordinator’s FIT+ list as the reference.</li>
              <li>A route to Malaffi test messages and the DDGO assessment.</li>
              <li>Two clinicians to sign held loops.</li>
            </ul>
          </div>
        </Reveal>
      </section>

      <p className={cn(wrapEvidence, note, "pt-10")} data-testid="methods">
        Synthetic cohort; no outcomes on real patients claimed. Frozen evaluation of {frozenOn()}: seeds 11, 23, 37, 41 and 59, held-out noise model v2. KPI, drop-offs, pilot
        streak: single integrated runs. Gate sweep, reader corpora, certificate: project’s results files.
        {once.contested && " Exactly-once count is the held-out figure; sources disagree on the total."}
      </p>
    </div>
  );
}
