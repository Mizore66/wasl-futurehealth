import { cn } from "@/lib/utils";

/**
 * The system's own identifiers behind a native disclosure (issue #5): plain words are the primary text on every pipeline screen,
 * and the raw ids stay one click away for the backend owner. Native <details>, so keyboard users and no-JS readers get it free.
 */
export function TechnicalDetail({ children, label = "Technical detail", className }: { children: React.ReactNode; label?: string; className?: string }) {
  return (
    <details data-testid="technical-detail" className={cn("group min-w-0", className)}>
      <summary className="inline-flex min-h-11 cursor-pointer list-none items-center gap-1.5 rounded-chip text-small text-muted hover:text-ink [&::-webkit-details-marker]:hidden">
        <span aria-hidden className="group-open:rotate-90 motion-safe:transition-transform">
          ›
        </span>
        {label}
      </summary>
      <div className="min-w-0 break-words font-mono text-small text-muted">{children}</div>
    </details>
  );
}
