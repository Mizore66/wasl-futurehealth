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

  const OUTCOMES: { title: string; marks: [Mark, string?][]; figure: React.ReactNode; caption: React.ReactNode; gap: string }[] = [
    {
      title: "End-to-end visibility",
      marks: [["built"]],
      figure: (
        <div className="grid gap-3">
          <HBar label="Error in the 31-day rate, Wasl" value={fixed("v2.kpi_abs_error_wasl", 3)} n={result("v2.kpi_abs_error_wasl").value} max={result("v2.kpi_abs_error_eid_only").value} tone="bg-(--teal)" strong />
          <HBar label="Error in the 31-day rate, Emirates ID only" value={fixed("v2.kpi_abs_error_eid_only", 3)} n={result("v2.kpi_abs_error_eid_only").value} max={result("v2.kpi_abs_error_eid_only").value} tone="bg-(--stub)" delay={0.1} />
        </div>
      ),
      caption: "One case per FIT+ result, across every provider, without needing an Emirates ID.",
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
          Where open cases get stuck, across {scope.cases.toLocaleString("en-US")} simulated cases. A case with no owner shows {ownerLabel("")}. Overdue cases go to the medical director.
        </>
      ),
      gap: "Some deadlines are our own defaults.",
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
      caption: "Messages in the patient's language, naming who looks after them. Teach-back read correctly 98 / 100 times; if the patient got it wrong, a nurse calls back.",
      gap: "Non-English messages are drafted by machine.",
    },
    {
      title: "Cross-provider hand-off",
      marks: [["built"], ["simulated", "Referrers"]],
      figure: (
        <>
          <div className={mid}>
            {result("v2.counts.handoff_packets_matched").display} / {result("v2.counts.handoff_truth_packets").display}
          </div>
          <div className="mt-2 text-[16px]">referrals handed on to the next provider, none extra</div>
        </>
      ),
      caption: <>A rejected referral goes back to the referring doctor with a date to act by. {result("v2.safety.handoff_false_closures").display} cases closed wrongly after re-referral.</>,
      gap: "No clinician messaging screen yet.",
    },
    {
      title: "People decide the hard cases",
      marks: [["built"]],
      figure: (
        <>
          <div className={mid}>{fc.display}</div>
          <div className="mt-2 text-[16px]">cases closed wrongly</div>
        </>
      ),
      caption: "When Wasl is unsure, two people review. Suspected cancer never waits for histology.",
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
          <div className="mt-2 text-[16px]">colonoscopy within 31 days, Emirates ID only vs Wasl</div>
        </>
      ),
      caption: "With 1.2 colonoscopy slots per FIT+: better queue management adds 13 points; booking across a group's hospitals takes it from 21.5% to 41.7%. Both simulated.",
      gap: "The size of the gain is taken from published trials.",
    },
  ];

  const HARDENED = [
    { n: once.display, what: "results lost or counted twice when we made the system crash", figure: "exactly-once" },
    { n: result("hl7.resources_exactly_once").display, what: "hospital messages, each received once; 888 repeats ignored" },
    { n: result("v2.safety.tamper_detected").display, what: `attempts to alter the record caught; ${result("v2.safety.tamper_false_alarms").display} false alarms` },
    { n: result("v2.safety.forged_with_effect").display, what: "fake sign-offs that took effect" },
  ];

  const PLAN = [
    { when: "Months 0–3", tone: "bg-(--mint) text-(--ink)", title: "Trial alongside staff", text: "Free. Wasl runs beside the coordinator’s own list, which stays in charge.", gate: "No wrong closures; finds at least 95% of completed checks" },
    { when: "Months 4–9", tone: "bg-(--teal) text-(--paper)", title: "Live outreach", text: "Rolled out clinic by clinic across 6–8 primary care centres.", gate: "Clear proof that more people complete their check" },
    { when: "Months 9–12", tone: "bg-(--ink) text-(--paper)", title: "Contract", text: "Agreement with ADHDS, or a DoH tender.", gate: "Signed" },
    { when: "Month 12 on", tone: "bg-(--sand-deep) text-(--ink)", title: "Cervical, breast", text: "First-year cost AED 0.54–1.39M, falling toward AED 5–28 per patient followed.", gate: "Same tests, alongside staff first" },
  ];

  const LIMITS: { limit: string; fix: string; when: string }[] = [
    { limit: "Patients, reviewers and clinic capacity are all simulated. No results on real patients yet.", fix: "Run beside the coordinator's list. To pass: no wrong closures, at least 95% of completed checks found.", when: "Months 0–3" },
    {
      limit: `${result("labelling.reports").display} reports labelled with AI help, then checked by a clinician, with full agreement after review. Not an independent second labeller, and a small sample.`,
      fix: "Staff label a larger set of local reports independently.",
      when: "Months 0–3",
    },
    { limit: "The safety level is proven on simulated cases only.", fix: "Retest on about 3.7k local cases.", when: "Months 0–3" },
    { limit: `The report readers made unsafe mistakes on 3 of ${corpora} sets of new reports before fixes.`, fix: `A clinician confirms every closure until ${QUOTED.pilotStreak.value} in a row are right, then checks 10% at random.`, when: "Months 0–9" },
    { limit: `Some name origins are sent to human review more often: ${QUOTED.originPooled.value} times as often overall, against a 1.25 limit (p = 0.007); ${QUOTED.originStandardised.value} within each programme (p = 0.185).`, fix: "Reported every time.", when: "Ongoing" },
    { limit: "Access gap for women who need a female list: 12.6 points against a 5-point limit (simulated).", fix: "Needs 30–40% of lists to be female lists. Wasl reports the gap; it cannot fix it.", when: "To DoH" },
    { limit: "With fewer than about 1 colonoscopy slot per FIT+, no software helps.", fix: "Wasl shows the shortfall by region.", when: "To DoH" },
    { limit: "Not yet connected to real Malaffi data.", fix: "DDGO data review alongside the trial.", when: "Months 0–3" },
  ];

  return (
    <div id="evidence" className="scroll-mt-4" data-testid="evidence">
      {/* Opening: the claim, the status words, then the headline chart. */}
      <section aria-labelledby="ev-h" className={cn(wrapEvidence, sectionY)}>
        <Reveal>
          <Head kicker="The evidence" title="Every FIT-positive patient, followed to a documented end" id="ev-h">
            No Emirates ID needed. Wasl closes a case on its own only when it is certain; otherwise two people decide. Tested on simulated patients.
          </Head>
          <div className="mt-8 flex flex-wrap items-center gap-x-6 gap-y-2 text-(--ink-soft)">
            <ClaimMark mark="built">Built: works today, tested on simulated patients</ClaimMark>
            <ClaimMark mark="simulated">Simulated: estimated with a model</ClaimMark>
            <ClaimMark mark="designed">Designed: planned, not built yet</ClaimMark>
          </div>
        </Reveal>

        <Reveal className="mt-14 grid items-stretch gap-6 lg:grid-cols-[1.5fr_1fr]">
          <figure
            role="img"
            aria-label={`Colonoscopy within 31 days of referral, ${kpi.truth.eligible_referrals.toLocaleString("en-US")} simulated patients: Emirates ID only ${pct(demo.eid)}, Wasl ${pct(demo.wasl)}, what really happened ${pct(demo.truth)}`}
            className={cn(card, "m-0")}
            data-testid="kpi"
          >
            <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
              <span className="text-[20px] font-semibold tracking-[-0.01em]">Colonoscopy within 31 days of referral</span>
              <span className={note}>Simulated, {kpi.truth.eligible_referrals.toLocaleString("en-US")} patients</span>
            </div>
            <div className="mt-7 grid gap-5">
              <HBar label="Seen when matching by Emirates ID" value={pct(demo.eid)} n={demo.eid} max={40} tone="bg-(--stub)" />
              <HBar label="Seen by Wasl" value={pct(demo.wasl)} n={demo.wasl} max={40} tone="bg-(--teal)" strong delay={0.15} />
              <HBar label="What really happened" value={pct(demo.truth)} n={demo.truth} max={40} tone="bg-(--ink)" delay={0.3} />
            </div>
          </figure>
          <div className={cn(card, "flex flex-col justify-between gap-8 bg-(--mint)")}>
            <div>
              <div className={big}>{missed}%</div>
              <p className="m-0 mt-3 text-[18px] leading-[1.45]">
                of completed checks look missed when records are matched by Emirates ID alone
                {view.chaseOneIn !== null && <>. About 1 in {view.chaseOneIn} people on the call list has already had the check</>}.
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
          <Head kicker="Safety" title="Wasl does not guess" id="safety-h">
            Closing a case ends follow-up. Wasl closes one on its own only when every check passes.
          </Head>
          <div className="mt-12 grid items-start gap-6 lg:grid-cols-3">
            <div className={card}>
              <div className="text-[16px] text-(--ink-soft)">Cases closed wrongly, in testing</div>
              <div className={cn(big, "mt-3")} data-figure="false-closures">
                {fc.display}
              </div>
              <p className={cn(note, "mt-3")}>
                At most {upperBound("v2.safety.crc_false_closures")}, with 95% confidence.
              </p>
              <ul className="m-0 mt-6 grid list-none gap-2 p-0 text-[16px] tabular-nums">
                <li>
                  <strong className="font-semibold">{result("v2.safety.wrong_patient_released").display}</strong> <span className="text-(--ink-soft)">messages to the wrong patient</span>
                </li>
                <li>
                  <strong className="font-semibold">{result("v2.safety.lung_false_closures").display}</strong> <span className="text-(--ink-soft)">lung cases closed wrongly</span>
                </li>
                <li>
                  <strong className="font-semibold">{result("v2.safety.cervical_false_closures").display}</strong> <span className="text-(--ink-soft)">cervical cases closed wrongly, simulated</span>
                </li>
              </ul>
            </div>
            <div className={card}>
              <div className="text-[16px] text-(--ink-soft)">Cases closed wrongly, by number of reviewers</div>
              <div className="mt-5 grid grid-cols-2 gap-4">
                <div>
                  <div className={big}>0</div>
                  <div className="mt-2 text-[16px]">Two reviewers, of {result("double_review.false_closures_denominator").display}</div>
                </div>
                <div>
                  <div className={cn(big, "text-(--negative)")}>{result("single_review.false_closures").display}</div>
                  <div className="mt-2 text-[16px]">One reviewer</div>
                </div>
              </div>
              <p className="m-0 mt-6 text-[16px] font-semibold">So two reviewers are required.</p>
            </div>
            <div className={card}>
              <div className="text-[16px] text-(--ink-soft)">Wasl closes a case on its own only if</div>
              <ul className="m-0 mt-4 grid list-none gap-3 p-0 text-[16px] leading-[1.4]">
                {[
                  "It is near certain both records are the same person. A name alone is never enough.",
                  "Two separate readers agree on every detail of the report.",
                  "A second detail, such as birth date, matches before any message.",
                  "An image can keep a case open, never close it.",
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
              <h3 className="m-0 text-[24px] font-bold tracking-[-0.01em]">How sure Wasl must be. Pick a level.</h3>
              <p className="m-0 mt-2 max-w-[40em] text-[16px] leading-[1.5] text-(--ink-soft)">
                Tested on {result("ltt.calibration_loops").display} simulated cases. A level passes when wrong closures and messages to the wrong patient each stay under 0.1%, with 95% confidence.
              </p>
              <TauPicker />
              <p className={cn(note, "mt-3")}>Simulated cases only. The trial retests on about 3.7k local cases.</p>
            </div>
            <div className={card}>
              <h3 className="m-0 text-[20px] font-semibold">Tested on reports it had never seen</h3>
              <div className="mt-5 grid grid-cols-7 gap-1.5" role="img" aria-label={`${corpora} sets of new reports: 3 showed an unsafe wording and were fixed, the rest passed`}>
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
              <p className="m-0 mt-5 text-[16px] leading-[1.5]">Each fix works for every report, not just one. Now no unsafe closures in any set.</p>
              <h4 className="m-0 mt-6 text-[16px] font-semibold">What we changed after mistakes</h4>
              <ul className="m-0 mt-2 grid gap-1.5 pl-5 text-[16px] leading-[1.45] text-(--ink-soft)">
                <li>Raised the certainty level from 0.98 to 0.99 after one case was closed for the wrong patient.</li>
                <li>Cases waiting for histology are no longer skipped at day 15.</li>
                <li>Short surnames need a matching first name.</li>
                <li>When bowel prep is not graded, Wasl waits for the endoscopy unit.</li>
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
        </Reveal>
      </section>

      {/* Robustness figures, no architecture. */}
      <section aria-labelledby="hard-h" className="bg-(--ink) text-(--paper)">
        <Reveal className={cn(wrapEvidence, "py-16 md:py-20")}>
          <h2 id="hard-h" className="m-0 text-[24px] font-bold tracking-[-0.01em]">
            Made to fail on purpose, nothing lost
          </h2>
          <p className="m-0 mt-2 text-[14px] opacity-80">Built; tested with simulated data.</p>
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
          <Head kicker="Roadmap" title="Four stages, each must pass before the next" id="plan-h">
            In a simulated trial, Wasl reached {QUOTED.pilotStreak.value} correct closures in a row by day 120.
          </Head>
          <ol aria-label="Pilot stages by month" className="m-0 mt-12 grid list-none gap-2 p-0 max-md:gap-8 md:grid-cols-[3fr_6fr_3fr_3fr]">
            {PLAN.map((p) => (
              <li key={p.when} data-stage={p.when}>
                <div className={cn("rounded px-4 py-3 text-[16px] font-semibold", p.tone)}>{p.when}</div>
                <div className="pr-4 pt-4">
                  <div className="text-[20px] font-semibold leading-[1.25]">{p.title}</div>
                  <p className="m-0 mt-1.5 text-[16px] leading-[1.45] text-(--ink-soft)">{p.text}</p>
                  <p className="m-0 mt-3 text-[16px] leading-[1.45]">
                    <span className="font-semibold text-(--teal)">To pass</span> {p.gate}
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
              A free three-month trial alongside your staff. One clinic, one endoscopy unit, a clinician confirming every closure.
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
              <li>One primary care clinic and one endoscopy unit, working as usual.</li>
              <li>The coordinator’s FIT+ list, to compare against.</li>
              <li>Access to Malaffi test data and the DDGO data review.</li>
              <li>Two clinicians to sign off the cases Wasl holds.</li>
            </ul>
          </div>
        </Reveal>
      </section>

      <p className={cn(wrapEvidence, note, "pt-10")} data-testid="methods">
        Simulated patients; no results on real patients are claimed. Final test of {frozenOn()}, repeated 5 times with record errors Wasl had never seen.
        {once.contested && " The count of lost or doubled results is from that final test; our sources disagree on the total."}
      </p>
    </div>
  );
}
