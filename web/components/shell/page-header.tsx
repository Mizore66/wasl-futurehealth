import { cx } from "@/lib/cx";
import { GlossaryPopover } from "@/components/wasl/glossary-popover";
import type { GlossaryKey } from "@/lib/domain/glossary";

/**
 * The staff title header: limestone-to-jade wash with the faded وصل calligraphy (globals.css .staff-heading).
 * One h1 per page. The header holds the title and a one-line subtitle, and nothing sits over the artwork.
 * `help` is a quiet "Terms used here" link at
 * the end of the subtitle, and `aside` (a scope control, a back link) sits under the subtitle in the text column.
 * `attached` makes it the top band of the page's main card: put it first inside that card.
 */
export function PageHeader({
  title,
  sub,
  aside,
  titleAside,
  help,
  className,
  compact = false,
  attached = false,
}: {
  title: React.ReactNode;
  sub?: React.ReactNode;
  aside?: React.ReactNode;
  /** Inline with the title, e.g. the full-record icon. */
  titleAside?: React.ReactNode;
  /** Glossary terms behind a quiet "Terms used here" text link after the subtitle. */
  help?: readonly GlossaryKey[];
  className?: string;
  compact?: boolean;
  attached?: boolean;
}) {
  return (
    <header
      className={cx(
        "staff-heading",
        // 7px: the inner radius of a 1px-bordered 8px card, so the wash meets the card corner whether or not the card clips.
        attached ? "mb-0 rounded-t-[7px] rounded-b-none border-0 border-b border-line" : "mb-3",
        // A control's drop-down hangs below the band, over the card body: above its sticky bars (z-10), under the top bar (z-20).
        Boolean(aside) && "z-[15] overflow-visible",
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
        {(sub || help) && (
          <p className="m-0 mt-1 text-small text-limestone-800">
            {sub}
            {sub && help && " "}
            {help && <GlossaryPopover terms={help} inline />}
          </p>
        )}
        {aside && <div className="mt-2 flex min-w-0 max-w-full flex-wrap items-center gap-2">{aside}</div>}
      </div>
    </header>
  );
}
