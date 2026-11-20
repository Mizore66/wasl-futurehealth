import Link from "next/link";
import type { MouseEvent, ReactNode } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { pageWindow, pagerTargets, rangeText, type PagerControl, type Paging } from "./paging";

const control = "h-11 min-w-9 px-2 font-normal tabular-nums text-ink";
const current = "bg-active font-semibold text-jade-800 hover:bg-active";
const STEPS = { prev: { label: "Previous page", icon: <ChevronLeft aria-hidden /> }, next: { label: "Next page", icon: <ChevronRight aria-hidden /> } };

type Target = { key: PagerControl; to: number | null; label: string; body: ReactNode; current?: boolean };
type FrameProps = { paging: Paging<unknown>; label: string; testId?: string; complete?: boolean; className?: string };

/** An incomplete list cannot name its last page, so it pages by the chevrons alone. */
function Frame({ paging, label, testId, complete = true, className, render }: FrameProps & { render: (t: Target) => ReactNode }) {
  if (paging.pages === 1) return null;
  const targets = pagerTargets(paging);
  const step = (key: "prev" | "next") => render({ key, to: targets[key], ...STEPS[key], body: STEPS[key].icon });
  return (
    <nav aria-label={label} className={cn("flex items-center justify-between gap-3", className)}>
      {testId && <p data-testid={testId} className="m-0 min-w-0 text-small text-muted tabular-nums">{rangeText(paging, complete)}</p>}
      <div className="flex shrink-0 items-center gap-0.5">
        {step("prev")}
        {complete && pageWindow(paging.page, paging.pages).map((slot, i) =>
          slot === "gap" ? (
            <span key={`gap-${i}`} aria-hidden className="inline-grid w-9 place-items-center text-muted">…</span>
          ) : (
            render({ key: slot, to: slot, label: `Page ${slot}`, body: slot, current: slot === paging.page })
          ),
        )}
        {step("next")}
      </div>
    </nav>
  );
}

export function Pager({ onPage, ...props }: FrameProps & { onPage: (page: number, via: PagerControl) => void }) {
  return (
    <Frame {...props} render={({ key, to, label, body, current: here }) => (
      <Button key={key} type="button" variant="tertiary" data-focus={`pager-${key}`} aria-label={label} aria-current={here ? "page" : undefined}
        disabled={to === null} className={cn(control, here && current)} onClick={() => to !== null && !here && onPage(to, key)}>
        {body}
      </Button>
    )} />
  );
}

export function PagerLinks({ hrefFor, onFollow, ...props }: FrameProps & { hrefFor: (page: number) => string; onFollow?: (href: string) => (e: MouseEvent<HTMLAnchorElement>) => void }) {
  return (
    <Frame {...props} render={({ key, to, label, body, current: here }) => {
      if (to === null) return <span key={key} aria-hidden className={cn(control, "inline-grid place-items-center text-muted opacity-40 [&_svg]:size-4")}>{body}</span>;
      if (here) return <span key={key} aria-current="page" aria-label={label} className={cn(control, current, "inline-grid place-items-center rounded-control")}>{body}</span>;
      const href = hrefFor(to);
      return (
        <Button key={key} asChild variant="tertiary" className={cn(control, "no-underline")}>
          <Link href={href} aria-label={label} rel={key === "prev" ? "prev" : key === "next" ? "next" : undefined} onClick={onFollow?.(href)}>{body}</Link>
        </Button>
      );
    }} />
  );
}
