import { HonestyLabel } from "@/components/wasl/honesty-label";
import { TechnicalDetail } from "@/components/pipeline/technical";
import { CountBars } from "@/components/results/charts";

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
      <p className="m-0 mt-1 flex flex-wrap gap-2 text-small">
        <span className="rounded-chip bg-raised px-2 py-0.5 text-ink-soft">No message was sent to anyone</span>
        <span className="rounded-chip bg-raised px-2 py-0.5 text-ink-soft">Counts are message intents, not patients</span>
      </p>

      <div className="mt-4 grid grid-cols-1 gap-x-10 gap-y-5 lg:grid-cols-2">
        <div className="min-w-0">
          <CountBars
            caption="Message intents by consent outcome (the counts overlap: held then released is in both)"
            tableHead={["Outcome", "Message intents"]}
            rows={LIFECYCLE.map((s) => ({ label: s.label, sub: s.what, count: timing[s.key] ?? 0, testId: `consent-${s.key}` }))}
          />
        </div>
        <div className="min-w-0">
          <h3 className="m-0 text-body font-medium">Safety counters, read against the counts</h3>
          <ul className="m-0 mt-2 flex list-none flex-col gap-2 p-0">
            {GUARDS.map((g) => (
              <li key={g.key} data-testid={`consent-${g.key}`} title={g.what} className="flex items-baseline justify-between gap-3 rounded-control bg-raised px-3 py-1.5 text-small">
                <span className="min-w-0 text-ink-soft">{g.label}</span>
                <span className="text-section font-semibold tabular-nums text-ink">{fmt(timing[g.key])}</span>
              </li>
            ))}
            <li data-testid="consent-opted_out_loops" className="flex items-baseline justify-between gap-3 rounded-control bg-raised px-3 py-1.5 text-small">
              <span className="min-w-0 text-ink-soft">loops with an opt-out (this counts loops, not message intents)</span>
              <span className="text-section font-semibold tabular-nums text-ink">{fmt(timing.opted_out_loops)}</span>
            </li>
          </ul>
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
