import type { Metadata } from "next";
import Link from "next/link";
import { Mark } from "@/components/brand/mark";
import { SkipLink } from "@/components/shell/skip-link";
import { HonestyLabel } from "@/components/wasl/honesty-label";

export const metadata: Metadata = { title: "Page not found" };

const link =
  "inline-flex min-h-11 items-center rounded-control px-4 text-body font-medium no-underline";

/** Unknown URLs outside /app. Next keeps the HTTP 404 status. */
export default function NotFound() {
  return (
    <>
      <SkipLink />
      <main id="main" tabIndex={-1} className="mx-auto flex min-h-dvh max-w-xl flex-col justify-center px-6 py-12 focus:outline-none">
        <Link href="/" className="inline-flex min-h-11 w-fit items-center gap-2.5 rounded-control text-ink no-underline hover:text-ink">
          <Mark size={28} />
          <span className="text-section font-semibold">Wasl</span>
        </Link>
        <p className="m-0 mt-8 text-small text-muted">404</p>
        <h1 className="m-0 mt-1 text-title font-semibold">This page could not be found</h1>
        <p className="m-0 mt-3 text-body text-ink-soft">
          Wasl has no page at this address. Go back to the site, or open the dashboard.
        </p>
        <div className="mt-6 flex flex-wrap gap-3">
          <Link href="/" className={`${link} bg-cta text-ground hover:text-ground`}>
            Home
          </Link>
          <Link href="/app" className={`${link} border border-control text-ink hover:bg-active hover:text-ink`}>
            Dashboard
          </Link>
        </div>
        <div className="mt-6">
          <HonestyLabel kind="built">Built · Synthetic data demo</HonestyLabel>
        </div>
      </main>
    </>
  );
}
