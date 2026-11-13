import { notFound } from "next/navigation";
import { Journey } from "@/components/person/journey";
import { getPerson } from "@/lib/api/person-source";

export const metadata = { title: "Person journey" };

export default async function Page({ params }: PageProps<"/app/people/[id]">) {
  const personID = (await params).id;
  if (!/^person_[0-9a-f]{64}$/.test(personID)) notFound();
  const journey = await getPerson(personID);
  if (!journey) notFound();
  return <Journey journey={journey} />;
}
