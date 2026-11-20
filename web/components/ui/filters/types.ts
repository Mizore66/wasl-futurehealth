/** One choice in a FilterMenu. `href` is the page URL with this value chosen, built on the server so the URL stays the source of truth. */
export type FilterOption = { value: string; label: string; count: number; href: string };

/** One attribute the list can be narrowed by, such as Programme. `value` is the chosen option's value, if any. */
export type FilterSpec = { label: string; value?: string; options: FilterOption[]; clearHref: string };

/** The 44px chip every filter control shares, in its unset and set states. Focus keeps the app's global outline. */
export const chip = "inline-flex h-11 shrink-0 cursor-pointer items-center gap-1.5 rounded-chip border px-3 text-body whitespace-nowrap no-underline";
export const chipUnset = "border-control bg-surface text-ink hover:bg-ground";
export const chipSet = "border-jade-600 bg-jade-50 text-ink";
/** The underlined-tab row shared by SegmentedFilter (links) and SegmentedToggle (client state). */
export const segmentRow = "flex min-w-0 gap-1 overflow-x-auto border-b border-line [scrollbar-width:thin]";
export const segmentTab = "inline-flex min-h-11 shrink-0 items-center gap-1.5 border-b-2 px-2.5 text-body whitespace-nowrap text-ink no-underline hover:bg-ground";
export const segmentOn = "border-jade-600 font-medium";
