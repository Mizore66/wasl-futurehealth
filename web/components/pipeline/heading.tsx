import Link from "next/link";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cn } from "@/lib/utils";

const VIEWS = [
  { href: "/app/pipeline", label: "Case flow" },
  { href: "/app/pipeline/matching", label: "Identity matching" },
  { href: "/app/pipeline/reading", label: "Report reading" },
] as const;

export const REPLAY_LINE = "Replaying the recorded showcase run · synthetic patients · real report texts";

/** The pipeline screens' title block: heading, the one honesty line, and links between the three views. */
export function PipelineHeading({
  title,
  question,
  current,
}: {
  title: string;
  question?: string;
  current: (typeof VIEWS)[number]["href"];
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
    </div>
  );
}
