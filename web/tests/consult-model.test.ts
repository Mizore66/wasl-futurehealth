import { describe, expect, it } from "vitest";
import { answerConsult, consultReplyBy, consultStatus, consultsForToday, newConsult, QUESTION_MAX, type Consult } from "@/components/case/consult-model";
import { dayToDate } from "@/lib/domain/day";

const ask = (over: Partial<Parameters<typeof newConsult>[0]> = {}, id = "c1") =>
  newConsult({ caseId: "p/ServiceRequest/1", caseLabel: "Case 1", specialty: "gastroenterology", question: "  Repeat colonoscopy   now? ", now: 100, ...over }, id);

const ok = (r: ReturnType<typeof ask>): Consult => {
  if (!r.ok) throw new Error(r.error);
  return r.consult;
};

describe("consult model", () => {
  it("names the owner, tidies the question and sets a 2-working-day reply-by date", () => {
    const c = ok(ask());
    expect(c.owner).toBe("Dr Layla Haddad");
    expect(c.question).toBe("Repeat colonoscopy now?");
    expect(c.replyBy).toBe(consultReplyBy(100));
    // Two working days: never lands on a weekend, and is at least two days later.
    expect(c.replyBy - c.askedDay).toBeGreaterThanOrEqual(2);
    expect([0, 6]).not.toContain(dayToDate(c.replyBy).getUTCDay());
  });

  it("skips the weekend: asked on a Friday, due on the Tuesday", () => {
    // Day 0 is Fri 1 May 2026.
    expect(dayToDate(0).getUTCDay()).toBe(5);
    expect(consultReplyBy(0)).toBe(4);
  });

  it("refuses an unknown specialty, an empty or an over-long question", () => {
    expect(ask({ specialty: "cardiology" }).ok).toBe(false);
    expect(ask({ question: "   " }).ok).toBe(false);
    expect(ask({ question: "x".repeat(QUESTION_MAX + 1) }).ok).toBe(false);
  });

  it("is waiting, then overdue after the reply-by day, and answered once a reply is recorded", () => {
    const c = ok(ask());
    expect(consultStatus(c, c.replyBy)).toBe("waiting");
    expect(consultStatus(c, c.replyBy + 1)).toBe("overdue");
    const answered = answerConsult(c, " Yes, within 6 weeks. ", c.replyBy + 2);
    expect(consultStatus(answered, c.replyBy + 2)).toBe("answered");
    expect(answered.reply).toBe("Yes, within 6 weeks.");
    // A second reply keeps the first; an empty one changes nothing.
    expect(answerConsult(answered, "No", 999)).toBe(answered);
    expect(answerConsult(c, "  ", 101)).toBe(c);
  });

  it("puts overdue and unread answers on Today, overdue first; waiting and read ones stay off", () => {
    const waiting = ok(ask({ now: 200 }, "w"));
    const overdue = ok(ask({ now: 100 }, "o"));
    const answered = answerConsult(ok(ask({ now: 90 }, "a")), "Fine", 150);
    const read = { ...answerConsult(ok(ask({ now: 80 }, "r")), "Fine", 150), seen: true };
    expect(consultsForToday([answered, waiting, read, overdue], 200).map((c) => c.id)).toEqual(["o", "a"]);
  });
});
