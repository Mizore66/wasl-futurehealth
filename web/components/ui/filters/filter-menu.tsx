"use client";

import { useId, useRef, useState, useTransition, type KeyboardEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { Check, ChevronDown, X } from "lucide-react";
import { Popover } from "radix-ui";
import { cn } from "@/lib/utils";
import { chip, chipSet, chipUnset, type FilterOption, type FilterSpec } from "./types";

/** Past this many options the menu adds a field that narrows them. */
const FILTERABLE_OVER = 8;
const TYPEAHEAD_RESET_MS = 500;

/**
 * A chip that opens a listbox of one attribute's values, each with its count. Choosing a value opens its URL;
 * choosing the chosen value again, or the chip's remove link, clears it.
 * Keyboard: Enter, Space or Down Arrow opens; arrows, Home, End and type-ahead move; Enter selects and closes;
 * Escape closes. Focus returns to the chip either way.
 */
export function FilterMenu({ label, value, options, clearHref, disabled, className }: FilterSpec & { disabled?: boolean; className?: string }) {
  const router = useRouter();
  const [pending, startTransition] = useTransition();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const typed = useRef({ text: "", at: 0 });
  const id = useId();
  const optionId = (i: number) => `${id}-option-${i}`;

  const selected = options.find((o) => o.value === value);
  const filterable = options.length > FILTERABLE_OVER;
  const needle = query.trim().toLowerCase();
  const shown = needle ? options.filter((o) => o.label.toLowerCase().includes(needle)) : options;
  const current = Math.min(active, shown.length - 1);

  const show = (next: boolean) => {
    if (disabled) return;
    if (next) {
      setQuery("");
      setActive(Math.max(0, options.findIndex((o) => o.value === value)));
    }
    setOpen(next);
  };

  const choose = (o: FilterOption) => {
    setOpen(false);
    startTransition(() => router.push(o.value === value ? clearHref : o.href, { scroll: false }));
  };

  const move = (to: number) => {
    const i = Math.max(0, Math.min(shown.length - 1, to));
    setActive(i);
    document.getElementById(optionId(i))?.scrollIntoView?.({ block: "nearest" });
  };

  const typeahead = (key: string) => {
    const now = Date.now();
    const text = (now - typed.current.at > TYPEAHEAD_RESET_MS ? "" : typed.current.text) + key.toLowerCase();
    typed.current = { text, at: now };
    const from = text.length === 1 ? current + 1 : current;
    const order = [...shown.slice(from), ...shown.slice(0, from)];
    const hit = order.find((o) => o.label.toLowerCase().startsWith(text));
    if (hit) move(shown.indexOf(hit));
  };

  const onListKey = (e: KeyboardEvent, inField: boolean) => {
    const keys: Record<string, () => void> = {
      ArrowDown: () => move(current + 1),
      ArrowUp: () => move(current - 1),
      Home: () => move(0),
      End: () => move(shown.length - 1),
      Enter: () => shown[current] && choose(shown[current]),
    };
    if (!inField) keys[" "] = keys.Enter;
    const act = keys[e.key];
    if (act) {
      e.preventDefault();
      act();
    } else if (!inField && e.key.length === 1 && !e.metaKey && !e.ctrlKey && !e.altKey) {
      e.preventDefault();
      typeahead(e.key);
    }
  };

  const activeId = shown[current] ? optionId(current) : undefined;

  return (
    <div className={cn("inline-flex shrink-0", className)}>
      <Popover.Root open={open} onOpenChange={show}>
        <Popover.Trigger
          aria-haspopup="listbox"
          aria-disabled={disabled || undefined}
          aria-busy={pending || undefined}
          onKeyDown={(e) => {
            if (e.key !== "ArrowDown" || open) return;
            e.preventDefault();
            show(true);
          }}
          className={cn(chip, selected ? cn(chipSet, "rounded-e-none border-e-0") : chipUnset, "max-w-72", disabled && "cursor-default opacity-50", pending && "opacity-70")}
        >
          {selected ? (
            <span className="truncate">
              {label}: <span className="font-medium">{selected.label}</span>
            </span>
          ) : (
            <>
              {label}
              <ChevronDown aria-hidden="true" className="size-4 text-muted" />
            </>
          )}
        </Popover.Trigger>
        <Popover.Portal>
          <Popover.Content
            align="start"
            sideOffset={4}
            collisionPadding={16}
            onOpenAutoFocus={(e) => {
              e.preventDefault();
              (filterable ? inputRef.current : listRef.current)?.focus();
            }}
            className="z-50 flex max-h-[min(24rem,var(--radix-popover-content-available-height))] w-72 max-w-[calc(100vw-2rem)] flex-col rounded-control border border-line bg-surface p-1 text-body text-ink shadow-[0_8px_24px_rgb(11_36_39/0.12)]"
          >
            {filterable && (
              <input
                ref={inputRef}
                type="text"
                role="combobox"
                aria-label={`Filter ${label} options`}
                aria-expanded="true"
                aria-controls={`${id}-list`}
                aria-autocomplete="list"
                aria-activedescendant={activeId}
                placeholder={`Filter ${label.toLowerCase()}`}
                value={query}
                onChange={(e) => {
                  setQuery(e.target.value);
                  setActive(0);
                }}
                onKeyDown={(e) => onListKey(e, true)}
                className="mb-1 h-11 shrink-0 rounded-chip border border-input bg-surface px-2.5 text-body text-ink placeholder:text-muted placeholder:opacity-100"
              />
            )}
            <div
              ref={listRef}
              id={`${id}-list`}
              role="listbox"
              aria-label={label}
              tabIndex={filterable ? -1 : 0}
              aria-activedescendant={filterable ? undefined : activeId}
              onKeyDown={(e) => onListKey(e, false)}
              className="min-h-0 overflow-y-auto outline-none"
            >
              {shown.map((o, i) => {
                const on = o.value === value;
                return (
                  <div
                    key={o.value}
                    id={optionId(i)}
                    role="option"
                    aria-selected={on}
                    data-active={i === current || undefined}
                    onPointerMove={() => setActive(i)}
                    onClick={() => choose(o)}
                    className={cn("flex min-h-11 cursor-pointer items-center gap-2 rounded-chip px-2 data-active:bg-active", on && "font-medium", o.count === 0 && "text-muted")}
                  >
                    <Check aria-hidden="true" className={cn("size-4 shrink-0 text-jade-700", !on && "invisible")} />
                    <span className="min-w-0 flex-1 truncate">{o.label}</span> <span className="shrink-0 font-normal text-muted tabular-nums">{o.count}</span>
                  </div>
                );
              })}
              {shown.length === 0 && <p className="m-0 px-2 py-3 text-muted">No {label.toLowerCase()} matches “{query.trim()}”.</p>}
            </div>
          </Popover.Content>
        </Popover.Portal>
      </Popover.Root>
      {selected && (
        <Link
          href={clearHref}
          scroll={false}
          aria-label={`Remove ${label} filter`}
          className={cn(chip, chipSet, "w-11 justify-center rounded-s-none px-0 hover:bg-active")}
        >
          <X aria-hidden="true" className="size-4" />
        </Link>
      )}
    </div>
  );
}
