"use client";

import { useId, useState } from "react";
import { MessagesSquare } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Pill } from "@/components/review/marks";
import { formatDayYear } from "@/lib/domain/day";
import { cn } from "@/lib/utils";
import {
  answerConsult,
  consultReplyBy,
  consultStatus,
  newConsult,
  QUESTION_MAX,
  SPECIALTIES,
  specialty,
  statusWords,
  type Consult,
} from "./consult-model";
import { saveConsult, useConsults } from "./consult-store";

const BUTTON =
  "inline-flex min-h-11 items-center justify-center rounded-control px-4 font-medium no-underline";
const QUIET =
  "border border-control bg-surface text-ink hover:bg-active hover:text-ink";
const PRIMARY =
  "cursor-pointer bg-cta text-white hover:bg-ink hover:text-white";
const FIELD =
  "rounded-control border border-input bg-surface px-2 text-body text-ink";

const tone = {
  waiting: "muted",
  overdue: "warning",
  answered: "positive",
} as const;

/** The case page's "Ask a specialist" button and form. */
export function AskSpecialist({
  caseId,
  caseLabel,
  now,
}: {
  caseId: string;
  caseLabel: string;
  now: number;
}) {
  const id = useId();
  const [open, setOpen] = useState(false);
  const [pick, setPick] = useState<string>(SPECIALTIES[0].id);
  const [question, setQuestion] = useState("");
  const [error, setError] = useState("");
  const chosen = SPECIALTIES.find((s) => s.id === pick) ?? SPECIALTIES[0];

  function submit(e: React.FormEvent) {
    e.preventDefault();
    const r = newConsult(
      { caseId, caseLabel, specialty: pick, question, now },
      `${caseId}#consult-${Date.now().toString(36)}`,
    );
    if (!r.ok) return setError(r.error);
    saveConsult(r.consult);
    setQuestion("");
    setError("");
    setOpen(false);
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <button
          type="button"
          className={cn(BUTTON, QUIET, "cursor-pointer gap-2")}
          data-testid="ask-specialist"
        >
          <MessagesSquare aria-hidden className="size-4" />
          Ask a specialist
        </button>
      </DialogTrigger>
      <DialogContent>
        <form onSubmit={submit} className="flex flex-col gap-4">
          <DialogHeader>
            <DialogTitle>Ask a specialist</DialogTitle>
            <DialogDescription>
              {chosen.owner} replies by {formatDayYear(consultReplyBy(now))}.
            </DialogDescription>
          </DialogHeader>
          <label
            htmlFor={`${id}-specialty`}
            className="flex flex-col gap-1 text-small text-muted"
          >
            Specialty
            <select
              id={`${id}-specialty`}
              value={pick}
              onChange={(e) => setPick(e.target.value)}
              className={cn(FIELD, "h-11")}
            >
              {SPECIALTIES.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.label}
                </option>
              ))}
            </select>
          </label>
          <label
            htmlFor={`${id}-question`}
            className="flex flex-col gap-1 text-small text-muted"
          >
            Question
            <textarea
              id={`${id}-question`}
              value={question}
              maxLength={QUESTION_MAX}
              rows={3}
              onChange={(e) => setQuestion(e.target.value)}
              className={cn(FIELD, "py-2")}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? `${id}-error` : undefined}
            />
          </label>
          {error && (
            <p
              id={`${id}-error`}
              role="alert"
              className="m-0 text-small text-warning"
            >
              {error}
            </p>
          )}
          <DialogFooter>
            <button type="submit" className={cn(BUTTON, PRIMARY)}>
              Send question
            </button>
          </DialogFooter>
          <p className="m-0 text-small text-muted">Kept in this browser.</p>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** This case's consults as timeline rows, inside the server-rendered Activity list. */
export function ConsultTimelineRows({
  caseId,
  now,
}: {
  caseId: string;
  now: number;
}) {
  const consults = useConsults().filter((c) => c.caseId === caseId);
  return (
    <>
      {consults.map((c) => (
        <ConsultRow key={c.id} c={c} now={now} />
      ))}
    </>
  );
}

function ConsultRow({ c, now }: { c: Consult; now: number }) {
  const id = useId();
  const [reply, setReply] = useState("");
  const status = consultStatus(c, now);
  const s = specialty(c.specialty);
  return (
    <li
      className="record-event-row"
      data-testid="consult-row"
      id={`consult-${c.id.split("#").pop()}`}
    >
      <span className="record-event-icon">
        <MessagesSquare aria-hidden="true" />
      </span>
      <span className="min-w-0">
        <strong>Asked {s.label}</strong>
        <span className="block">“{c.question}”</span>
        <span className="mt-1 flex flex-wrap items-center gap-2 text-small text-muted">
          <Pill tone={tone[status]}>{statusWords[status]}</Pill>
          {c.owner} · reply by {formatDayYear(c.replyBy)}
        </span>
        {c.reply ? (
          <span className="mt-1 block" data-testid="consult-reply">
            <span className="text-muted">Reply: </span>
            {c.reply}
            {!c.seen && (
              <button
                type="button"
                className="ml-2 cursor-pointer text-small underline"
                onClick={() => saveConsult({ ...c, seen: true })}
              >
                Mark as read
              </button>
            )}
          </span>
        ) : (
          <form
            className="mt-2 flex flex-wrap items-end gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              const next = answerConsult(c, reply, now);
              if (next !== c) saveConsult(next);
            }}
          >
            <label
              htmlFor={`${id}-reply`}
              className="flex min-w-[200px] flex-1 flex-col gap-1 text-small text-muted"
            >
              Specialist&apos;s reply
              <input
                id={`${id}-reply`}
                value={reply}
                maxLength={1000}
                onChange={(e) => setReply(e.target.value)}
                className={cn(FIELD, "h-11")}
              />
            </label>
            <button
              type="submit"
              disabled={!reply.trim()}
              className={cn(
                BUTTON,
                QUIET,
                "cursor-pointer disabled:cursor-not-allowed disabled:opacity-60",
              )}
            >
              Record reply
            </button>
          </form>
        )}
      </span>
      <small className="tabular-nums">{formatDayYear(c.askedDay)}</small>
    </li>
  );
}
