import { plain } from "@/lib/domain/terms";
import { cn } from "@/lib/utils";

/**
 * A backend identifier in plain words (issue #5). The raw identifier is not shown to staff.
 */
export function TechTerm({ id, label, className }: { id: string; label?: string; className?: string }) {
  return <span className={cn("align-baseline", className)}>{label ?? plain(id)}</span>;
}
