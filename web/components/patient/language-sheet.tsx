"use client";

import Link from "next/link";
import { useState } from "react";
import { cn } from "@/lib/utils";
import { BottomSheet } from "./bottom-sheet";
import type { Lang } from "./i18n";

export interface LanguageOption {
  lang: Lang;
  dir: "rtl" | "ltr";
  label: string;
  href: string;
  font: string;
}

export function LanguageSheet({
  current,
  currentLabel,
  title,
  closeLabel,
  options,
}: {
  current: Lang;
  currentLabel: string;
  title: string;
  closeLabel: string;
  options: LanguageOption[];
}) {
  const [open, setOpen] = useState(false);
  return (
    <BottomSheet
      open={open}
      onOpenChange={setOpen}
      title={title}
      closeLabel={closeLabel}
      trigger={
        <button
          type="button"
          aria-label={`${title}: ${currentLabel}`}
          className="ms-auto flex h-11 cursor-pointer items-center gap-1.5 rounded-control border border-control bg-surface px-3 text-[15px] text-ink"
        >
          <GlobeIcon />
          <span>{currentLabel}</span>
        </button>
      }
    >
      <ul className="m-0 flex list-none flex-col gap-2 p-0">
        {options.map((o) => {
          const on = o.lang === current;
          return (
            <li key={o.lang}>
              <Link
                href={o.href}
                lang={o.lang}
                dir={o.dir}
                aria-current={on ? "true" : undefined}
                onClick={() => setOpen(false)}
                style={{ fontFamily: o.font }}
                className={cn(
                  "flex min-h-[52px] w-full items-center gap-3 rounded-control border px-4 text-[17px] text-ink no-underline hover:text-ink",
                  on ? "border-control bg-raised" : "border-line bg-surface",
                )}
              >
                <span className="flex-1">{o.label}</span>
                <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke={on ? "#0a0a0a" : "transparent"} strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M5 12l5 5 9-10" />
                </svg>
              </Link>
            </li>
          );
        })}
      </ul>
    </BottomSheet>
  );
}

function GlobeIcon() {
  return (
    <svg aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18" />
      <path d="M12 3c2.5 2.5 3.8 5.5 3.8 9s-1.3 6.5-3.8 9c-2.5-2.5-3.8-5.5-3.8-9S9.5 5.5 12 3z" />
    </svg>
  );
}
