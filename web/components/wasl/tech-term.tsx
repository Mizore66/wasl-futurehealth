import { plain } from "@/lib/domain/terms";
import { cn } from "@/lib/utils";

/**
 * A backend identifier in plain words, with the raw identifier one click away (issue #5).
 * Native <details>, so it works without JavaScript and keyboard users get it for free.
 */
export function TechTerm({ id, label, className }: { id: string; label?: string; className?: string }) {
  return (
    <details className={cn("group inline-block align-baseline", className)}>
      <summary className="inline-flex min-h-6 cursor-pointer list-none items-center gap-1 rounded-chip [&::-webkit-details-marker]:hidden">
        <span>{label ?? plain(id)}</span>
        <span aria-hidden className="text-small text-muted group-open:rotate-90 motion-safe:transition-transform">
          ›
        </span>
        <span className="sr-only">, show the system identifier</span>
      </summary>
      <code className="mt-1 block font-mono text-small text-muted">{id}</code>
    </details>
  );
}
