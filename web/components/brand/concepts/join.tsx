import type { Concept } from "./types";

/** a) Two records, two strands, one person: the strands merge and end in a single dot. */
export const join: Concept = {
  id: "join",
  name: "Join",
  idea: "Two spellings arrive as two strands and leave as one person.",
  draw: (ink) => (
    <g fill="none" stroke={ink} strokeWidth={3.4} strokeLinecap="round">
      <path d="M6 8.5C12.5 8.5 13 16 19 16M6 23.5C12.5 23.5 13 16 19 16H22" />
      <circle cx={24.5} cy={16} r={4.6} fill={ink} stroke="none" />
      <circle cx={6} cy={8.5} r={2.5} fill={ink} stroke="none" />
      <circle cx={6} cy={23.5} r={2.5} fill={ink} stroke="none" />
    </g>
  ),
};
