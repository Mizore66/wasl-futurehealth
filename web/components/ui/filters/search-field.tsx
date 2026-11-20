"use client";

import { useEffect, useId, useRef, useState, type ComponentProps } from "react";
import { useRouter } from "next/navigation";
import { CornerDownLeft, Search, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type Props = Omit<ComponentProps<"input">, "type" | "value" | "defaultValue" | "onChange"> & {
  label: string;
  /** The search the URL holds now. */
  defaultValue?: string;
  /** The URL without this search, opened when a committed search is cleared. */
  clearHref: string;
  /** Renders a submit button with this name. The field sits in a GET form, which Enter also submits. */
  submitLabel?: string;
};

/**
 * A search box for a GET form: a leading icon, then a `/` hint that turns into a clear button once there is text.
 * `/` anywhere outside a field focuses it, and Escape clears it.
 */
export function SearchField({ label, defaultValue = "", clearHref, submitLabel, className, id, ...input }: Props) {
  const router = useRouter();
  const fieldId = useId();
  const ref = useRef<HTMLInputElement>(null);
  const [value, setValue] = useState(defaultValue);
  const [committed, setCommitted] = useState(defaultValue);
  if (committed !== defaultValue) {
    setCommitted(defaultValue);
    setValue(defaultValue);
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "/" || e.metaKey || e.ctrlKey || e.altKey) return;
      const el = e.target as HTMLElement | null;
      if (el && (el.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName))) return;
      e.preventDefault();
      ref.current?.focus();
      ref.current?.select();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  const clear = () => {
    setValue("");
    ref.current?.focus();
    if (committed) router.push(clearHref, { scroll: false });
  };

  return (
    <div
      className={cn(
        "flex min-h-11 min-w-0 items-center rounded-control border border-input bg-surface ps-3 text-muted",
        "focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-hue",
        className,
      )}
    >
      <Search aria-hidden="true" className="size-4 shrink-0" />
      <label htmlFor={id ?? fieldId} className="sr-only">
        {label}
      </label>
      <input
        ref={ref}
        id={id ?? fieldId}
        type="search"
        autoComplete="off"
        placeholder={label}
        {...input}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key !== "Escape" || !value) return;
          e.preventDefault();
          clear();
        }}
        className="h-11 min-w-0 flex-1 border-0 bg-transparent px-2 text-body text-ink outline-hidden placeholder:text-muted placeholder:opacity-100 [&::-webkit-search-cancel-button]:appearance-none"
      />
      {value ? (
        <button type="button" onClick={clear} aria-label="Clear search" className="inline-flex size-11 shrink-0 cursor-pointer items-center justify-center text-muted hover:text-ink">
          <X aria-hidden="true" className="size-4" />
        </button>
      ) : (
        <kbd aria-hidden="true" className="me-2 hidden rounded-[3px] border border-line px-1.5 font-sans text-small sm:inline">
          /
        </kbd>
      )}
      {submitLabel && (
        <Button type="submit" variant="quiet" size="touch" aria-label={submitLabel} title={submitLabel} className="w-11 rounded-s-none bg-transparent px-0 text-jade-700 hover:bg-jade-50">
          <CornerDownLeft aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}
