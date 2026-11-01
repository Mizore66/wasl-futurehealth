import type { Concept } from "./types";

/** c) صل from وصل as one thread: the ص bowl, the baseline, and the ل rising out of it. */
export const sad: Concept = {
  id: "sad",
  name: "Sad-lam thread",
  idea: "The connected ص and ل of وصل drawn as a single unbroken thread.",
  draw: (ink) => (
    <g fill="none" stroke={ink} strokeWidth={3.2} strokeLinecap="round" strokeLinejoin="round">
      <path d="M8 4.5V21C8 27.5 2.5 27.5 2.5 23.5M8 22H15C15 14 28.5 12.5 28.5 18.5C28.5 22 24 22 15 22" />
    </g>
  ),
  small: (ink) => (
    <g fill="none" stroke={ink} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round">
      <path d="M5 3V10.5C5 13.5 2.5 13.5 2.5 12.5M5 11H8C8 5 14.5 4.5 14.5 8.5C14.5 11 12 11 8 11" />
    </g>
  ),
};
