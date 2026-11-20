"use client";

import { useId } from "react";
import { Popover } from "radix-ui";
import { cx } from "@/lib/cx";
import { ALL_GLOSSARY_KEYS, GLOSSARY, type GlossaryKey } from "@/lib/domain/glossary";

/**
 * "Terms used here": a small, keyboard-operable glossary (N10). Radix Popover gives the button its expanded state,
 * moves focus into the panel on open, closes on Escape or an outside click, and returns focus to the button.
 */
export function GlossaryPopover({
  terms = ALL_GLOSSARY_KEYS,
  className,
  inline = false,
}: {
  terms?: readonly GlossaryKey[];
  className?: string;
  /** A quiet text link that runs on in a line of text (the page header's subtitle), not a button. */
  inline?: boolean;
}) {
  const headingId = useId();
  return (
    <Popover.Root>
      <Popover.Trigger asChild>
        {inline ? (
          <button
            type="button"
            // 44px tall for touch; the negative margin keeps the subtitle's line height as if it were plain text.
            className={cx(
              "-my-3.5 inline-flex min-h-11 cursor-pointer items-center border-0 bg-transparent p-0 align-middle font-[inherit] text-jade-800 underline decoration-dotted underline-offset-4 hover:text-ink",
              className,
            )}
            data-testid="glossary-trigger"
          >
            Terms used here
          </button>
        ) : (
          <button
            type="button"
            className={cx(
              "inline-flex min-h-11 items-center gap-1.5 rounded-control px-2 text-small text-muted underline decoration-dotted underline-offset-4 hover:bg-active hover:text-ink",
              className,
            )}
            data-testid="glossary-trigger"
          >
            <span aria-hidden className="flex size-5 items-center justify-center rounded-full border border-control text-[11px] font-medium leading-none">
              ?
            </span>
            Terms used here
          </button>
        )}
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          align={inline ? "start" : "end"}
          sideOffset={6}
          collisionPadding={12}
          aria-labelledby={headingId}
          className="z-50 w-[min(92vw,26rem)] rounded-card border border-line bg-surface p-4 text-body text-ink shadow-lg focus:outline-none"
          data-testid="glossary-panel"
        >
          <div className="flex items-start justify-between gap-3">
            <h2 id={headingId} className="m-0 text-section font-semibold">
              Terms used here
            </h2>
            <Popover.Close className="min-h-11 rounded-control px-2 text-small text-muted hover:bg-active hover:text-ink">Close</Popover.Close>
          </div>
          <dl className="m-0 mt-2 flex flex-col gap-3">
            {terms.map((key) => (
              <div key={key}>
                <dt className="font-medium">{GLOSSARY[key].term}</dt>
                <dd className="m-0 text-small text-muted">{GLOSSARY[key].meaning}</dd>
              </div>
            ))}
          </dl>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
