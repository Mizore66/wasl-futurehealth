import type { Metadata } from "next";
import Link from "next/link";
import { PageHeader } from "@/components/shell/page-header";
import { HonestyLabel } from "@/components/wasl/honesty-label";

export const metadata: Metadata = { title: "Page not found" };

const link =
  "inline-flex min-h-11 items-center rounded-control px-4 text-body font-medium no-underline";

/** Rendered inside the app shell's `<main id="main">`, so it adds no landmark of its own. */
export default function AppNotFound() {
  return (
    <section aria-labelledby="nf-title" className="rounded-card border border-line bg-surface">
      <PageHeader attached sub="404" title={<span id="nf-title">This page could not be found</span>} />
      <div className="max-w-xl p-4 md:p-pad">
        <p className="m-0 text-body text-ink-soft">
          There is no page at this address. Go back to the dashboard, or pick a section from the menu.
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
          <HonestyLabel kind="built" />
        </div>
      </div>
    </section>
  );
}
