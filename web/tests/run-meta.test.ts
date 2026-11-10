import { describe, expect, it } from "vitest";
import { runStripCopy } from "@/components/shell/run-meta-copy";
import { formatUtcInstant } from "@/lib/domain/day";

describe("formatUtcInstant", () => {
  it("formats an ISO instant in UTC to the minute", () => {
    expect(formatUtcInstant("2026-11-06T13:56:52Z")).toBe("6 Nov 2026, 13:56 UTC");
    expect(formatUtcInstant("2026-10-31T23:47:27+04:00")).toBe("31 Oct 2026, 19:47 UTC");
  });
  it("returns the raw string when it does not parse", () => {
    expect(formatUtcInstant("not a date")).toBe("not a date");
  });
});

describe("runStripCopy", () => {
  // The values the published snapshot (web/data/api/dashboard.json) carries.
  const meta = { data_mode: "simulated", loaded_at: "2026-11-06T13:56:52Z", run: "v7strict-cached-fhir-lung-dr-breast-ldct-ifhas-drnext-lungsurv-xreflink" };

  it("shows plain words for the published snapshot, with no identifiers or ISO timestamp", () => {
    const copy = runStripCopy(meta, "fixtures");
    expect(copy).toEqual({ data: "Simulated data", when: "Snapshot 6 Nov 2026, 13:56 UTC" });
    const visible = `${copy.data} ${copy.when}`;
    expect(visible).not.toContain(meta.run);
    expect(visible).not.toContain("data_mode");
    expect(visible).not.toMatch(/\d{4}-\d{2}-\d{2}T/);
  });

  it("says when a live run was loaded rather than calling it a snapshot", () => {
    expect(runStripCopy(meta, "live").when).toBe("Run loaded 6 Nov 2026, 13:56 UTC");
  });

  it("does not relabel a data mode other than simulated", () => {
    expect(runStripCopy({ ...meta, data_mode: "demo" }, "live").data).toBe("Data mode: demo");
  });
});
