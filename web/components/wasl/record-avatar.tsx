import Link from "next/link";
import { caseHref } from "@/lib/domain/ids";
import { cn } from "@/lib/utils";

/**
 * Wraps an avatar so it opens the patient's record; with no record to open it stays a plain picture. `className` styles the link only.
 * The link pads 8px each side (cancelled by a negative margin) so a 32px face is a 48px target without moving anything.
 */
export function RecordAvatar({
  id,
  name,
  linked,
  className,
  children,
}: {
  id: string;
  name: string;
  linked: boolean;
  className?: string;
  children: React.ReactNode;
}) {
  if (!linked) return <span className="flex flex-none">{children}</span>;
  return (
    <Link href={caseHref("/app/patients", id)} aria-label={`Open ${name}'s record`} className={cn("-m-2 flex flex-none rounded-full p-2", className)}>
      {children}
    </Link>
  );
}
