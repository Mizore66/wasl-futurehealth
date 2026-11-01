"use client";

import { useEffect, useState } from "react";
import { LayoutGroup, motion, useReducedMotion } from "motion/react";
import { LabAvatar as Avatar } from "./lab-avatar";
import type { SystemId } from "./systems";
import { CAST } from "./cast";

const FLOW = [
  { ...CAST[0], spelling: "F. Alzabi", outcome: 4 },
  { ...CAST[1], spelling: "K. Hamadi", outcome: 5 },
  { ...CAST[3], spelling: "M. Santos", outcome: 4 },
  { ...CAST[5], spelling: "N. Mansouri", outcome: 6 },
  { ...CAST[6], spelling: "R. Nair", outcome: 4 },
  { ...CAST[9], spelling: "H. Aldhaheri", outcome: 5 },
  { ...CAST[12], spelling: "L. Hadad", outcome: 4 },
  { ...CAST[13], spelling: "A. Alkaabi", outcome: 4 },
];

const STAGES = [
  { name: "Received", sub: "HL7 v2 · FHIR R4" },
  { name: "Matching identity", sub: "a second detail, or a person" },
  { name: "Reading the report", sub: "two readers" },
  { name: "Safety gate", sub: "both agree" },
  { name: "Closed with evidence", sub: "closure_decision", tone: "positive" },
  { name: "Waiting for two reviewers", sub: "needs_human", tone: "held" },
  { name: "Kept open", sub: "not_closed", tone: "negative" },
] as const;

const SPACING = 2;
const DWELL = 2;
const PERIOD = FLOW.length * SPACING + 4 * DWELL + 6;

function stageOf(i: number, t: number, outcome: number): number | null {
  const age = t - i * SPACING;
  if (age < 0) return null;
  const s = Math.floor(age / DWELL);
  return s >= 4 ? outcome : s;
}

/** A mock of the case flow: records arrive as "?", resolve into a face at the link, then move through each analysis. */
export function PipelineMock({ initial }: { initial: SystemId }) {
  const [system, setSystem] = useState<SystemId>(initial);
  const [t, setT] = useState(9);
  const reduced = useReducedMotion();
  useEffect(() => {
    const id = setInterval(() => setT((n) => (n + 1) % PERIOD), reduced ? 1600 : 900);
    return () => clearInterval(id);
  }, [reduced]);

  return (
    <>
      <div className="switch" role="group" aria-label="Avatar system">
        {(["portrait", "token", "sadu"] as const).map((id) => (
          <button key={id} type="button" aria-pressed={system === id} onClick={() => setSystem(id)}>
            {id === "portrait" ? "A" : id === "token" ? "B" : "C"}
          </button>
        ))}
      </div>
    <LayoutGroup id={`flow-${system}`}>
      <ol className="flow">
        {STAGES.map((stage, si) => {
          const here = FLOW.map((p, i) => ({ p, i, s: stageOf(i, t, p.outcome) })).filter((x) => x.s === si);
          return (
            <li key={stage.name} className="flow-row" data-tone={"tone" in stage ? stage.tone : undefined} data-outcome={si >= 4}>
              <div className="flow-stage">
                <span className="flow-name">{stage.name}</span>
                <span className="flow-sub">{stage.sub}</span>
              </div>
              <div className="flow-chips">
                {here.map(({ p, i }) => {
                  const age = t - i * SPACING;
                  const linked = si >= 2;
                  const justLinked = si === 2 && age - 2 * DWELL < 1;
                  return (
                    <motion.span
                      key={p.key}
                      layoutId={p.key}
                      className="chip"
                      transition={reduced ? { duration: 0 } : { type: "spring", stiffness: 260, damping: 30 }}
                    >
                      <Avatar
                        system={system}
                        person={p}
                        size={24}
                        state={!linked ? "unknown" : justLinked ? "resolving" : "known"}
                      />
                      <span>{linked ? p.name : p.spelling}</span>
                    </motion.span>
                  );
                })}
              </div>
              <span className="flow-count">{here.length}</span>
            </li>
          );
        })}
      </ol>
    </LayoutGroup>
    </>
  );
}
