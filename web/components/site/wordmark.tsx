import Link from "next/link";
import { cn } from "@/lib/utils";

/** "Wasl | وصل": the bilingual name, a connection and a receipt. */
export function Wordmark({ size = "md", className }: { size?: "sm" | "md"; className?: string }) {
  const sm = size === "sm";
  return (
    <Link href="/" aria-label="Wasl, home" className={cn("flex items-center gap-3.5 text-(--ink) no-underline hover:text-(--ink)", className)}>
      <span className={cn("font-bold leading-none tracking-[-0.02em]", sm ? "text-[24px]" : "text-[28px]")}>Wasl</span>
      <span aria-hidden className={cn("w-[3px] bg-(--teal)", sm ? "h-[22px]" : "h-[26px]")} />
      <span lang="ar" dir="rtl" className={cn("font-arabic font-semibold leading-none text-(--teal)", sm ? "text-[22px]" : "text-[26px]")}>
        وصل
      </span>
    </Link>
  );
}
