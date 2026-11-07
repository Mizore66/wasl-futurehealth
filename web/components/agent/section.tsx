import { cn } from "@/lib/utils";

/** The card every agent-page section sits in. */
export function AgentSection({
  id,
  title,
  note,
  aside,
  children,
  className,
}: {
  id: string;
  title: string;
  note?: React.ReactNode;
  aside?: React.ReactNode;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <section id={id} aria-labelledby={`${id}-h`} className={cn("min-w-0 scroll-mt-4 rounded-card border border-line bg-surface p-4 md:p-pad", className)}>
      <div className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h2 id={`${id}-h`} className="m-0 text-section font-medium">
          {title}
        </h2>
        {aside}
      </div>
      {note && <p className="m-0 mt-1 max-w-[72ch] text-small text-muted">{note}</p>}
      {children}
    </section>
  );
}
