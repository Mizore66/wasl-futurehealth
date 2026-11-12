import { expect, test } from "@playwright/test";
import { LONG_TASK_BUDGET_MS, SHELL_MARGIN_MS, longTaskBudget } from "./long-task-budget";

test.describe("longTaskBudget", () => {
  test("keeps the fixed 200ms bar on a fast runner", () => {
    expect(longTaskBudget(90)).toBe(LONG_TASK_BUDGET_MS);
    expect(longTaskBudget(160)).toBe(LONG_TASK_BUDGET_MS);
  });

  test("tracks a slow runner's shell floor by a small margin (CI: shell 213ms)", () => {
    expect(longTaskBudget(213)).toBe(213 + SHELL_MARGIN_MS);
    expect(213).toBeLessThanOrEqual(longTaskBudget(213));
  });

  test("still fails a real regression on a slow runner", () => {
    // Shell 213ms; a filtered render adding more than the margin on top must exceed the budget.
    expect(213 + SHELL_MARGIN_MS + 1).toBeGreaterThan(longTaskBudget(213));
    // And on a fast runner 250ms against a 100ms shell is a regression.
    expect(250).toBeGreaterThan(longTaskBudget(100));
  });

  test("margin is small", () => {
    expect(SHELL_MARGIN_MS).toBeLessThanOrEqual(LONG_TASK_BUDGET_MS / 4);
  });
});
