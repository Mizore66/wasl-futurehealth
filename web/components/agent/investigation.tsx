import { Pill } from "@/components/review/marks";
import { TechnicalDetail } from "@/components/pipeline/technical";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { AgentInvestigation, AgentProvenance } from "@/lib/api/agent-schema";
import { AgentSection } from "./section";
import { fieldWords, labelWords, recordParts, refWords, toolWords } from "./words";

/** What the agent concluded, the two records it compared, and what it cited. Citations show the tool and the record reference only. */
export function InvestigationCard({ inv, near }: { inv: AgentInvestigation; near?: { p: number; tau: number } }) {
  const [a, b] = inv.pair.map(recordParts);
  return (
    <AgentSection
      id="investigation"
      title="The agent's investigation"
      note="One cited read on whether two registrations are the same person. The agent looked up records with a tool and cites what it saw. It cannot sign a review or override a conflict."
      aside={<HonestyLabel kind="replay">Cached, produced offline</HonestyLabel>}
    >
      <p className="m-0 mt-4 flex flex-wrap items-center gap-x-3 gap-y-1" data-testid="agent-label">
        <span className="text-muted">Agent&apos;s cited label</span>
        <span className="text-section font-medium">{labelWords(inv.label)}</span>
        <Pill tone="warning">Experimental read</Pill>
      </p>
      <p className="m-0 mt-2 max-w-[72ch]">
        It compared the referral clinic&apos;s registration{" "}
        <span className="break-all text-small">
          {a.provider} {a.mrn}
        </span>{" "}
        with the registration at the hospital that did the exam{" "}
        <span className="break-all text-small">
          {b.provider} {b.mrn}
        </span>
        .{near && ` The match probability was ${near.p.toFixed(3)}, just under the ${near.tau} auto-close line, so a person had to decide.`}
      </p>

      <h3 className="m-0 mt-5 text-body font-medium">Citations ({inv.citations.length})</h3>
      <p className="m-0 mt-1 text-small text-muted">Each line is one fact the agent read from a registration record. Only the tool name and the record reference are shown, never the agent&apos;s prompt.</p>
      <ol className="m-0 mt-3 list-none p-0" aria-label="Agent citations" data-testid="agent-citations">
        {inv.citations.map((c, i) => (
          <li key={`${c.arg}-${c.field}-${i}`} className="grid gap-x-4 gap-y-1 border-t border-rule py-3 sm:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1.4fr)]" data-citation>
            <span className="min-w-0">
              <span className="block text-small text-muted">Tool</span>
              <span className="block">{toolWords(c.tool)}</span>
              <span className="block break-all font-mono text-small text-muted">{refWords(c.arg)}</span>
            </span>
            <span className="min-w-0">
              <span className="block text-small text-muted">Field</span>
              <span className="block">{fieldWords(c.field)}</span>
            </span>
            <span className="min-w-0">
              <span className="block text-small text-muted">Value it read</span>
              <span className="block break-words font-medium" dir="auto">
                {c.value}
              </span>
            </span>
          </li>
        ))}
      </ol>
      <TechnicalDetail label="Show tool names and full record references" className="mt-1">
        <ul className="m-0 list-none p-0">
          {inv.citations.map((c, i) => (
            <li key={i} className="break-all">
              {c.tool}({c.arg}).{c.field}
            </li>
          ))}
        </ul>
        <p className="m-0 mt-2">Review item {inv.item}</p>
      </TechnicalDetail>
    </AgentSection>
  );
}

/** Who produced the agent's read and under which versions. */
export function ProvenanceCard({ prov }: { prov: AgentProvenance }) {
  return (
    <AgentSection
      id="provenance"
      title="Where the agent's read came from"
      note="The agent's output was produced offline and saved. Opening this page did not call a model."
      aside={<HonestyLabel kind="simulated">Simulated run</HonestyLabel>}
    >
      <dl className="m-0 mt-4 grid gap-x-6 gap-y-3 sm:grid-cols-2" data-testid="agent-provenance">
        <Row term="Model">
          <code className="font-mono">{prov.model}</code>
        </Row>
        <Row term="Prompt version">
          <code className="font-mono">{prov.prompt_ver}</code>
        </Row>
        <Row term="Schema version">
          <code className="font-mono">{prov.schema_ver}</code>
        </Row>
        <Row term="Cached">
          <Pill tone={prov.cached ? "positive" : "muted"} shape={prov.cached ? "tick" : "dash"}>
            {prov.cached ? "Cached: replayed from saved output" : "Not cached"}
          </Pill>
        </Row>
        <Row term="Experimental">
          <Pill tone={prov.experimental ? "warning" : "muted"} shape={prov.experimental ? "tick" : "dash"}>
            {prov.experimental ? "Experimental: not part of the default flow" : "Not experimental"}
          </Pill>
        </Row>
      </dl>
    </AgentSection>
  );
}

function Row({ term, children }: { term: string; children: React.ReactNode }) {
  return (
    <div className="min-w-0">
      <dt className="text-small text-muted">{term}</dt>
      <dd className="m-0 mt-1 break-words">{children}</dd>
    </div>
  );
}
