import { ESLint } from "eslint";
import { describe, expect, it } from "vitest";

describe("ESLint generated-artifact boundaries", () => {
  it("ignores Playwright reports and traces instead of linting generated JavaScript", async () => {
    const eslint = new ESLint({ cwd: process.cwd() });

    await expect(eslint.isPathIgnored("playwright-report/trace/assets/generated.js")).resolves.toBe(true);
    await expect(eslint.isPathIgnored("test-results/artifacts/trace.js")).resolves.toBe(true);
  });
});
