import { HINTED, MASTER } from "../mark";
import type { Concept } from "./types";

/** c) Began as صل from وصل as one thread; now the full word وصل in the same stroke. Chosen; the product draws it from `../mark`. */
export const sad: Concept = {
  id: "sad",
  name: "Wasl thread",
  idea: "وصل drawn as one weight of stroke: the و, the ص bowl and the ل joined along the baseline.",
  draw: (ink) => (
    <g fill="none" stroke={ink} strokeWidth={MASTER.stroke} strokeLinecap="round" strokeLinejoin="round">
      <path d={MASTER.d} />
    </g>
  ),
  small: (ink) => (
    <g fill="none" stroke={ink} strokeWidth={HINTED.stroke} strokeLinecap="round" strokeLinejoin="round">
      <path d={HINTED.d} />
    </g>
  ),
};
