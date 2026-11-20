import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { LAST_CASE_COOKIE } from "@/components/shell/remember-case";
import { getCases } from "@/lib/api/source";
import { caseHref } from "@/lib/domain/ids";

export const metadata = { title: "Audit trail" };

/** A case id as the app writes them (provider/Type/id), so a tampered cookie can't redirect anywhere else. */
const CASE_ID = /^[A-Za-z0-9._-]+\/[A-Za-z]+\/[A-Za-z0-9._#:-]+$/;

/**
 * The sidebar's Audit trail: the audit of the case last opened in this browser, or, before any case has been opened,
 * of the first open case. A stable URL, so the link is prefetched like every other destination.
 */
export default async function Page() {
  const remembered = decodeURIComponent((await cookies()).get(LAST_CASE_COOKIE)?.value ?? "");
  let id = CASE_ID.test(remembered) ? remembered : "";
  if (!id) {
    const { cases } = await getCases({ closed: false, limit: 1 });
    id = cases[0]?.id ?? "";
  }
  redirect(id ? `${caseHref("/app/patients", id)}/audit` : "/app/patients");
}
