import type { Concept } from "./types";

/** d) A W from one continuous thread, knotted once at the middle peak. */
export const w: Concept = {
  id: "w",
  name: "Thread W",
  idea: "One line, no lift, with a single loop where the two halves meet.",
  draw: (ink) => (
    <path
      d="M3.5 6L9.5 26L17.5 9.5C20.5 3 11 1.5 12.6 8L22.5 26L28.5 6"
      fill="none"
      stroke={ink}
      strokeWidth={3.2}
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  ),
};
