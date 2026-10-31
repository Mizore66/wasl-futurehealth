"use client";

import { useId, useState } from "react";

/** A card heading with a "Source" disclosure; the note opens under the heading. */
export function SourceHeading({ id, title, children }: { id: string; title: string; children: React.ReactNode }) {
  const [open, setOpen] = useState(false);
  const noteId = useId();
  return (
    <>
      <div className="flex items-baseline justify-between gap-3">
        <h2 id={id} className="m-0 text-section font-medium">
          {title}
        </h2>
        <button
          type="button"
          aria-expanded={open}
          aria-controls={noteId}
          onClick={() => setOpen(!open)}
          className="flex-none cursor-pointer border-0 bg-transparent p-0 text-small text-hue hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-hue"
        >
          {open ? "Hide source" : "Source"}
        </button>
      </div>
      <div id={noteId} hidden={!open} className="mt-3 rounded-control bg-raised px-4 py-3 text-small leading-[18px] text-ink-soft">
        {children}
      </div>
    </>
  );
}
