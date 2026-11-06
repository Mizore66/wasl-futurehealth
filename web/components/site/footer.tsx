import Link from "next/link";
import { Logo } from "@/components/brand/mark";
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
      <HonestyLabel kind={honesty} className="inline h-auto whitespace-normal bg-transparent px-0 text-[14px] font-normal text-(--muted) [&>[data-dot]]:hidden">
        {qualifier}
      </HonestyLabel>{" "}
      {rest}{" "}
      <Link href="/experts" className="inline-flex min-h-11 items-center text-(--teal) underline underline-offset-[0.18em] hover:text-(--ink)">
        The evidence, for experts
      </Link>
      .
    </p>
  );

  if (big) {
    return (
      <footer className={cn(wrap, "pb-12 pt-20")}>
        <div aria-hidden>
          <Logo className="text-[clamp(2.75rem,1rem_+_7vw,8rem)]" />
        </div>
        <div className="mt-8">{print}</div>
      </footer>
    );
  }

  return (
    <footer className={cn(wrap, "flex flex-wrap items-center justify-between gap-6 py-12")}>
      <Wordmark size="sm" className="min-h-11" />
      <div className="max-w-[70ch]">{print}</div>
    </footer>
  );
}
