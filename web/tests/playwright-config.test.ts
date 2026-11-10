import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => {
  vi.unstubAllEnvs();
  vi.resetModules();
});

describe("Playwright CI configuration", () => {
  it("rejects focused tests and emits actionable CI diagnostics", async () => {
    vi.stubEnv("CI", "1");
    vi.resetModules();
    const { default: config } = await import("../playwright.config");

    expect(config.forbidOnly).toBe(true);
    expect(config.reporter).toEqual(expect.arrayContaining([["github"], ["list"], ["html", { open: "never" }]]));
    expect(config.use?.trace).toBe("retain-on-failure");
  });
});
