import Link from "next/link";
import { PageHeader } from "@/components/shell/page-header";

export default function NotFound() {
  return (
    <section aria-labelledby="person-not-found" className="rounded-card border border-line bg-surface">
      <PageHeader attached title={<span id="person-not-found">Person journey not found</span>} />
      <div className="max-w-[72ch] p-4 md:p-pad">
        <p className="m-0">This person could not be found. No records with a similar name were shown instead.</p>
        <Link className="mt-3 inline-flex min-h-11 items-center rounded-control px-2 underline" href="/app">Return to the worklist</Link>
      </div>
    </section>
  );
}
