import "server-only";
import { getReviewCase } from "@/lib/api/reviewer-session";
import { apiMode, getAudit, getUndoProjection } from "@/lib/api/source";
import type { Audit, UndoProjection } from "@/lib/api/schema";
import { concealsInitialRead, protocolRead } from "@/lib/domain/stage";

/*
 * The audit history and undo projection are read with shared credentials, so they must never carry an independent
 * review's initial read to a viewer who may not see it. Protocol events are shown only to a signed-in reviewer, and only
 * while no initial read is concealed. The viewer's state is read again after the audit fetch, so a first read that lands
 * while the fetch is in flight is still withheld. Withheld data never leaves this function, so it cannot enter RSC props.
 * Legacy history (no protocol events) is unchanged.
 */
export type HistoryStatus = "ok" | "signin" | "concealed";
export type GatedHistory = { status: HistoryStatus; audit: Audit | null; projection: UndoProjection | null };

type Viewer = Awaited<ReturnType<typeof getReviewCase>>;
const hidden = (v: Viewer) => v.authenticated && concealsInitialRead(v.detail.api_review_states ?? []);

export async function gatedHistory(id: string, opts: { before?: Viewer; needAudit?: boolean } = {}): Promise<GatedHistory> {
  if (apiMode() !== "live") return { status: "ok", audit: await getAudit(id), projection: null };
  const before = opts.before ?? (await getReviewCase(id));
  // The review page has nothing to show for history while concealed: do not even request it.
  if (hidden(before) && !opts.needAudit) return { status: "concealed", audit: null, projection: null };

  const [audit, projection] = await Promise.all([getAudit(id), getUndoProjection(id)]);
  const after = await getReviewCase(id);
  if (!audit) return { status: "ok", audit: null, projection: null };

  const hasProtocol = audit.history.some((e) => protocolRead(e) !== undefined);
  const concealed = hidden(before) || hidden(after);
  const status: HistoryStatus = concealed ? "concealed" : hasProtocol && !after.authenticated ? "signin" : "ok";
  if (status === "ok") return { status, audit, projection };
  return { status, audit: { ...audit, history: [] }, projection: null };
}
