"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";
import { wrap } from "./styles";
import { Wordmark } from "./wordmark";

const LINKS = [
  { href: "/how-it-works", label: "How it works" },
  { href: "/#evidence", label: "Evidence" },
];

export function SiteHeader() {
  const path = usePathname();

  return (
    <nav
      aria-label="Primary"
      className={cn(wrap, "flex flex-wrap items-center justify-between gap-x-4 gap-y-4 py-5 md:gap-x-8 md:py-7")}
    >
      <Wordmark className="min-h-11" />
      <div className="flex items-center gap-8 max-md:contents">
      <div className="flex items-center gap-8 text-[18px] max-md:order-last max-md:w-full max-md:gap-6">
        {LINKS.map((l) => {
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
          "px-[22px] py-[13px] text-[18px]",
          "max-md:px-4 max-md:py-2.5 max-md:text-[16px]",
        )}
      >
        Open the dashboard
      </Link>
      </div>
    </nav>
  );
}
