import { cn } from "@/lib/utils";
import { cta } from "./styles";

/** The request action. No address exists yet, so it shows the placeholder and stays on the page instead of opening a mail client. */
export function ContactCta({ label = "Request a pilot", className }: { label?: string; className?: string }) {
  return (
    <a href="#pilot" className={cn(cta, className)}>
      {label}: [team email]
    </a>
  );
}
