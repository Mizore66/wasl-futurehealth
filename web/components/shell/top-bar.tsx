"use client";

import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { usePathname } from "next/navigation";
import { LayoutGrid } from "lucide-react";
import { AppearanceButton } from "./appearance-button";

const CRUMBS: [RegExp, string][] = [
  [/^\/app\/today/, "Today"],
  [/^\/app\/patients\/[^/]+\/audit/, "Audit trail"],
  [/^\/app\/patients/, "Patients"],
  [/^\/app\/referrals/, "Referrals"],
  [/^\/app\/recovery/, "Recovery"],
  [/^\/app\/review/, "Review desk"],
  [/^\/app\/pipeline/, "Pipeline"],
  [/^\/app\/results/, "Results"],
  [/^\/app\/agent-review/, "Agent review"],
  [/^\/app\/people/, "People"],
  [/^\/app\/?$/, "Programme"],
];

export function crumbFor(pathname: string): string {
  return CRUMBS.find(([re]) => re.test(pathname))?.[1] ?? "Workspace";
}

/** Audit carries its own Palette and Layout controls in the bar; every other page has the shared Appearance button. */
export function isAuditRoute(pathname: string): boolean {
  return /^\/app\/patients\/[^/]+\/audit(\/|$)/.test(pathname);
}

export const TOPBAR_SLOT = "data-topbar-slot";

/** The right-hand end of the bar: Appearance, or the slot a page portals its own controls into. */
export function TopActions() {
  const pathname = usePathname();
  if (isAuditRoute(pathname)) return <div {...{ [TOPBAR_SLOT]: "" }} className="flex items-center gap-2" />;
  return <AppearanceButton />;
}

/** Portals page controls into whichever top-bar slot is visible at this width (desktop bar or phone header). */
export function TopbarActionsPortal({ children }: { children: React.ReactNode }) {
  const [slot, setSlot] = useState<HTMLElement | null>(null);
  useEffect(() => {
    const pick = () => {
      const all = [...document.querySelectorAll<HTMLElement>(`[${TOPBAR_SLOT}]`)];
      setSlot(all.find((el) => el.offsetParent !== null) ?? all[0] ?? null);
    };
    pick();
    window.addEventListener("resize", pick);
    return () => window.removeEventListener("resize", pick);
  }, []);
  return slot ? createPortal(children, slot) : null;
}

/** Desktop bar above the page: where you are, then the page-level controls. */
export function TopBar() {
  const pathname = usePathname();
  return (
    <div className="sticky top-0 z-20 hidden min-h-[54px] items-center justify-between gap-3 border-b border-line bg-surface px-[clamp(18px,3vw,40px)] md:flex">
      <nav aria-label="Breadcrumb" className="flex items-center gap-2.5 text-[12px] text-muted">
        <LayoutGrid aria-hidden className="size-4" strokeWidth={1.5} />
        <span>Care workspace</span>
        <span aria-hidden>/</span>
        <b aria-current="page" className="font-semibold text-ink">
          {crumbFor(pathname)}
        </b>
      </nav>
      <TopActions />
    </div>
  );
}
