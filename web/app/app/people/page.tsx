import { notFound, redirect } from "next/navigation";
import { getPersonByCase } from "@/lib/api/person-source";
import { PersonAssociationUnavailableView } from "@/components/person/journey";
import { PageHeader } from "@/components/shell/page-header";

export default async function Page({ searchParams }: PageProps<"/app/people">) {
  const caseID = (await searchParams).case_id;
  if (caseID === undefined) {
    return (
      <section aria-labelledby="person-lookup" className="rounded-card border border-line bg-surface">
        <PageHeader attached title={<span id="person-lookup">Person journey</span>} />
        <div className="max-w-[72ch] p-4 md:p-pad">
          <p className="m-0">Open a case and choose “View all programmes”. Wasl resolves the opaque person ID from trusted canonical identity links.</p>
          <p className="mb-0 text-muted">There is no person-list endpoint and names are never used to find or combine records.</p>
        </div>
      </section>
    );
  }
  if (typeof caseID !== "string" || !caseID) notFound();
  const journey = await getPersonByCase(caseID);
  if (!journey) notFound();
  if (journey.schema === "wasl-person-association-unavailable/2") return <PersonAssociationUnavailableView result={journey} />;
  redirect(`/app/people/${journey.person.id}`);
}
