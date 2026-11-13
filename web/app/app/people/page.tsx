import { notFound, redirect } from "next/navigation";
import { getPersonByCase } from "@/lib/api/person-source";

export default async function Page({ searchParams }: PageProps<"/app/people">) {
  const caseID = (await searchParams).case_id;
  if (caseID === undefined) {
    return (
      <section className="max-w-2xl" aria-labelledby="person-lookup">
        <h1 id="person-lookup" className="m-0 text-title font-semibold">Person journey</h1>
        <p>Open a case and choose “View all programmes”. Wasl resolves the opaque person ID from trusted canonical identity links.</p>
        <p className="text-muted">There is no person-list endpoint and names are never used to find or combine records.</p>
      </section>
    );
  }
  if (typeof caseID !== "string" || !caseID) notFound();
  const journey = await getPersonByCase(caseID);
  if (!journey) notFound();
  redirect(`/app/people/${journey.person.id}`);
}
