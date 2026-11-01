import readerJson from "@/data/reader.json";
import { PipelineHeading } from "@/components/pipeline/heading";
import { ReadingReplay, type StandsFor } from "@/components/pipeline/reading-replay";
import type { ReaderData } from "@/components/pipeline/reader-types";
import { personForLoop } from "@/lib/people";
import { referralSeed } from "@/lib/people/identity";

export const metadata = { title: "Report reading" };

/** A report stands for a cast member only where that fixture's recorded evidence quotes the same text. */
function castLines(data: ReaderData): Record<string, StandsFor> {
  const out: Record<string, StandsFor> = {};
  for (const c of data.cases) {
    const person = c.standsFor && personForLoop(c.standsFor.loop);
    if (person) out[c.uid] = { line: `${person.display_name}'s report ${c.standsFor!.report}`, seed: referralSeed(c.standsFor!.loop) };
  }
  return out;
}

export default function Page() {
  return (
    <ReadingReplay
      heading={
        <PipelineHeading
          title="Report reading"
          question="Does this report prove the colonoscopy is complete?"
          current="/app/pipeline/reading"
        />
      }
      standsFor={castLines(readerJson as ReaderData)}
    />
  );
}
