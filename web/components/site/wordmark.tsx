import Link from "next/link";
import { Logo } from "@/components/brand/mark";
import { cn } from "@/lib/utils";

/** The Wasl logo as the site's home link. */
export function Wordmark({ size = "md", className }: { size?: "sm" | "md"; className?: string }) {
  return (
    <Link href="/" aria-label="Wasl, home" className={cn("flex items-center no-underline", className)}>
      <Logo className={size === "sm" ? "text-[28px]" : "text-[34px]"} />
    </Link>
  );
}
