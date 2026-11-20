import { HonestyLabel } from "@/components/wasl/honesty-label";
import { TechnicalDetail } from "@/components/pipeline/technical";

const fmt = (n: number | undefined) => (n === undefined ? "not reported" : n.toLocaleString("en-US"));

/** Issue #9. Each entry: the dashboard field, the plain label, and what the backend counts (pipeline/report7.go, `consent_timing`). */
export const LIFECYCLE = [
  {
    key: "intents_ever_held_for_consent",
    label: "Held",
    what: "Queued because no contact permission was on file yet, including ones released later.",
  },
  {
    key: "released_total",
    label: "Released",
    what: "Cleared to send, including ones never held.",
  },
  {
    key: "intents_cancelled_by_consent",
    label: "Cancelled",
    what: "Dropped because consent was withheld or withdrawn.",
  },
] as const;

export const GUARDS = [
  {
    key: "released_without_granted_consent",
    label: "Released without granted consent",
    what: "No contact permission granted earlier in the record.",
  },
  {
    key: "released_after_opt_out_on_file",
    label: "Released after an opt-out was on file",
    what: "The patient's opt-out was already recorded. This is the violation.",
  },
  {
    key: "released_before_opt_out_recorded",
    label: "Released before the opt-out was recorded",
    what: "The patient opted out later.",
  },
] as const;

/**
 * The consent hold, release and cancellation counts for the active run, read from `dashboard.metrics.consent_timing`.
 * Nothing is hardcoded: every figure is a field of the dashboard payload. Counts are message intents, not patients.
 */
export function ConsentLifecycle({ timing }: { timing: Record<string, number> }) {
  return (
    <section aria-labelledby="consent-h" data-testid="consent-lifecycle" className="bg-surface px-pad py-5 lg:col-span-2">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 id="consent-h" className="m-0 text-section font-medium">
          Consent: held, released, cancelled
        </h2>
        <HonestyLabel kind="simulated">Simulated messages</HonestyLabel>
      </div>
      <p className="m-0 text-small text-muted">
        Planned reminder and recall messages in this simulated run. No message was sent to anyone. Counts are message intents, not patients.
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
        The counts overlap. An intent held and then released is in both.
      </p>

      <div className="mt-5 grid grid-cols-1 gap-x-10 gap-y-5 border-t border-rule pt-4 lg:grid-cols-2">
        <div className="min-w-0">
          <h3 className="m-0 text-body font-medium">Safety counters</h3>
          <p className="m-0 text-small text-muted">Read each zero against the released and cancelled counts above.</p>
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
          <p className="m-0 text-small text-muted">This counts loops (one referral followed to its end), not message intents.</p>
        </div>
      </div>

      <TechnicalDetail label="Technical detail: dashboard field names">
        {Object.entries(timing).map(([k, v]) => (
          <div key={k}>
            {k} = {v}
          </div>
        ))}
      </TechnicalDetail>
    </section>
  );
}
