"use client";

import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { usePathname } from "next/navigation";
import { LayoutGrid } from "lucide-react";
import { RunMeta } from "./run-meta";
import { TourButton } from "./guided-tour";

const CRUMBS: [RegExp, string][] = [
  [/^\/app\/today/, "Today"],
  [/^\/app\/patients\/[^/]+\/audit/, "Audit trail"],
  [/^\/app\/patients/, "Patients"],
  [/^\/app\/referrals/, "Referrals"],
  [/^\/app\/recovery/, "Recovery"],
  [/^\/app\/review/, "Review desk"],
  [/^\/app\/evidence/, "Evidence"],
  [/^\/app\/pipeline/, "Pipeline"],
  [/^\/app\/results/, "Results"],
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

/** The right-hand end of the bar: the slot a page portals its own controls into (only Audit has any). */
export function TopActions() {
  const pathname = usePathname();
  if (isAuditRoute(pathname)) return <div {...{ [TOPBAR_SLOT]: "" }} className="flex items-center gap-2" />;
  return null;
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

/** Desktop bar above the page: where you are on the left; the run notice and any page controls on the right. */
export function TopBar() {
  const pathname = usePathname();
  return (
    <div className="sticky top-0 z-20 hidden min-h-[54px] items-center justify-between gap-3 border-b border-line bg-surface px-[clamp(18px,3vw,40px)] md:flex">
      <nav aria-label="Breadcrumb" data-tour="breadcrumb" className="flex flex-none items-center gap-2.5 text-[12px] text-muted">
        <LayoutGrid aria-hidden className="size-4" strokeWidth={1.5} />
        <span>Care workspace</span>
        <span aria-hidden>/</span>
        <b aria-current="page" className="font-semibold text-ink">
          {crumbFor(pathname)}
        </b>
      </nav>
      {/* Positioned, so the notice's drop-down anchors to the bar's right edge. */}
      <div className="relative flex min-w-0 items-center justify-end gap-2">
        <div data-tour="run" className="min-w-0">
          <RunMeta />
        </div>
        <TopActions />
        <TourButton />
      </div>
    </div>
  );
}
