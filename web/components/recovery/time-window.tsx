import { cx } from "@/lib/cx";
import { windowGeometry, type Align } from "./candidates";

const anchor = (a: Align) => (a === "start" ? "text-left" : a === "end" ? "-translate-x-full text-right" : "-translate-x-1/2 text-center");
const dotX = (a: Align) => (a === "start" ? "before:left-0" : a === "end" ? "before:left-full" : "before:left-1/2");

const dot =
  "before:absolute before:size-2.5 before:-translate-x-1/2 before:-translate-y-1/2 before:rounded-full before:border-2 before:border-jade-600 before:bg-jade-50 before:content-['']";
const label = "block text-[13px] font-[550] leading-[1.3] text-muted";
const day = "block whitespace-nowrap text-[clamp(18px,2.2cqi,30px)] font-[650] leading-[1.2]";

/**
 * The scheduling window as one axis: the requested day and the due day at the ends, the slot above the axis and the
 * gap from slot to due below it. Values come from the slot and the candidate it is measured against.
 */
export function TimeWindow({
  requested,
  slotDay,
  due,
  assigned,
  stale,
}: {
  requested: number | null;
  slotDay: number;
  due: number | null;
  assigned: boolean;
  stale: boolean;
}) {
  const slotWord = assigned ? "Assigned" : stale ? "Slot unavailable" : "Slot";
  if (requested === null || due === null) {
    return (
      <div className="relative mx-[7px] h-[126px] @min-[701px]:h-[160px]" data-testid="recovery-window">
        <div aria-hidden className="absolute inset-x-0 top-[56px] h-0.5 bg-jade-300 @min-[701px]:top-[70px]" />
        <ol aria-label="Scheduling window" className="m-0 list-none p-0">
          <li data-mark="slot" className="absolute bottom-[calc(100%-56px+15px)] left-1/2 -translate-x-1/2 text-center text-jade-800 @min-[701px]:bottom-[calc(100%-70px+15px)]">
            <span className={label}>{slotWord}</span>
            <strong className={cx(day, "text-[clamp(19px,3.35cqi,45px)]")}>Day {Math.floor(slotDay)}</strong>
          </li>
        </ol>
        <p className="absolute inset-x-0 top-[72px] m-0 text-center text-small text-muted @min-[701px]:top-[86px]">No eligible patient for this slot.</p>
      </div>
    );
  }
  const g = windowGeometry(Math.floor(requested), Math.floor(slotDay), Math.floor(due));
  const past = g.gap < 0;
  return (
    <div
      className="relative mx-[7px] h-[126px] [--axis:56px] @min-[701px]:h-[160px] @min-[701px]:[--axis:70px]"
      style={{ "--slot": `${g.slot}%` } as React.CSSProperties}
      data-testid="recovery-window"
      data-assigned={assigned}
    >
      <div
        aria-hidden
        className="absolute inset-x-0 top-(--axis) h-0.5 bg-[linear-gradient(to_right,var(--color-jade-300)_var(--slot),var(--color-jade-800)_var(--slot))]"
      />
      <ol aria-label="Scheduling window" className="m-0 list-none p-0">
        <li data-mark="requested" className={cx("absolute top-[calc(var(--axis)+1px)] left-0 pt-[22px] @min-[701px]:pt-[29px]", dot, "before:top-0 before:left-0")}>
          <span className={label}>Requested</span>
          <strong className={day}>Day {Math.floor(requested)}</strong>
        </li>
        <li
          data-mark="slot"
          aria-current={assigned ? "true" : undefined}
          className={cx(
            "absolute bottom-[calc(100%-var(--axis)+15px)] text-jade-800",
            anchor(g.slotAlign),
            "after:absolute after:top-[calc(100%+15px)] after:size-3.5 after:-translate-x-1/2 after:-translate-y-1/2 after:rounded-full after:bg-jade-700 after:shadow-[0_0_0_4px_var(--color-jade-100)] after:content-['']",
            g.slotAlign === "start" ? "after:left-0" : g.slotAlign === "end" ? "after:left-full" : "after:left-1/2",
          )}
          style={{ left: `${g.slot}%` }}
        >
          <span className={label}>{slotWord}</span>
          <strong className={cx(day, "text-[clamp(19px,3.35cqi,45px)]")}>Day {Math.floor(slotDay)}</strong>
        </li>
        <li
          data-mark="due"
          className={cx("absolute top-[calc(var(--axis)+1px)] pt-[22px] @min-[701px]:pt-[29px]", anchor(g.endAlign), dot, dotX(g.endAlign), "before:top-0")}
          style={{ left: `${g.end}%` }}
        >
          <span className={label}>Due</span>
          <strong className={day}>Day {Math.floor(due)}</strong>
        </li>
      </ol>
      {/* Centred on the slot-to-due segment; a narrow segment lets the words spill evenly into the gutter. */}
      <div
        className="absolute top-[calc(var(--axis)+6px)] flex justify-center @min-[701px]:top-[calc(var(--axis)+9px)]"
        style={{ left: `${Math.min(g.slot, g.end)}%`, right: `${100 - Math.max(g.slot, g.end)}%` }}
      >
        <p
          data-testid="recovery-gap"
          className={cx("m-0 whitespace-nowrap text-[11px] font-[650] leading-[1.4] @min-[701px]:text-[12px]", past ? "text-clay-ink" : "text-jade-800")}
        >
          {g.gapText}
          <span className="sr-only">{past ? " (slot after the due day)" : " from slot to due day"}</span>
        </p>
      </div>
    </div>
  );
}
