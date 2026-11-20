"use client";

import Link from "next/link";
import { MessagesSquare } from "lucide-react";
import { Pill } from "@/components/review/marks";
import {
  consultStatus,
  consultsForToday,
  specialty,
  statusWords,
} from "@/components/case/consult-model";
import { useConsults } from "@/components/case/consult-store";
import { formatDay } from "@/lib/domain/day";
import { caseHref } from "@/lib/domain/ids";

/** Today's specialist consults: replies that are overdue or have come back unread. Nothing shows when there are none. */
export function ConsultsDue({ now }: { now: number }) {
  const due = consultsForToday(useConsults(), now);
  if (due.length === 0) return null;
  return (
    <section
      aria-labelledby="consults-due-h"
      className="border-b border-line px-4 py-3"
      data-testid="consults-due"
    >
      <h2 id="consults-due-h" className="m-0 text-section font-medium">
        Specialist consults
      </h2>
      <ul className="m-0 mt-2 list-none p-0">
        {due.map((c) => {
          const status = consultStatus(c, now);
          return (
            <li
              key={c.id}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-rule py-2 first:border-t-0"
            >
              <MessagesSquare aria-hidden className="size-4 text-muted" />
              <Pill tone={status === "overdue" ? "warning" : "positive"}>
                {statusWords[status]}
              </Pill>
              <Link
                href={`${caseHref("/app/patients", c.caseId)}#timeline`}
                className="font-medium"
              >
                {c.caseLabel}
              </Link>
              <span className="text-small text-muted">
                {specialty(c.specialty).label} · {c.owner} ·{" "}
                {status === "overdue"
                  ? `due ${formatDay(c.replyBy)}`
                  : "reply to read"}
              </span>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
