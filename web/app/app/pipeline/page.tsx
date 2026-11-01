import { notFound } from "next/navigation";
import readerJson from "@/data/reader.json";
import type { ReaderData } from "@/components/pipeline/reader-types";
import type { RulesBySha } from "@/components/pipeline/profile";
import { PipelineHeading } from "@/components/pipeline/heading";
import { PipelineReplay, type TamperDemo } from "@/components/pipeline/pipeline-replay";
import type { ChainNode } from "@/components/wasl/sealed-chain";
import { getAudit, getFixtures } from "@/lib/api/source";
import type { ChainEntry } from "@/lib/api/schema";
import { formatDay } from "@/lib/domain/day";
import { personForLoop } from "@/lib/people";

export const metadata = { title: "Pipeline" };

const short = (hash: string) => hash.slice(0, 12);

/** One audit entry in plain words; `body` is the backend's own record. */
function node(e: ChainEntry, i: number): ChainNode {
  const b = e.body as Record<string, unknown>;
  const step = `step ${i + 1} · ${e.type}`;
  switch (e.type) {
    case "link_decision":
      return {
        id: e.hash,
        kind: "system",
        who: "Linker",
        what: `matched report ${String(b.downstream_event).split("/").at(-1)} · p ${Number(b.path_p_cal).toFixed(3)}`,
        when: `${step} · ${short(e.hash)}`,
      };
    case "evidence":
      return {
        id: e.hash,
        kind: "system",
        who: "Two readers",
        what: `${b.agree ? "agree" : "disagree"} · prep ${b.label}`,
        when: `${step} · ${short(e.hash)}`,
      };
    case "closure_decision":
      return {
        id: e.hash,
        kind: "system",
        who: "Safety gate",
        what: `${b.state === "closed" ? "closed with evidence" : String(b.state)} · ${formatDay(Number(b.closed_day))}`,
        when: `${step} · ${short(e.hash)}`,
      };
    default:
      return { id: e.hash, kind: "system", who: "System", what: e.type, when: `${step} · ${short(e.hash)}` };
  }
}

/** The tamper demo rewrites the readers' step of the closed fixture: its hash stops matching, and so does the link to the closure after it. */
function tamperDemo(loop: string, chain: ChainEntry[]): TamperDemo {
  const intact = chain.map(node);
  const at = chain.findIndex((e) => e.type === "evidence");
  const evidence = chain[at];
  const label = String((evidence.body as Record<string, unknown>).label);
  const tampered = intact.map((n, i) =>
    i === at
      ? { ...n, what: n.what.replace(`prep ${label}`, "prep inadequate"), when: `step ${i + 1} · hash no longer matches ${short(evidence.hash)}`, broken: true }
      : i === at + 1
        ? { ...n, when: `step ${i + 1} · follows a changed record`, broken: true }
        : n,
  );
  const name = personForLoop(loop)?.display_name;
  return {
    title: name ? `${name}'s record` : "A closed case's record",
    intact,
    tampered,
    step: at + 1,
    changed: `The readers' step was set to "inadequate". Its hash no longer matches ${short(evidence.hash)}, so it and the closure after it are flagged.`,
  };
}

/** The rules reader's answers, for the replay reports whose text data/reader.json also holds. */
const rules: RulesBySha = Object.fromEntries((readerJson as ReaderData).cases.map((c) => [c.sha, c.rules]));

export default async function Page() {
  const loop = (await getFixtures()).fixtures.evidence_closure.id;
  const audit = await getAudit(loop);
  if (!audit) notFound();

  return (
    <PipelineReplay
      heading={<PipelineHeading title="Pipeline" current="/app/pipeline" />}
      tamper={tamperDemo(loop, audit.chain)}
      rules={rules}
    />
  );
}
