import { HonestyLabel } from "@/components/wasl/honesty-label";
import { DEPENDENCY_GLOSS, DEPENDENCY_ORDER, type HealthState } from "./health";
import { TechnicalDetail } from "./technical";

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

/** "31 Oct 2026, 23:47 UTC" from an ISO instant; the raw string if it does not parse. */
function utc(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const hh = String(d.getUTCHours()).padStart(2, "0");
  const mm = String(d.getUTCMinutes()).padStart(2, "0");
  return `${d.getUTCDate()} ${MONTHS[d.getUTCMonth()]} ${d.getUTCFullYear()}, ${hh}:${mm} UTC`;
}

/**
 * Read-only `/healthz` details, kept apart from the replay (issue #11). The replay is a recording; this panel is the only place
 * on the screen that reports backend dependency values, and it says where those values came from and when.
 */
export function HealthPanel({ state }: { state: HealthState }) {
  return (
    <section aria-labelledby="health-h" data-testid="health-panel" className="mt-10 min-w-0 border-t border-line pt-5">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 id="health-h" className="m-0 text-body font-medium">
          Backend health
        </h2>
        {state.kind === "snapshot" && <HonestyLabel kind="published">Published snapshot, not live</HonestyLabel>}
        {state.kind === "live" && <HonestyLabel kind="built">Live API check</HonestyLabel>}
      </div>
      <p className="m-0 mt-1 text-small text-muted">
        Separate from the replay above. The replay animation is a recording of a past run and says nothing about whether any server is up now.
      </p>

      {state.kind === "unavailable" ? (
        <p data-testid="health-unavailable" className="m-0 mt-3 rounded-control bg-ground px-3 py-2 text-body">
          <span className="font-medium">Health unavailable.</span> The health check could not be loaded, so nothing here shows whether the backend is up.
          Treat any earlier value as stale.
          <span className="block text-small text-muted">Reason: {state.reason}</span>
        </p>
      ) : (
        <>
          <p data-testid="health-source" className="m-0 mt-3 text-body">
            {state.kind === "snapshot"
              ? `From the published snapshot taken ${utc(state.takenAt)}. It is not a live check, and it does not change while you watch.`
              : `Fetched from the live API ${utc(state.fetchedAt)}, when this page loaded. It does not refresh by itself, so reload to check again.`}
          </p>
          <dl className="m-0 mt-3 grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
            <Fact label="Status" value={state.health.status} testid="health-status" />
            <Fact label="Cases in the run" value={state.health.cases.toLocaleString("en-US")} testid="health-cases" />
          </dl>
          <h3 className="m-0 mt-4 text-body font-medium">Dependencies</h3>
          <dl className="m-0 mt-2 grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2">
            {DEPENDENCY_ORDER.map((k) => (
              <Fact
                key={k}
                label={DEPENDENCY_GLOSS[k].label}
                value={state.health.dependencies[k] ?? "not reported"}
                note={DEPENDENCY_GLOSS[k].note}
                testid={`dep-${k}`}
              />
            ))}
          </dl>
          <p className="m-0 mt-3 text-small text-muted">
            The message bus is not required to read a saved run, so a bus value of &ldquo;not required&rdquo; is expected here. The report reader is cached:
            its answers were produced earlier and are replayed from storage.
          </p>
          <TechnicalDetail label="Technical detail: the backend's own names">
            <div>status = {state.health.status}</div>
            <div>cases = {state.health.cases}</div>
            {Object.entries(state.health.dependencies).map(([k, v]) => (
              <div key={k}>
                {k} = {v}
              </div>
            ))}
          </TechnicalDetail>
        </>
      )}
    </section>
  );
}

function Fact({ label, value, note, testid }: { label: string; value: string; note?: string; testid: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-small text-muted">{label}</dt>
      <dd className="m-0 break-words text-body font-medium" data-testid={testid}>
        {value}
      </dd>
      {note && <dd className="m-0 text-small text-muted">{note}</dd>}
    </div>
  );
}
