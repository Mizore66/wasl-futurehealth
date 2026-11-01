import { Hanken_Grotesk } from "next/font/google";
import { SiteHeader } from "@/components/site/header";

const hanken = Hanken_Grotesk({ variable: "--font-hanken", subsets: ["latin"], weight: ["400", "500", "600", "700"] });

/** The site's names for the shared sand-and-thread tokens in globals.css. */
const PALETTE = {
  "--paper": "var(--color-ground)",
  "--sand": "var(--color-raised)",
  "--sand-deep": "var(--color-active)",
  "--mint": "var(--color-mint)",
  "--card": "var(--color-surface)",
  "--ink": "var(--color-ink)",
  "--ink-soft": "var(--color-ink-soft)",
  "--line": "var(--color-control)",
  "--held": "var(--color-pill-warning-ink)",
  "--held-bg": "var(--color-pill-warning-bg)",
  "--negative": "var(--color-negative)",
  "--teal": "var(--color-hue)",
  "--muted": "var(--color-muted)",
  "--stub": "var(--color-unknown)",
  fontFamily: "var(--font-hanken), system-ui, sans-serif",
} as React.CSSProperties;

export default function SiteLayout({ children }: { children: React.ReactNode }) {
  return (
    <div
      className={`${hanken.variable} min-h-dvh bg-(--paper) text-[18px] leading-[1.55] text-(--ink) [&_:focus-visible]:outline-3 [&_:focus-visible]:outline-offset-3 [&_:focus-visible]:outline-(--teal)`}
      style={PALETTE}
    >
      <SiteHeader />
      <main>{children}</main>
    </div>
  );
}
