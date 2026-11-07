import { Pill } from "@/components/review/marks";
import { TechnicalDetail } from "@/components/pipeline/technical";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { AgentCaseDetail, AgentInvestigation } from "@/lib/api/agent-schema";
import type { ChainEntry } from "@/lib/api/schema";
import { plain } from "@/lib/domain/terms";
import { AgentSection } from "./section";
import { labelWords, readWords, signerWords } from "./words";

type Signoff = AgentCaseDetail["signoffs"][number];

/** The human link decision, with the agent's read beside the human r1 read. */
export function LinkDecisionCard({ inv, signoff }: { inv: AgentInvestigation; signoff?: Signoff }) {
  const reads = inv.human.reads;
  return (
    <AgentSection
      id="link-decision"
      title="The human link decision"
      note="Whether the two registrations are the same person. A person signed this decision. The reads that went into it were r1 and the agent."
      aside={<HonestyLabel kind="simulated">Simulated reviewer</HonestyLabel>}
    >
      <p className="m-0 mt-4 flex flex-wrap items-center gap-x-3 gap-y-1" data-testid="human-link-decision">
        <span className="text-muted">Signed link decision</span>
        <span className="text-section font-medium">{labelWords(inv.human.link)}</span>
        <Pill tone="positive" shape="tick">
          Signed by a person
        </Pill>
      </p>
      <div className="mt-4 grid gap-gap sm:grid-cols-2" data-testid="reads-pair" aria-label="The two reads side by side" role="group">
        <ReadBox testid="read-r1" who={readWords("r1")} said={labelWords(inv.human.link)} note="Counted in the signed decision" />
        <ReadBox testid="read-agent" who={readWords("agent")} said={labelWords(String(signoff?.agent_read || inv.label))} note="Experimental. Advice only; it cannot sign." />
      </div>
      <p className="m-0 mt-3 text-small text-muted" data-testid="reads-list">
        Reads recorded on this decision:{" "}
        <span className="text-ink">{reads.map((r) => readWords(r)).join(" + ")}</span> (<span className="font-mono">{reads.join(" + ")}</span>). The sample stores one signed decision for these reads. It does not store r1&apos;s
        answer separately, so r1&apos;s box shows the signed decision.
      </p>
      <p className="m-0 mt-2 text-small text-muted">Signed by role: {plain(inv.human.role)}.</p>
    </AgentSection>
  );
}

function ReadBox({ who, said, note, testid }: { who: string; said: string; note: string; testid: string }) {
  return (
    <div data-testid={testid} className="min-w-0 rounded-card border border-line bg-raised p-4">
      <p className="m-0 text-small text-muted">{who}</p>
      <p className="m-0 mt-1 text-section font-medium">{said}</p>
      <p className="m-0 mt-1 text-small text-muted">{note}</p>
    </div>
  );
}

function signoffKind(s: Signoff): string {
  return s.kinds.includes("link") ? "Identity link" : s.kinds.includes("evidence") ? "Bowel preparation evidence" : plain(s.kinds[0] ?? "review");
}

/** Who made the final sign-off, and every sign-off on the case. */
export function SignoffCard({ signoffs, finalEntry }: { signoffs: Signoff[]; finalEntry?: ChainEntry }) {
  const final = signoffs.find((s) => finalEntry && s.item === finalEntry.body.item) ?? signoffs[signoffs.length - 1];
  return (
    <AgentSection
      id="signoff"
      title="Who made the final sign-off"
      note="The case closed only after a person signed. The agent contributes one cited read and has no sign-off of its own."
      aside={<HonestyLabel kind="simulated">Simulated reviewer</HonestyLabel>}
    >
      {final && (
        <p className="m-0 mt-4" data-testid="final-signoff">
          <Pill tone="positive" shape="tick">
            Final sign-off by a person
          </Pill>
          <span className="mt-2 block max-w-[72ch]">
            {signerWords(final.reviewer, final.role)} signed the {signoffKind(final).toLowerCase()} as{" "}
            <span className="font-medium">{final.evidence ? plain(final.evidence).toLowerCase() : labelWords(final.link).toLowerCase()}</span>, from reads{" "}
            {final.reads.map((r) => readWords(r)).join(" and ")}.
          </span>
        </p>
      )}
      <ol className="m-0 mt-4 list-none p-0" aria-label="Sign-offs on this case" data-testid="signoff-list">
        {signoffs.map((s) => (
          <li key={s.item} className="border-t border-rule py-3" data-signoff={s.item}>
            <p className="m-0 font-medium">{signoffKind(s)}</p>
            <p className="m-0 mt-1 text-muted">
              {signerWords(s.reviewer, s.role)} · reads {s.reads.map((r) => readWords(r)).join(" + ")}
              {s.agent_read ? ` · agent read: ${labelWords(String(s.agent_read)).toLowerCase()}` : ""} · decision{" "}
              {s.evidence ? plain(s.evidence).toLowerCase() : labelWords(s.link).toLowerCase()}
            </p>
          </li>
        ))}
      </ol>
      <TechnicalDetail label="Show reviewer strings and item ids" className="mt-1">
        <ul className="m-0 list-none p-0">
          {signoffs.map((s) => (
            <li key={s.item} className="break-all">
              {s.item}: {s.reviewer}
            </li>
          ))}
        </ul>
      </TechnicalDetail>
    </AgentSection>
  );
}
