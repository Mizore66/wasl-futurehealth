import { HonestyLabel } from "@/components/wasl/honesty-label";
import { TechnicalDetail } from "@/components/pipeline/technical";

const fmt = (n: number | undefined) => (n === undefined ? "not reported" : n.toLocaleString("en-US"));

/** Issue #9. Each entry: the dashboard field, the plain label, and what the backend counts (pipeline/report7.go, `consent_timing`). */
export const LIFECYCLE = [
  {
    key: "intents_ever_held_for_consent",
    label: "Held",
    what: "Message intents queued because no contact permission was on file yet. Counted if held at any point, even if released later.",
  },
  {
    key: "released_total",
    label: "Released",
    what: "Message intents cleared to go out. Includes intents that were never held.",
  },
  {
    key: "intents_cancelled_by_consent",
    label: "Cancelled",
    what: "Message intents dropped because consent was withheld or withdrawn.",
  },
] as const;

export const GUARDS = [
  {
    key: "released_without_granted_consent",
    label: "Released without granted consent",
    what: "Released intents with no granted contact permission earlier in the record.",
  },
  {
    key: "released_after_opt_out_on_file",
    label: "Released after an opt-out was on file",
    what: "Released after the patient's opt-out was already recorded. This is the violation.",
  },
  {
    key: "released_before_opt_out_recorded",
    label: "Released before the opt-out was recorded",
    what: "Released for a patient who opted out later, when no opt-out was yet on file.",
  },
] as const;

/**
 * The consent hold, release and cancellation counts for the active run, read from `dashboard.metrics.consent_timing`.
 * Nothing is hardcoded: every figure is a field of the dashboard payload. Counts are message intents, not patients.
 */
export function ConsentLifecycle({ timing }: { timing: Record<string, number> }) {
  return (
    <section aria-labelledby="consent-h" data-testid="consent-lifecycle" className="rounded-card border border-line bg-surface px-pad py-5 lg:col-span-2">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 id="consent-h" className="m-0 text-section font-medium">
          Consent: held, released, cancelled
        </h2>
        <HonestyLabel kind="simulated">Simulated messages</HonestyLabel>
      </div>
      <p className="m-0 text-small text-muted">
        What the safety gate did with planned reminder and recall messages in this simulated run. No message was sent to anyone. Each count is a number
        of message intents, not of patients: one patient can have several.
      </p>

      <ol aria-label="Message intents by consent outcome" className="m-0 mt-4 grid list-none grid-cols-1 gap-3 p-0 md:grid-cols-3">
        {LIFECYCLE.map((s, i) => (
          <li key={s.key} data-testid={`consent-${s.key}`} className="relative min-w-0 rounded-control bg-ground px-4 py-3">
            <div className="flex items-baseline justify-between gap-3">
              <span className="text-body font-medium">{s.label}</span>
              <span className="text-title font-medium tabular-nums">{fmt(timing[s.key])}</span>
            </div>
            <span className="block text-small text-muted">{s.what}</span>
            {i < LIFECYCLE.length - 1 && (
              <svg aria-hidden width="16" height="16" viewBox="0 0 16 16" className="absolute -bottom-[14px] left-1/2 z-10 -translate-x-1/2 rotate-90 text-control md:-right-[14px] md:bottom-auto md:left-auto md:top-1/2 md:-translate-y-1/2 md:translate-x-0 md:rotate-0">
                <path d="M5 2l6 6-6 6" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            )}
          </li>
        ))}
      </ol>
      <p className="m-0 mt-2 text-small text-muted">
        The boxes overlap and do not add up: an intent held and then released is in both, and many were released without ever being held.
      </p>

      <div className="mt-5 grid grid-cols-1 gap-x-10 gap-y-5 border-t border-rule pt-4 lg:grid-cols-2">
        <div className="min-w-0">
          <h3 className="m-0 text-body font-medium">Safety counters, beside the activity</h3>
          <p className="m-0 text-small text-muted">
            A zero here is a result, not a lack of activity: it sits against the released and cancelled counts above.
          </p>
          <dl className="m-0 mt-3 flex flex-col gap-3">
            {GUARDS.map((g) => (
              <div key={g.key} data-testid={`consent-${g.key}`} className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-4">
                <dt className="min-w-0 text-body">
                  {g.label}
                  <span className="block text-small text-muted">{g.what}</span>
                </dt>
                <dd className="m-0 text-title font-medium tabular-nums">{fmt(timing[g.key])}</dd>
              </div>
            ))}
          </dl>
        </div>
        <div className="min-w-0">
          <h3 className="m-0 text-body font-medium">Opt-outs</h3>
          <div data-testid="consent-opted_out_loops" className="mt-1 flex items-baseline gap-3">
            <span className="text-title font-medium tabular-nums">{fmt(timing.opted_out_loops)}</span>
            <span className="text-body">loops with an opt-out</span>
          </div>
          <p className="m-0 text-small text-muted">
            This counts loops (a referral followed to its end), not message intents. It is not comparable with the cancelled count.
          </p>
          <p className="m-0 mt-3 text-small text-muted">
            Wrong-patient messages are counted separately, in the False closures card above.
          </p>
        </div>
      </div>

      <table className="mt-5 w-full border-collapse text-left text-small">
        <caption className="pb-1 text-left text-small text-muted">The same figures as a table</caption>
        <thead>
          <tr className="border-b border-rule">
            <th scope="col" className="py-1.5 pr-3 font-medium">
              Count
            </th>
            <th scope="col" className="py-1.5 pr-3 text-right font-medium">
              Value
            </th>
            <th scope="col" className="py-1.5 font-medium">
              Counts
            </th>
          </tr>
        </thead>
        <tbody>
          {[...LIFECYCLE.map((s) => ({ ...s, unit: "message intents" })), ...GUARDS.map((g) => ({ ...g, unit: "message intents" })), { key: "opted_out_loops", label: "Loops with an opt-out", unit: "loops" }].map((r) => (
            <tr key={r.key} className="border-b border-rule">
              <th scope="row" className="py-1.5 pr-3 font-normal">
                {r.label}
              </th>
              <td className="py-1.5 pr-3 text-right tabular-nums">{fmt(timing[r.key])}</td>
              <td className="py-1.5 text-muted">{r.unit}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <TechnicalDetail label="Technical detail: the dashboard's field names">
        {Object.entries(timing).map(([k, v]) => (
          <div key={k}>
            {k} = {v}
          </div>
        ))}
      </TechnicalDetail>
    </section>
  );
}
