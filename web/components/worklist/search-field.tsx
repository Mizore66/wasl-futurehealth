"use client";

import { useEffect, useRef } from "react";
import { cx } from "@/lib/cx";

/*
 * One search control for the worklists.
 *  - The whole control (icon, text, border) gets the focus indicator via :focus-within, not just the caret.
 *  - Placeholder text uses `muted` (6.5:1 on the surface) at full opacity, so no browser default dims it.
 *  - The label stays for screen readers; the placeholder only hints at the format.
 */

const REFOCUS = "wasl:refocus-search";

/** Call before a submit that reloads the page; the field with the same `restoreKey` takes focus again on mount. */
export function rememberSearchFocus(restoreKey: string) {
  try {
    sessionStorage.setItem(REFOCUS, restoreKey);
  } catch {
    /* storage blocked: focus simply is not restored */
  }
}

type Props = Omit<React.ComponentProps<"input">, "type"> & { label: string; restoreKey?: string };

export function SearchField({ label, restoreKey, className, ...input }: Props) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (!restoreKey) return;
    try {
      if (sessionStorage.getItem(REFOCUS) !== restoreKey) return;
      sessionStorage.removeItem(REFOCUS);
    } catch {
      return;
    }
    const el = ref.current;
    if (!el) return;
    el.focus();
    el.setSelectionRange(el.value.length, el.value.length);
  }, [restoreKey]);

  return (
    <label
      className={cx(
        "flex items-center gap-2 rounded-control border border-control bg-surface px-2.5",
        "focus-within:border-hue focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-hue",
        className,
      )}
    >
      <svg aria-hidden width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" className="text-muted">
        <circle cx="7" cy="7" r="4.5" />
        <path d="M10.5 10.5l3 3" />
      </svg>
      <span className="sr-only">{label}</span>
      <input ref={ref} type="search" {...input} className="h-11 min-w-0 flex-1 border-0 bg-transparent text-body text-ink outline-hidden placeholder:text-muted placeholder:opacity-100" />
    </label>
  );
}
