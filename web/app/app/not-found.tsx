import type { Metadata } from "next";
import Link from "next/link";
import { HonestyLabel } from "@/components/wasl/honesty-label";

export const metadata: Metadata = { title: "Page not found" };

const link =
  "inline-flex min-h-11 items-center rounded-control px-4 text-body font-medium no-underline";

/** Rendered inside the app shell's `<main id="main">`, so it adds no landmark of its own. */
export default function AppNotFound() {
  return (
    <section aria-labelledby="nf-title" className="max-w-xl py-6">
      <p className="m-0 text-small text-muted">404</p>
      <h1 id="nf-title" className="m-0 mt-1 text-title font-semibold">
        This page could not be found
      </h1>
      <p className="m-0 mt-3 text-body text-ink-soft">
        The dashboard has no page at this address. Go back to the programme overview or pick a section from the sidebar.
      </p>
      <div className="mt-6 flex flex-wrap gap-3">
        <Link href="/app" className={`${link} bg-cta text-ground hover:text-ground`}>
          Dashboard
        </Link>
        <Link href="/" className={`${link} border border-control text-ink hover:bg-active hover:text-ink`}>
          Home
        </Link>
      </div>
      <div className="mt-6">
        <HonestyLabel kind="built">Built · the app shell</HonestyLabel>
      </div>
    </section>
  );
}
