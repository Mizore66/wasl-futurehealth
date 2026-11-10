import Link from "next/link";
import { notFound } from "next/navigation";
import { AgentBanner } from "@/components/agent/banner";
import { AgentChain } from "@/components/agent/chain";
import { LinkDecisionCard, SignoffCard } from "@/components/agent/decision";
import { InvestigationCard, ProvenanceCard } from "@/components/agent/investigation";
import { AgentSection } from "@/components/agent/section";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { getAgentDisclosure, getAgentLane, getAgentReviewCase } from "@/lib/api/agent-source";
import { shortId } from "@/lib/domain/ids";

export const metadata = { title: "Agent review (experimental)" };

export default async function Page() {
  const [lane, disclosure, { detail, audit }] = await Promise.all([getAgentLane(), getAgentDisclosure(), getAgentReviewCase()]);
  const inv = detail.agent_investigations?.[0];
  if (!inv || !detail.agent_provenance) notFound();

  const item = detail.review_items.find((r) => r.item === inv.item);
  const tau = Number(/tau=([0-9.]+)/.exec(detail.meta.policy)?.[1]);
  const near = item && typeof item.p === "number" && tau ? { p: item.p, tau } : undefined;
  const linkSignoff = detail.signoffs.find((s) => s.item === inv.item);
  const finalEntry = [...audit.chain].reverse().find((e) => e.type === "human_signoff");

  return (
    <>
      <p className="m-0">
        <Link href="/app/review" className="inline-flex min-h-11 items-center">
          ← Back to the default review flow
        </Link>
      </p>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="m-0 text-title font-semibold">Agent review</h1>
          <p className="m-0 mt-1 max-w-[72ch] text-muted">
            One case where an experimental evidence agent gave a cited read beside a human reviewer. Case <span className="text-small">{shortId(detail.case.id)}</span>.
          </p>
        </div>
        <HonestyLabel kind="simulated">Simulated, experimental</HonestyLabel>
      </header>

      <div className="mt-6 flex flex-col gap-gap">
        <AgentBanner meta={detail.meta} cases={lane.cases} />

        <section aria-labelledby="disclosure-h" className="rounded-card border border-line bg-surface p-4 md:p-pad" data-testid="agent-disclosure">
          <h2 id="disclosure-h" className="m-0 text-section font-medium">
            What this run does and does not show
          </h2>
          <p className="m-0 mt-3 max-w-[72ch] text-section font-medium" data-testid="disclosure-text">
            {disclosure.text}
          </p>
          <p className="m-0 mt-2 text-small text-muted">
            Copied word for word from the recording guide ({disclosure.source}, line {disclosure.line}).
          </p>
        </section>

        <InvestigationCard inv={inv} near={near} />
        <ProvenanceCard prov={detail.agent_provenance} />
        <LinkDecisionCard inv={inv} signoff={linkSignoff} />
        <SignoffCard signoffs={detail.signoffs} finalEntry={finalEntry} />
        <AgentChain audit={audit} />

        <AgentSection
          id="backup"
          title="No-agent backup path"
          note="The default review flow does not use the agent and is unchanged. Use it for a clean backup recording."
          aside={<HonestyLabel kind="simulated">Default flow</HonestyLabel>}
        >
          <ul className="m-0 mt-3 flex list-none flex-wrap gap-x-6 gap-y-1 p-0">
            <li>
              <Link href="/app/review" className="inline-flex min-h-11 items-center">
                Default review queue
              </Link>
            </li>
            <li>
              <Link href="/app/patients" className="inline-flex min-h-11 items-center">
                All patients
              </Link>
            </li>
          </ul>
        </AgentSection>
      </div>
    </>
  );
}
