import { Pill } from "@/components/review/marks";
import { TechnicalDetail } from "@/components/pipeline/technical";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { AGENT_LANE_SOURCE } from "@/lib/api/agent-source";
import type { Meta } from "@/lib/api/schema";
import { tauOf } from "./words";

/** The prominent "this is a separate experimental run" banner. */
export function AgentBanner({ meta, cases }: { meta: Meta; cases: number }) {
  const tau = tauOf(meta.policy);
  return (
    <section
      aria-labelledby="agent-banner-h"
      data-testid="agent-banner"
      className="rounded-card border-2 border-ink bg-raised p-4 md:p-pad"
    >
      <div className="flex flex-wrap items-center gap-2">
        <Pill tone="warning">Experimental</Pill>
        <HonestyLabel kind="simulated">Simulated data</HonestyLabel>
        <HonestyLabel kind="replay">Cached agent output</HonestyLabel>
      </div>
      <h2 id="agent-banner-h" className="m-0 mt-3 text-section font-medium">
        A separate experimental run, not the default review flow
      </h2>
      <p className="m-0 mt-2 max-w-[72ch]">
        This page shows one saved run of the evidence agent, <span className="font-medium">r1agent10</span>. It covers colorectal cancer (CRC) only, {cases} simulated cases. None of the other programmes exist
        in this run, and nothing here is mixed with the seven-programme sample on the other pages.
      </p>
      <dl className="m-0 mt-4 grid gap-x-6 gap-y-3 sm:grid-cols-2">
        <Fact term="Run">r1agent10, seed 987</Fact>
        <Fact term="Run id">
          <code className="font-mono">{meta.run}</code>
        </Fact>
        <Fact term="Programme">Colorectal (CRC) only</Fact>
        <Fact term="Data">Simulated. All patients, providers and outcomes are synthetic.</Fact>
        <Fact term="Policy">
          Strict link policy{tau ? `, auto-close line ${tau}` : ""}, a person must sign, evidence agent switched on, image reader cached.
        </Fact>
        <Fact term="Source">{AGENT_LANE_SOURCE}</Fact>
      </dl>
      <TechnicalDetail label="Show the exact policy string" className="mt-1">
        <span className="break-all">{meta.policy}</span>
      </TechnicalDetail>
    </section>
  );
}

function Fact({ term, children }: { term: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-small text-muted">{term}</dt>
      <dd className="m-0 break-words">{children}</dd>
    </div>
  );
}
