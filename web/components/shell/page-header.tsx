import { cx } from "@/lib/cx";

/**
 * The staff title header: limestone-to-jade wash with the calligraphy pen strokes (globals.css .staff-heading).
 * One h1 per page. `aside` sits at the end of the band (a status chip, a back link, record icons).
 */
export function PageHeader({
  title,
  sub,
  aside,
  titleAside,
  className,
  compact = false,
}: {
  title: React.ReactNode;
  sub?: React.ReactNode;
  aside?: React.ReactNode;
  /** Inline with the title, e.g. the full-record icon. */
  titleAside?: React.ReactNode;
  className?: string;
  compact?: boolean;
}) {
  return (
    <header className={cx("staff-heading mb-3 flex flex-wrap items-center justify-between gap-3", className)}>
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
      {aside && <div className="flex flex-none flex-wrap items-center gap-2">{aside}</div>}
    </header>
  );
}
