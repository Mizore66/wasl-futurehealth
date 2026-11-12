"use client";

import { cn } from "@/lib/utils";

/** Native radios in a fieldset: arrow keys, Space and the group name come from the browser. */
export function Chips<T extends string>({
  name,
  options,
  value,
  onPick,
  disabled,
}: {
  name: string;
  options: [T, string][];
  value: T | undefined;
  onPick: (v: T) => void;
  disabled: boolean;
}) {
  return (
    <div className="mt-2 flex flex-wrap gap-2">
      {options.map(([v, label]) => (
        <label
          key={v}
          className={cn(
            "relative flex min-h-11 min-w-[7rem] flex-1 cursor-pointer items-center justify-center rounded-chip px-3 py-1 text-center text-ink has-[:focus-visible]:outline-2 has-[:focus-visible]:outline-offset-2 has-[:focus-visible]:outline-hue",
            value === v ? "border-2 border-ink bg-active font-medium" : "border border-control bg-surface hover:bg-active",
          )}
        >
          <input
            type="radio"
            name={name}
            value={v}
            checked={value === v}
            disabled={disabled}
            onChange={() => onPick(v)}
            className="absolute -inset-0.5 m-0 size-[calc(100%+4px)] cursor-pointer opacity-0"
          />
          {label}
        </label>
      ))}
    </div>
  );
}
