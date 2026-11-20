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
    label: "No consent",
    what: "Released without contact permission granted earlier in the record.",
  },
  {
    key: "released_after_opt_out_on_file",
    label: "After opt-out",
    what: "Released after the patient's opt-out was already recorded. This is the violation.",
  },
  {
    key: "released_before_opt_out_recorded",
    label: "Before opt-out",
    what: "Released before an opt-out the patient recorded later.",
  },
] as const;

/**
 * The consent hold, release and cancellation counts for the active run, read from `dashboard.metrics.consent_timing`.
 * Nothing is hardcoded: every figure is a field of the dashboard payload. Counts are message intents, not patients.
 * Labels stay one or two words; what each count means is in the tooltip and the table view.
 */
export function ConsentLifecycle({ timing }: { timing: Record<string, number> }) {
  const released = timing.released_total ?? 0;
  return (
    <section aria-labelledby="consent-h" data-testid="consent-lifecycle" className="bg-surface px-pad py-5 lg:col-span-2">
      <h2 id="consent-h" className="m-0 text-section font-medium">
        Consent: held, released, cancelled
      </h2>
      <p className="m-0 mt-1 text-small text-muted">Simulated message intents, not patients. No message was sent.</p>

      <div className="mt-4 grid grid-cols-1 gap-x-10 gap-y-5 lg:grid-cols-2">
        <div className="min-w-0">
          <CountBars
            caption="Message intents by consent outcome. The counts overlap: held then released is in both."
            tableHead={["Outcome", "Message intents"]}
            rows={LIFECYCLE.map((s) => ({ label: s.label, what: s.what, count: timing[s.key] ?? 0, testId: `consent-${s.key}` }))}
          />
        </div>
        <div className="min-w-0">
          <h3 className="m-0 text-body font-medium">Safety counters, read against the counts</h3>
          <div className="mt-2">
            <CountBars
              caption="Released message intents that broke a consent rule, out of all released"
              tableHead={["Counter", "Released intents"]}
              checked
              rows={GUARDS.map((g) => ({ label: g.label, what: g.what, count: timing[g.key] ?? 0, of: released, testId: `consent-${g.key}` }))}
            />
          </div>
          <p data-testid="consent-opted_out_loops" className="m-0 mt-2 text-small text-muted">
            {fmt(timing.opted_out_loops)} loops had an opt-out on file (this counts loops, not message intents).
          </p>
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
