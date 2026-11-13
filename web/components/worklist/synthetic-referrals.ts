import { readFile } from "node:fs/promises";
import path from "node:path";
import { z } from "zod";
import { CaseSummary } from "@/lib/api/schema";
import { apiMode } from "@/lib/api/source";

/**
 * Fixtures mode only. The published CRC sample holds no referral the unit has not answered, so the inbox adds these
 * synthetic ones (web/data/referral-inbox-synthetic.json) to show its Accept, conditions and Return controls. They are
 * kept out of the API samples so the other pages' counts stay the run's. The live API never gets them.
 */
export async function syntheticReferrals(q: string): Promise<CaseSummary[]> {
  if (apiMode() === "live") return [];
  const file = path.join(process.cwd(), "data", "referral-inbox-synthetic.json");
  const cases = z.array(CaseSummary).parse(JSON.parse(await readFile(file, "utf8")));
  return cases.filter((c) => c.id.includes(q));
}
