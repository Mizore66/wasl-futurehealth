"use client";

import { useTransition } from "react";
import { useRouter } from "next/navigation";
import { Check, SlidersHorizontal, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { chip, chipSet, chipUnset, type FilterSpec } from "./types";

/**
 * Under 640px the filter chips fold into one "Filters (n)" button and this bottom sheet. Each choice applies at once
 * (the URL stays the source of truth), so the result count on the closing button is the real one, not a guess.
 */
export function FilterSheet({
  filters,
  clearHref,
  total,
  one = "result",
  many = `${one}s`,
  className,
}: {
  filters: FilterSpec[];
  clearHref: string;
  total: number;
  one?: string;
  many?: string;
  className?: string;
}) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const go = (href: string) => startTransition(() => router.push(href, { scroll: false }));
  const set = filters.filter((f) => f.value).length;

  return (
    <Dialog>
      <DialogTrigger className={cn(chip, set ? chipSet : chipUnset, className)}>
        <SlidersHorizontal aria-hidden="true" className="size-4" />
        Filters{set > 0 && ` (${set})`}
      </DialogTrigger>
      <DialogContent
        showCloseButton={false}
        aria-busy={pending || undefined}
        className="top-auto bottom-0 left-0 flex max-h-[85dvh] max-w-none translate-x-0 translate-y-0 flex-col gap-0 rounded-none rounded-t-[12px] border-line bg-surface p-0 text-ink shadow-none sm:max-w-none"
      >
        <header className="flex items-center justify-between border-b border-line ps-4">
          <DialogTitle className="text-body font-medium">Filters</DialogTitle>
          <DialogDescription className="sr-only">Each choice applies straight away.</DialogDescription>
          <DialogClose asChild>
            <Button variant="quiet" size="touch" aria-label="Close" className="w-11 bg-transparent px-0">
              <X aria-hidden="true" />
            </Button>
          </DialogClose>
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto px-4">
          {filters.map((f) => (
            <section key={f.label} aria-label={f.label} className="border-b border-line py-3 last:border-b-0">
              <h3 className="m-0 mb-1 text-small font-medium text-muted">{f.label}</h3>
              <ul className="m-0 list-none p-0">
                {f.options.map((o) => {
                  const on = o.value === f.value;
                  return (
                    <li key={o.value}>
                      <button
                        type="button"
                        aria-pressed={on}
                        onClick={() => go(on ? f.clearHref : o.href)}
                        className={cn("flex min-h-11 w-full cursor-pointer items-center gap-2 rounded-chip px-2 text-start text-body hover:bg-ground", on && "font-medium", o.count === 0 && "text-muted")}
                      >
                        <Check aria-hidden="true" className={cn("size-4 shrink-0 text-jade-700", !on && "invisible")} />
                        <span className="min-w-0 flex-1">{o.label}</span> <span className="shrink-0 font-normal text-muted tabular-nums">{o.count}</span>
                      </button>
                    </li>
                  );
                })}
              </ul>
            </section>
          ))}
        </div>
        <footer className="flex items-center justify-between gap-3 border-t border-line p-4">
          <Button variant="quiet" size="touch" disabled={set === 0} onClick={() => go(clearHref)} className="bg-transparent">
            Clear all
          </Button>
          <DialogClose asChild>
            <Button variant="cta" size="touch" aria-busy={pending || undefined}>
              {pending ? "Updating…" : `Show ${total} ${total === 1 ? one : many}`}
            </Button>
          </DialogClose>
        </footer>
      </DialogContent>
    </Dialog>
  );
}
