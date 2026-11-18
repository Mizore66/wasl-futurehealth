import Link from "next/link";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cn } from "@/lib/utils";

const VIEWS = [
  { href: "/app/pipeline", label: "Case flow" },
  { href: "/app/pipeline/matching", label: "Identity matching" },
  { href: "/app/pipeline/reading", label: "Report reading" },
] as const;

export const REPLAY_LINE = "Replaying the recorded showcase run · synthetic patients · names and faces are generated · real report texts";

/** Shown when a live run carries no showcase fixtures and the screen replays the published snapshot instead. */
export const SNAPSHOT_NOTE = "This live run has no showcase cases, so this view replays the published fixture snapshot, not the live ledger.";

/** The pipeline screens' title block: heading, the one honesty line, and links between the three views. */
export function PipelineHeading({
  title,
  question,
  current,
  note,
}: {
  title: string;
  question?: string;
  current: (typeof VIEWS)[number]["href"];
  /** An extra honesty line, e.g. SNAPSHOT_NOTE. */
  note?: string;
}) {
  return (
    <div className="flex min-w-0 flex-[1_1_480px] flex-col gap-1">
      <nav aria-label="Pipeline views" className="flex gap-4 text-small">
        {VIEWS.map((v) => (
          <Link
            key={v.href}
            href={v.href}
            aria-current={v.href === current ? "page" : undefined}
            className={cn(v.href === current ? "font-medium text-ink no-underline" : "text-muted no-underline hover:text-ink")}
          >
            {v.label}
          </Link>
        ))}
      </nav>
      <h1 className="m-0 text-title font-semibold">{title}</h1>
      {question && <p className="m-0 text-body">{question}</p>}
      <p className="m-0 flex flex-wrap items-center gap-2 text-small text-muted">
        <HonestyLabel kind="replay" />
        <span>{REPLAY_LINE}</span>
      </p>
      {note && (
        <p className="m-0 text-small text-muted" data-testid="pipeline-snapshot-note">
          {note}
        </p>
      )}
    </div>
  );
}
