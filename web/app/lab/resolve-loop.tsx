"use client";

import { useEffect, useState } from "react";
import type { PersonSeed } from "@/components/avatar/traits";
import { LabAvatar as Avatar } from "./lab-avatar";
import type { SystemId } from "./systems";

const CYCLE = ["unknown", "resolving", "known"] as const;

/** Loops "?" → joining → face so the transition can be watched; reduced motion still steps, without the animation. */
export function ResolveLoop({ system, person, size }: { system: SystemId; person: PersonSeed; size: number }) {
  const [i, setI] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setI((n) => (n + 1) % CYCLE.length), 1400);
    return () => clearInterval(id);
  }, []);
  return <Avatar system={system} person={person} state={CYCLE[i]} size={size} />;
}
