import Link from "next/link";

export default function NotFound() {
  return (
    <section className="max-w-2xl" aria-labelledby="person-not-found">
      <h1 id="person-not-found" className="m-0 text-title font-semibold">Person journey not found</h1>
      <p>The opaque person ID is unknown, or the trusted identity projection does not contain it. No similar-name records were substituted.</p>
      <Link className="inline-flex min-h-11 items-center rounded-control px-2 underline" href="/app">Return to the worklist</Link>
    </section>
  );
}
