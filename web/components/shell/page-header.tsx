import { cx } from "@/lib/cx";
import { RunMeta } from "./run-meta";

/**
 * The staff title header: limestone-to-jade wash with the calligraphy pen strokes (globals.css .staff-heading).
 * One h1 per page, and one header per page carries `run`, the simulated-data notice with its details drop-down.
 * `attached` makes it the top band of the page's main card: put it first inside that card. `aside` sits at the end of
 * the band (a status chip, a back link, record icons).
 */
export function PageHeader({
  title,
  sub,
  aside,
  titleAside,
  className,
  compact = false,
  attached = false,
  run = false,
}: {
  title: React.ReactNode;
  sub?: React.ReactNode;
  aside?: React.ReactNode;
  /** Inline with the title, e.g. the full-record icon. */
  titleAside?: React.ReactNode;
  className?: string;
  compact?: boolean;
  attached?: boolean;
  run?: boolean;
}) {
  return (
    <header
      className={cx(
        "staff-heading flex flex-wrap items-center justify-between gap-3",
        // 7px: the inner radius of a 1px-bordered 8px card, so the wash meets the card corner whether or not the card clips.
        attached ? "mb-0 rounded-t-[7px] rounded-b-none border-0 border-b border-line" : "mb-3",
        // The drop-downs hang below the band, over the card body: above its sticky bars (z-10), under the top bar (z-20).
        run && "z-[15] overflow-visible",
        className,
      )}
    >
      <div className="min-w-0 max-w-full sm:max-w-[70%]">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <h1
            className={cx(
              "staff-display m-0 break-words leading-[1.05] text-ink",
              compact ? "text-[clamp(28px,2.6vw,36px)]" : "text-[clamp(32px,3vw,44px)]",
            )}
          >
            {title}
          </h1>
          {titleAside}
        </div>
        {sub && <p className="m-0 mt-1 text-small text-limestone-800">{sub}</p>}
      </div>
      {(aside || run) && (
        // ml-auto keeps the group, and the drop-downs anchored to its right edge, at the right even when it wraps to its own line.
        <div className="relative ml-auto flex min-w-0 max-w-full flex-wrap items-center justify-end gap-2">
          {run && <RunMeta />}
          {aside}
        </div>
      )}
    </header>
  );
}
