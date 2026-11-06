"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import { wrap, wrapExpert } from "./styles";
import { Wordmark } from "./wordmark";

/** The buyer pages link to each other; the expert page links to its own sections. */
const BUYER_LINKS = [
  { href: "/how-it-works", label: "How it works" },
];

const EXPERT_LINKS = [
  { href: "#outcomes", label: "Outcomes" },
  { href: "#safety", label: "Safety" },
  { href: "#deploy", label: "Deployment" },
  { href: "#plan", label: "Pilot" },
  { href: "#limits", label: "Limits" },
];

export function SiteHeader() {
  const path = usePathname();
  const expert = path === "/experts";
  const links = expert ? EXPERT_LINKS : BUYER_LINKS;

  return (
    <nav
      aria-label="Primary"
      className={cn(expert ? wrapExpert : wrap, "flex flex-wrap items-center justify-between gap-x-4 gap-y-4 py-5 md:gap-x-8 md:py-7")}
    >
      <Wordmark className="min-h-11" />
      <div className="flex items-center gap-8 max-md:contents">
      <div className={cn("flex items-center gap-8", expert ? "text-[16px]" : "text-[18px]", "max-md:order-last max-md:w-full max-md:gap-6", expert && "max-lg:hidden")}>
        {links.map((l) => {
          const current = l.href === path;
          return (
            <Link
              key={l.href}
              href={l.href}
              aria-current={current ? "page" : undefined}
              className={cn(
                "inline-flex min-h-11 items-center hover:text-(--teal)",
                current ? "font-bold text-(--teal) underline underline-offset-[0.18em]" : "text-(--ink) no-underline",
              )}
            >
              {l.label}
            </Link>
          );
        })}
      </div>
      <Link
        href="/app"
        className={cn(
          "inline-flex min-h-11 items-center bg-(--ink) font-semibold text-(--paper) no-underline hover:bg-(--teal) hover:text-(--paper)",
          expert ? "rounded px-[18px] py-[11px] text-[16px]" : "px-[22px] py-[13px] text-[18px]",
          "max-md:px-4 max-md:py-2.5 max-md:text-[16px]",
        )}
      >
        Open the dashboard
      </Link>
      </div>
    </nav>
  );
}
