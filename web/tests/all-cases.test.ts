import { beforeEach, describe, expect, it, vi } from "vitest";
import { getAllCases } from "@/lib/api/all-cases";

const source = vi.hoisted(() => ({ getCases: vi.fn() }));
vi.mock("@/lib/api/source", () => source);

const serve = (ids: string[], pageSize: number) =>
  source.getCases.mockImplementation(async ({ offset = 0 }: { offset?: number }) => ({
    meta: { m: 1 },
    cases: ids.slice(offset, offset + pageSize).map((id) => ({ id })),
    total: ids.length,
    limit: pageSize,
    offset,
  }));

beforeEach(() => {
  source.getCases.mockReset();
});

describe("getAllCases", () => {
  it("walks every page once and keeps the query filters", async () => {
    const ids = Array.from({ length: 1201 }, (_, i) => `c${String(i).padStart(4, "0")}`);
    serve(ids, 500);
    const all = await getAllCases({ provider: "ahs-mushrif", programme: "crc", q: "c" });
    expect(all.cases.map((c) => c.id)).toEqual(ids);
    expect(all.total).toBe(1201);
    expect(source.getCases.mock.calls.map(([q]) => q.offset)).toEqual([0, 500, 1000]);
    for (const [q] of source.getCases.mock.calls) expect(q).toMatchObject({ provider: "ahs-mushrif", programme: "crc", q: "c" });
  });

  it("stops at an exact page boundary without an extra request", async () => {
    serve(Array.from({ length: 1000 }, (_, i) => `c${i}`), 500);
    const all = await getAllCases({});
    expect(all.cases).toHaveLength(1000);
    expect(source.getCases).toHaveBeenCalledTimes(2);
  });

  it("returns an empty result for no matches", async () => {
    serve([], 500);
    expect((await getAllCases({})).cases).toEqual([]);
    expect(source.getCases).toHaveBeenCalledTimes(1);
  });
});
