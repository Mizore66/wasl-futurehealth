import type { ReactNode } from "react";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { formatUtcInstant as utc } from "@/lib/domain/day";
import type { Operations } from "@/lib/api/schema";
import { completedRunRate, DEPENDENCY_GLOSS, DEPENDENCY_ORDER, type OperationsState } from "./health";

/** Point-in-time synthetic observations and separately labelled completed-run measurements (issue #81). */
export function HealthPanel({ state }: { state: OperationsState }) {
  return (
    <section aria-labelledby="health-h" data-testid="health-panel" className="mt-10 min-w-0 border-t border-line pt-5">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 id="health-h" className="m-0 text-body font-medium">Synthetic operational readiness</h2>
        {state.kind === "snapshot" && <HonestyLabel kind="published">Published snapshot, not live</HonestyLabel>}
        {state.kind === "live" && <HonestyLabel kind="built">Live synthetic API check</HonestyLabel>}
      </div>
      <p className="m-0 mt-1 text-small text-muted">
        Separate from the recorded replay above. Checks do not auto-refresh. This is not production monitoring, a hospital live feed,
        proof of NATS cluster quorum, a verified restore, or an accumulated service-level objective.
      </p>

      {state.kind === "unavailable" ? (
        <p data-testid="health-unavailable" className="m-0 mt-3 rounded-control bg-ground px-3 py-2 text-body">
          <span className="font-medium">Operational data unavailable.</span> The operational check could not be loaded or validated.
          No earlier API, queue, run, or backup values are shown.
          <span className="block text-small text-muted">Reason: {state.reason}</span>
        </p>
      ) : (
        <OperationsDetails state={state} />
      )}
    </section>
  );
}

function OperationsDetails({ state }: { state: Extract<OperationsState, { kind: "snapshot" | "live" }> }) {
  const { operations: o } = state;
  return (
    <>
      <p data-testid="health-source" className="m-0 mt-3 text-body">
        {state.kind === "snapshot"
          ? `Published snapshot, not live. Observed ${utc(o.observed_at)}; the completed-run data was loaded ${utc(o.meta.loaded_at)}.`
          : `Live synthetic API check fetched ${utc(state.fetchedAt)}, when this page loaded. Reload to check again.`}
      </p>

      <h3 className="m-0 mt-5 text-body font-medium">API health</h3>
      <dl className="m-0 mt-2 grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
        <Fact label="Status" value={o.api.status} testid="health-status" />
        <Fact label="Source" value={o.api.source} testid="api-source" />
        <Fact label="Observed at" value={<Instant value={o.observed_at} />} testid="observed-at" />
        <Fact label="Completed-run data loaded" value={<Instant value={o.meta.loaded_at} />} testid="loaded-at" />
      </dl>
      <h4 className="m-0 mt-4 text-body font-medium">Dependencies reported by /healthz</h4>
      <dl className="m-0 mt-2 grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2">
        {DEPENDENCY_ORDER.map((key) => (
          <Fact key={key} label={DEPENDENCY_GLOSS[key].label} value={o.api.dependencies[key]} note={DEPENDENCY_GLOSS[key].note} testid={`dep-${key}`} />
        ))}
      </dl>

      <h3 className="m-0 mt-5 text-body font-medium">NATS and durable-consumer lag</h3>
      <dl className="m-0 mt-2 grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
        <Fact label="Authenticated JetStream probe" value={o.nats.status} note={o.nats.status === "unreachable" ? o.nats.reason : undefined} testid="nats-status" />
        <Fact label="NATS observation source" value={o.nats.source} testid="nats-source" />
        <Fact label="Known durable-consumer coverage" value={o.queue.status} note={o.queue.status === "available" ? undefined : o.queue.reason} testid="queue-status" />
        {o.queue.consumers.length === 0 ? (
          <Fact label="Durable-consumer pending and unacknowledged counts" value="Unmeasured" note="The current queue observation failed; zero backlog is not inferred." testid="queue-unmeasured" />
        ) : (
          o.queue.consumers.map((consumer) => (
            <Fact
              key={`${consumer.stream}/${consumer.consumer}`}
              label={`${consumer.stream}/${consumer.consumer}`}
              value={consumer.status === "available" ? `${consumer.num_pending} pending, ${consumer.num_ack_pending} unacknowledged` : "Unavailable"}
              note={consumer.status === "unavailable" ? consumer.reason : "Current JetStream consumer information."}
              testid={`queue-${consumer.stream}-${consumer.consumer}`}
            />
          ))
        )}
      </dl>
      <p className="m-0 mt-2 text-small text-muted">Source: {o.queue.source}. Ordered ephemeral consumers are not continuously measured.</p>

      <h3 className="m-0 mt-5 text-body font-medium">Current API measurements</h3>
      <dl className="m-0 mt-2 grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2">
        <Fact label="Current API latency" value="Unmeasured" note={o.api_measurements.latency.reason} testid="api-latency" />
        <Fact label="Current API error rate" value="Unmeasured" note={o.api_measurements.error_rate.reason} testid="api-error-rate" />
      </dl>

      <h3 className="m-0 mt-5 text-body font-medium">Completed synthetic run</h3>
      <p className="m-0 mt-1 text-small text-muted">
        Accumulated observations from {o.run.source}, scoped to {o.run.scope}. These are not current API latency, current API error rates, or an SLO.
      </p>
      {o.run.status === "unmeasured" ? (
        <dl className="m-0 mt-2"><Fact label="Latency and recovery" value="Unmeasured" note={o.run.reason} testid="run-unmeasured" /></dl>
      ) : (
        <CompletedRun run={o.run} />
      )}

      <h3 className="m-0 mt-5 text-body font-medium">Latest completed bundle</h3>
      <dl className="m-0 mt-2 grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
        <Fact label="Backup metadata" value={o.backup.status} note={o.backup.status === "available" ? undefined : o.backup.reason} testid="backup-status" />
        <Fact label="Backup metadata source" value={o.backup.source} testid="backup-source" />
        {o.backup.status === "available" && (
          <>
            <Fact label="Bundle schema" value={o.backup.bundle_schema} testid="backup-schema" />
            <Fact label="Latest completed bundle created" value={<Instant value={o.backup.created_at} />} testid="backup-created-at" />
            <Fact label="Latest completed bundle age at observation" value={bundleAge(o.backup.age_seconds)} testid="backup-age" />
            <Fact label="Verification" value="Not verified by API" testid="backup-verification" />
          </>
        )}
        {o.backup.status === "unavailable" && o.backup.created_at && (
          <Fact label="Manifest creation time" value={<Instant value={o.backup.created_at} />} testid="backup-created-at" />
        )}
        {o.backup.status !== "available" && <Fact label="Latest completed bundle age" value={o.backup.status === "unmeasured" ? "Unmeasured" : "Unavailable"} testid="backup-age" />}
      </dl>
      <p className="m-0 mt-2 text-small text-muted">
        The API reads only the configured manifest&apos;s schema and creation time. It does not authenticate or mount the bundle, read keys,
        decrypt payloads, or verify a restore. Bundle age is not proof of a successful backup or restore.
      </p>
    </>
  );
}

