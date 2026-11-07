import type { KeyboardEvent } from "react";

/**
 * Roving tabindex for a `role=tablist` of buttons, with automatic activation:
 * switching a saved view is a cheap client-side filter, so focus and selection move together.
 * Exactly one tab (the selected one) is in the Tab order; arrows, Home and End move between them.
 * Pair `tabProps(id)` on each tab with `listProps` on the tablist.
 */
export function useRovingTabs<T extends string>({ ids, selected, onSelect, prefix }: { ids: readonly T[]; selected: T; onSelect: (id: T) => void; prefix: string }) {
  const tabId = (id: T) => `${prefix}-tab-${id}`;
  const panelId = `${prefix}-panel`;

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return;
    const at = ids.findIndex((id) => tabId(id) === (e.target as HTMLElement).id);
    if (at < 0) return;
    const next = { ArrowRight: (at + 1) % ids.length, ArrowLeft: (at - 1 + ids.length) % ids.length, Home: 0, End: ids.length - 1 }[e.key];
    if (next === undefined) return;
    e.preventDefault();
    onSelect(ids[next]);
    document.getElementById(tabId(ids[next]))?.focus();
  };

  return {
    panelId,
    tabId,
    listProps: { role: "tablist" as const, onKeyDown },
    tabProps: (id: T) => ({
      id: tabId(id),
      role: "tab" as const,
      type: "button" as const,
      "aria-selected": selected === id,
      "aria-controls": panelId,
      tabIndex: selected === id ? 0 : -1,
    }),
  };
}

/** Class for a tab's focus ring: inset, because the tablist scrolls and would clip an outer outline. */
export const TAB_FOCUS = "focus-visible:-outline-offset-2";
