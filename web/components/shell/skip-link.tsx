"use client";

import { usePathname } from "next/navigation";

const SKIP = "sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-50 focus:rounded-control focus:bg-ink focus:px-4 focus:py-3 focus:text-body focus:text-ground focus:no-underline";

/** Worklist pages whose list section carries `id="worklist"` (Today, Referrals). */
const WORKLISTS = new Set(["/app/today", "/app/referrals"]);

/** Move focus to the first row of the worklist, or to the list itself when it has no rows (WCAG 2.4.1). */
export function focusWorklist(doc: Document = document): boolean {
  const list = doc.getElementById("worklist");
  if (!list) return false;
  (list.querySelector<HTMLElement>("[role=tabpanel] button[aria-pressed], button[data-case-row]") ?? list).focus();
  return true;
}

/**
 * First focusable element on every page with a persistent `<main id="main">`. Visible only while focused. On a
 * worklist page a second link skips the sidebar, filters, tabs and search and lands on the first row.
 */
export function SkipLink() {
  const path = usePathname();
  return (
    <>
      <a href="#main" className={SKIP}>
        Skip to main content
      </a>
      {WORKLISTS.has(path) && (
        <a
          href="#worklist"
          className={SKIP}
          onClick={(e) => {
            if (focusWorklist()) e.preventDefault();
          }}
        >
          Skip to worklist
        </a>
      )}
    </>
  );
}
