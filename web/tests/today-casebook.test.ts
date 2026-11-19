import { describe, expect, it } from "vitest";
import { contactWords, taskSentence, taskTitle } from "@/components/worklist/today-task";

const base = { view: "call" as const, status: "Notified, hasn't confirmed", next: "Call to confirm they received the result" };

describe("Today casebook words", () => {
  it("titles a case by its task, from the work item alone", () => {
    expect(taskTitle({ ...base, communicationStep: "receipt" })).toBe("Receipt outstanding");
    expect(taskTitle({ ...base, status: "Never notified", communicationStep: "notify" })).toBe("Initial notice due");
    expect(taskTitle({ ...base, teachBackCallback: { status: "Teach-back failed · nurse callback open", ref: "a1b2c3d4" } })).toBe("Teach-back callback");
    // Once the case leaves To call, the title follows where it now stands.
    expect(taskTitle({ ...base, view: "waiting", status: "Booking needed · date to confirm", communicationStep: "receipt" })).toBe("Booking needed");
    expect(taskTitle({ ...base, status: "No-show, rebook" })).toBe("No-show, rebook");
  });

  it("explains the next action in a sentence, without repeating the title", () => {
    expect(taskSentence({ ...base, communicationStep: "receipt" })).toBe("Notified, hasn't confirmed. Call to confirm they received the result.");
    expect(taskSentence({ ...base, status: "No-show, rebook", next: "Arrange a new date" })).toBe("Arrange a new date.");
  });

  it("names the contact restriction before any tab is opened", () => {
    expect(contactWords({ contactBlocked: true, consent: "granted" })).toEqual({ label: "Contact on hold", tone: "warn" });
    expect(contactWords({ contactBlocked: false, consent: "granted" })).toEqual({ label: "Contact permitted", tone: "ok" });
    expect(contactWords({ contactBlocked: false, consent: "unknown" })).toEqual({ label: "Contact permission unknown", tone: "warn" });
    expect(contactWords({ contactBlocked: false, consent: "denied" })).toEqual({ label: "Texts not permitted", tone: "warn" });
  });
});
