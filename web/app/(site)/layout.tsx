import { Hanken_Grotesk } from "next/font/google";
import { SiteHeader } from "@/components/site/header";

const hanken = Hanken_Grotesk({ variable: "--font-hanken", subsets: ["latin"], weight: ["400", "500", "600", "700"] });

/** The site's own palette (the C boards): warm paper, sand fields, one teal for the thread and the lead figures. */
const PALETTE = {
  "--paper": "#f6f3ee",
  "--sand": "#efe8dc",
  "--sand-deep": "#e6dccb",
  "--mint": "#dbe8e6",
  "--card": "#fffdf8",
  "--ink": "#0B2427",
  "--teal": "#0f4f4c",
  "--muted": "#4a5859",
  "--stub": "#6b7a79",
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
