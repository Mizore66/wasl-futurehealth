import Link from "next/link";
import { getFixtures } from "@/lib/api/source";
import { caseHref, shortId } from "@/lib/domain/ids";
import { plain } from "@/lib/domain/terms";
import { personForLoop } from "@/lib/people";

/**
 * The page a wrong case id lands on: says what happened and lists the demo cases (issue #1).
 * It renders inside the segment's not-found.tsx, so the status stays 404.
 */
export async function CaseNotFound({ base }: { base: "/app/patients" | "/app/review" }) {
  const { fixtures } = await getFixtures();
  const cases = Object.values(fixtures).filter((c): c is NonNullable<typeof c> => !!c);
  return (
    <>
      <h1 className="m-0 text-title font-semibold">We could not find that case</h1>
      <p className="m-0 mt-2 max-w-[72ch] text-muted">
        Case ids look like <code className="font-mono">ahs-mushrif/ServiceRequest/AHS-sr-1079</code>, not a plain number. This run has no case with the id in
        the address. Try one of the demo cases:
      </p>
      <ul className="m-0 mt-4 flex list-none flex-col gap-2 p-0" aria-label="Demo cases">
        {cases.map((c) => {
          const person = personForLoop(c.id);
          return (
            <li key={c.id}>
              <Link
                href={caseHref(base, c.id)}
                className="flex min-h-14 flex-col justify-center rounded-card border border-line bg-surface px-4 py-2 text-ink no-underline hover:bg-active hover:text-ink"
              >
                <span className="font-medium">{person?.display_name ?? shortId(c.id)}</span>
                <span className="text-muted">
                  {plain(c.fixture)} · <span className="font-mono text-small">{shortId(c.id)}</span>
                </span>
              </Link>
            </li>
          );
        })}
      </ul>
      <p className="m-0 mt-4">
        <Link href={base === "/app/patients" ? "/app/patients" : "/app/review"} className="inline-flex min-h-11 items-center">
          {base === "/app/patients" ? "Back to all patients" : "Back to Review"}
        </Link>
      </p>
    </>
  );
}
