"use client";

import { motion, useReducedMotionConfig } from "motion/react";
import { useId, useState } from "react";

/** Direct feedback: the note opens and closes in 160ms, or at once under reduced motion (root MotionConfig). */
const DISCLOSE = { duration: 0.16, ease: [0.2, 0.7, 0.2, 1] } as const;

/**
 * A card heading with a "Source" disclosure; the note opens under the heading. The note stays in the
 * DOM so aria-controls always resolves; while closed or closing it is inert (out of the tab order and
 * the accessibility tree) and ends hidden.
 */
export function SourceHeading({
  id,
  title,
  sub,
  display = false,
  children,
}: {
  id: string;
  title: string;
  /** A short line under the heading. */
  sub?: React.ReactNode;
  /** The staff display face, for a card's lead heading. */
  display?: boolean;
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const noteId = useId();
  const reduce = useReducedMotionConfig();
  return (
    <>
      <div className="flex items-baseline justify-between gap-3">
        <div className="min-w-0">
          <h2
            id={id}
            className={display ? "staff-display m-0 text-[clamp(23px,2.2vw,36px)] leading-[1.1] text-jade-900" : "m-0 text-section font-medium"}
          >
            {title}
          </h2>
          {sub && <p className="m-0 mt-1 text-small text-muted">{sub}</p>}
        </div>
        {/* Touch widths get a 44px hit area of transparent padding; the negative margin keeps the row's compact height.
            The fixed minimum width fits "Hide source", so opening the note never re-wraps the heading's takeaway line. */}
        <button
          type="button"
          aria-expanded={open}
          aria-controls={noteId}
          onClick={() => setOpen(!open)}
          className="-my-[13px] inline-flex min-h-11 min-w-[5.5rem] flex-none md:my-0 md:min-h-0 cursor-pointer items-center justify-end border-0 bg-transparent p-0 text-small text-hue hover:text-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-hue"
        >
          {open ? "Hide source" : "Source"}
        </button>
      </div>
      <motion.div
        id={noteId}
        inert={!open}
        initial={false}
        animate={open ? { height: "auto", opacity: 1, visibility: "visible" } : { height: 0, opacity: 0, transitionEnd: { visibility: "hidden" } }}
        transition={reduce ? { duration: 0 } : DISCLOSE}
        className="overflow-hidden"
      >
        <div className="pt-3">
          <div className="rounded-control bg-raised px-4 py-3 text-small leading-[18px] text-ink-soft">{children}</div>
        </div>
      </motion.div>
    </>
  );
}
