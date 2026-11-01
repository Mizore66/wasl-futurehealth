import { HINTED, MASTER } from "../mark";
import type { Concept } from "./types";

/** c) صل from وصل as one thread: the ص bowl, the baseline, and the ل rising out of it. Chosen; the product draws it from `../mark`. */
export const sad: Concept = {
  id: "sad",
  name: "Sad-lam thread",
  idea: "The connected ص and ل of وصل drawn as a single unbroken thread.",
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
