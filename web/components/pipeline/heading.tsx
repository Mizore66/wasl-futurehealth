import Link from "next/link";
import { PageHeader } from "@/components/shell/page-header";
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

export type PipelineHead = {
  title: string;
  question?: string;
  current: (typeof VIEWS)[number]["href"];
  /** An extra honesty line, e.g. SNAPSHOT_NOTE. */
  note?: string;
};

/**
 * The pipeline screens' main card: the page header as its top band, the tab bar between the three views, the one
 * honesty line and the replay `controls`, then the screen's lead section. The card does not clip, so the run notice's
 * drop-down can hang past a short body.
 */
export function PipelineHeading({ title, question, current, note, controls, children }: PipelineHead & { controls?: React.ReactNode; children?: React.ReactNode }) {
  return (
    <div className="rounded-card border border-line bg-surface">
      <PageHeader attached title={title} sub={question} />
      <nav aria-label="Pipeline views" className="flex flex-wrap gap-x-6 border-b border-rule px-4 sm:px-6">
        {VIEWS.map((v) => {
          const on = v.href === current;
          return (
            <Link
              key={v.href}
              href={v.href}
              aria-current={on ? "page" : undefined}
              className={cn(
                "-mb-px inline-flex min-h-11 items-center border-b-2 py-2.5 text-body no-underline",
                on ? "border-jade-700 font-semibold text-jade-900 hover:text-jade-900" : "border-transparent font-medium text-muted hover:text-jade-800",
              )}
            >
              {v.label}
            </Link>
          );
        })}
      </nav>
      <div className="flex flex-col gap-4 px-4 py-4 sm:px-6">
        <div className="flex flex-col gap-1">
          <p className="m-0 flex flex-wrap items-center gap-2 text-small text-ink-soft">
            <HonestyLabel kind="replay" />
            <span>{REPLAY_LINE}</span>
          </p>
          {note && (
            <p className="m-0 max-w-[72ch] text-small text-ink-soft" data-testid="pipeline-snapshot-note">
              {note}
            </p>
          )}
        </div>
        {controls}
      </div>
      {children}
    </div>
  );
}
