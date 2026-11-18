import "server-only";
import { ALL_DICTS } from "./all";
import { en } from "./en";
import type { DictSet } from "./index";

export { ALL_DICTS };

/**
 * The dictionaries one phone surface may render. Server-only: the translation-review route passes the drafts to its
 * client components through `DictsProvider` props, so they reach that route's payload only, never a patient bundle.
 */
export function dictsForSurface(surface: "patient" | "review"): DictSet {
  return surface === "review" ? ALL_DICTS : { en };
}
