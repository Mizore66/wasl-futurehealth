import { Pill } from "@/components/review/marks";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { bodySummary, entryLabel, shortHash } from "@/components/case/words";
import type { AgentAudit } from "@/lib/api/agent-schema";
import type { ChainEntry } from "@/lib/api/schema";
import { AgentSection } from "./section";
import { signerWords } from "./words";

function entryText(e: ChainEntry): string {
  if (e.type !== "human_signoff") return bodySummary(e);
  const b = e.body as { evidence?: string; reviewer?: string };
  return `${signerWords(b.reviewer ?? "", "clinician")} signed the evidence as ${b.evidence || "reviewed"}.`;
}

/** The case's sealed chain, read from the agent lane's own audit sample. */
export function AgentChain({ audit }: { audit: AgentAudit }) {
  const intact = audit.chain.length === audit.chain_length;
  return (
    <AgentSection
      id="audit"
      title="The case's audit chain"
      note="Each sealed entry carries the hash of the one before it. This proves the record is unaltered, not that the decisions were clinically right."
      aside={<HonestyLabel kind="simulated">Simulated run, real hashes</HonestyLabel>}
    >
      <p className="m-0 mt-3 flex flex-wrap items-center gap-3" role="status" data-testid="agent-chain-verified">
        {audit.chain_verified && intact ? (
          <Pill tone="positive" shape="tick">
            Verified · {audit.chain.length} of {audit.chain_length} intact
          </Pill>
        ) : (
          <Pill tone="negative" shape="cross">
            Chain not verified
          </Pill>
        )}
        <span className="text-small text-muted">The check ran when this snapshot was published. This page shows that result and does not re-run it.</span>
      </p>
      <ol className="m-0 mt-3 list-none p-0" aria-label="Chain entries">
        {audit.chain.map((e, i) => (
          <li key={e.hash} className="border-t border-rule py-3" data-entry={e.type}>
            <div className="flex items-baseline gap-3">
              <span className="w-5 flex-none text-right text-muted">{i + 1}</span>
              <div className="min-w-0 flex-1">
                <p className="m-0 font-medium">{entryLabel(e.type)}</p>
                <p className="m-0 mt-1">{entryText(e)}</p>
                <p className="m-0 mt-1 break-all text-small text-muted">
                  Hash <span className="font-mono">{shortHash(e.hash)}</span> · after{" "}
                  <span className="font-mono">{e.prev === "genesis" ? "start of chain" : shortHash(e.prev)}</span>
                </p>
              </div>
            </div>
          </li>
        ))}
      </ol>
    </AgentSection>
  );
}
