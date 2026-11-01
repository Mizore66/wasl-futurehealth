import type { Concept } from "./types";

/** b) The receipt: a slip of proof, closed by a seal that breaks its corner, as in the landing's last scene. */
export const seal: Concept = {
  id: "seal",
  name: "Sealed slip",
  idea: "A وصل is also a receipt. The loop is done when the slip carries its seal.",
  draw: (ink) => (
    <g fill="none" stroke={ink} strokeWidth={3} strokeLinecap="round" strokeLinejoin="round">
      <path d="M23.5 14.5V6.5Q23.5 3 20 3H9Q5.5 3 5.5 6.5V25Q5.5 28.5 9 28.5H14" />
      <path d="M10 9H19M10 14.5H15.5" />
      <circle cx={22.5} cy={23} r={6.5} fill={ink} stroke="none" />
    </g>
  ),
};