function CompletedRun({ run }: { run: Extract<Operations["run"], { status: "available" }> }) {
  const latency = run.latency_ms;
  const recovery = run.recovery;
  return (
    <>
      <dl className="m-0 mt-2 grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
        <Fact label="Latency observations" value={latency.n.toLocaleString("en-US")} testid="run-latency-n" />
        <Fact label="p50 latency" value={milliseconds(latency.p50)} testid="run-latency-p50" />
        <Fact label="p90 latency" value={milliseconds(latency.p90)} testid="run-latency-p90" />
        <Fact label="p99 latency" value={milliseconds(latency.p99)} testid="run-latency-p99" />
        <Fact label="Maximum latency" value={milliseconds(latency.max)} testid="run-latency-max" />
        <Fact label="Run metrics source" value={run.source} testid="run-source" />
      </dl>
      <h4 className="m-0 mt-4 text-body font-medium">Recorded integrity counts and rates</h4>
      <dl className="m-0 mt-2 grid grid-cols-1 gap-x-8 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
        {([
          ["generated_resources", "Generated resources"], ["fhir_msgs", "FHIR messages"],
          ["expected_outputs", "Expected outputs"], ["live_outputs", "Live outputs"],
          ["ingest_lost", "Ingest lost"], ["ingest_duplicates", "Ingest duplicates"],
          ["lost_outputs", "Lost outputs"], ["duplicate_outputs", "Duplicate outputs"],
        ] as const).map(([key, label]) => <Fact key={key} label={label} value={recovery[key].toLocaleString("en-US")} testid={`run-${key}`} />)}
        <Fact label="Ingest lost / generated resources" value={completedRunRate(recovery.ingest_lost, recovery.generated_resources)} testid="rate-ingest-lost" />
        <Fact label="Ingest duplicates / FHIR messages" value={completedRunRate(recovery.ingest_duplicates, recovery.fhir_msgs)} testid="rate-ingest-duplicates" />
        <Fact label="Lost outputs / expected outputs" value={completedRunRate(recovery.lost_outputs, recovery.expected_outputs)} testid="rate-lost-outputs" />
        <Fact label="Duplicate outputs / live outputs" value={completedRunRate(recovery.duplicate_outputs, recovery.live_outputs)} testid="rate-duplicate-outputs" />
      </dl>
    </>
  );
}

function milliseconds(value: number): string {
  return `${value.toLocaleString("en-US", { maximumFractionDigits: 3 })} ms`;
}

function bundleAge(seconds: number): string {
  if (seconds < 60) return `${seconds} seconds`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)} minutes ${seconds % 60} seconds`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} hours ${Math.floor((seconds % 3600) / 60)} minutes`;
  return `${Math.floor(seconds / 86400)} days ${Math.floor((seconds % 86400) / 3600)} hours`;
}

function Instant({ value }: { value: string }) {
  return <time dateTime={value}>{utc(value)}</time>;
}

function Fact({ label, value, note, testid }: { label: string; value: ReactNode; note?: string; testid: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-small text-muted">{label}</dt>
      <dd className="m-0 break-words text-body font-medium" data-testid={testid}>{value}</dd>
      {note && <dd className="m-0 text-small text-muted">{note}</dd>}
    </div>
  );
}
