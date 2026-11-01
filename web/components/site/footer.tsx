import Link from "next/link";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { HonestyKind } from "@/lib/honesty";
import { cn } from "@/lib/utils";
import { small, wrap } from "./styles";
import { Wordmark } from "./wordmark";

/**
 * The small print. Its first sentence is the page's honesty label, written as a plain qualifier
 * (the C direction cuts pills) but still marked `data-honesty` for the honesty lint.
 */
export function SiteFooter({
  honesty,
  qualifier,
  rest,
  big = false,
}: {
  honesty: HonestyKind;
  qualifier: string;
  rest: string;
  big?: boolean;
}) {
  const print = (
    <p className={cn(small, "max-w-[90ch] leading-[1.5]")}>
      <HonestyLabel kind={honesty} className="inline h-auto whitespace-normal bg-transparent px-0 text-[14px] text-(--muted)">
        {qualifier}
      </HonestyLabel>{" "}
      {rest}{" "}
      <Link href="/experts" className="text-(--teal) underline underline-offset-[0.18em] hover:text-(--ink)">
        The evidence, for experts
      </Link>
      .
    </p>
  );

  if (big) {
    return (
      <footer className={cn(wrap, "pb-12 pt-20")}>
        <div aria-hidden className="flex items-center gap-[clamp(1rem,2vw,2rem)] text-[clamp(2.5rem,1rem_+_6vw,7rem)] font-bold leading-none tracking-[-0.04em]">
          <span>Wasl</span>
          <span className="h-[0.8em] w-[clamp(4px,0.6vw,8px)] bg-(--teal)" />
          <span lang="ar" dir="rtl" className="font-arabic font-semibold tracking-normal text-(--teal)">
            وصل
          </span>
        </div>
        <div className="mt-8">{print}</div>
      </footer>
    );
  }

  return (
    <footer className={cn(wrap, "flex flex-wrap items-center justify-between gap-6 py-12")}>
      <Wordmark size="sm" />
      <div className="max-w-[70ch]">{print}</div>
    </footer>
  );
}
