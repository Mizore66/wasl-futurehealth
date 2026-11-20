"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
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

/**
 * On the landing, the header scrolls away with the story. Once the evidence section reaches the top, this
 * compact bar slides in so the dashboard and How it works stay one click away.
 */
export function LandingStickyNav() {
  const path = usePathname();
  const [shown, setShown] = useState(false);

  useEffect(() => {
    if (path !== "/") return;
    let frame = 0;
    const check = () => {
      frame = 0;
      const top = document.getElementById("evidence")?.getBoundingClientRect().top;
      setShown(top !== undefined && top <= 80);
    };
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(check);
    };
    check();
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("resize", onScroll);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("resize", onScroll);
    };
  }, [path]);

  if (path !== "/") return null;

  return (
    <div
      inert={!shown}
      data-shown={shown || undefined}
      className={cn(
        "fixed inset-x-0 top-0 z-40 border-b border-(--line) bg-(--paper)/95 backdrop-blur transition-transform duration-300 ease-out motion-reduce:transition-none",
        shown ? "translate-y-0" : "-translate-y-full",
      )}
    >
      <nav aria-label="Site" className={cn(wrap, "flex items-center justify-between gap-4 py-2")}>
        <Wordmark size="sm" className="min-h-11" />
        <div className="flex items-center gap-6 text-[16px]">
          <Link href="/how-it-works" className="inline-flex min-h-11 items-center text-(--ink) no-underline hover:text-(--teal) max-sm:hidden">
            How it works
          </Link>
          <Link
            href="/app"
            className="inline-flex min-h-11 items-center bg-(--ink) px-4 py-2 font-semibold text-(--paper) no-underline hover:bg-(--teal) hover:text-(--paper)"
          >
            Open the dashboard
          </Link>
        </div>
      </nav>
    </div>
  );
}
