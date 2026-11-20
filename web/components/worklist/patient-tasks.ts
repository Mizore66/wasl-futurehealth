import type { CaseSummary, PatientTask } from "@/lib/api/schema";
import { staffDueTime } from "@/lib/domain/staff-time";

/**
 * Staff tasks opened by verified patient requests (pipeline/patient_task.go): a failed teach-back callback or a
 * patient contest. The API sorts open tasks first, earliest due first. Older APIs without `patient_tasks` still
 * expose an open teach-back callback through `teach_back_callback`.
 */
export function openPatientTasks(c: CaseSummary): PatientTask[] {
  if (c.patient_tasks) return c.patient_tasks.filter((t) => t.status === "open");
  const cb = c.teach_back_callback;
  return cb && cb.status === "open" ? [{ ...cb, kind: "teach_back_callback" }] : [];
}

/** True while any patient-opened task is past its due time. */
export function patientTaskOverdue(c: CaseSummary): boolean {
  return openPatientTasks(c).some((t) => t.overdue === true);
}

/**
 * An open patient contest waits on a senior clinician who did not sign the case; closure is held meanwhile. The
 * coordinator cannot resolve it, so the row waits rather than asking for a call.
 */
export function patientContestWords(c: CaseSummary): { status: string; next: string } | null {
  const open = openPatientTasks(c);
  const contest = open.find((t) => t.kind === "patient_contest");
  const revision = open.find((t) => t.kind === "patient_contest_revision");
  if (!contest && revision) {
    const due = revision.due_at ? ` ${revision.overdue ? "overdue since" : "due"} ${staffDueTime(revision.due_at)}` : "";
    return {
      status: `Patient contest · decision to be revised${due}`,
      next: "A senior clinician replaces the questioned decision. Automatic closure stays held until then",
    };
  }
  if (!contest) return null;
  const due = contest.due_at ? ` ${contest.overdue ? "overdue since" : "due"} ${staffDueTime(contest.due_at)}` : "";
  return {
    status: `Patient contest · independent review${due}`,
    next: "A senior clinician who did not sign this case reviews the contest. Automatic closure stays held until then",
  };
}
