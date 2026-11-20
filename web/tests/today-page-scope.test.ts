import { readFileSync } from "node:fs";
import path from "node:path";
import { beforeEach, expect, it, vi } from "vitest";
import type { ComponentProps, ReactElement } from "react";
import Page from "@/app/app/today/page";
import { TodayBoard } from "@/components/worklist/today-board";
import { TodayScope } from "@/components/worklist/today-scope";
import { CaseList, type Programme } from "@/lib/api/schema";
import { matches, type CaseQuery } from "@/lib/api/source";

const mocks = vi.hoisted(() => ({ all: vi.fn() }));
vi.mock("@/lib/api/all-cases", () => ({ getAllCases: mocks.all }));
vi.mock("@/components/worklist/run-clock", () => ({ runClock: async () => 120, hasRecord: () => true }));

const PROGRAMMES: Programme[] = ["crc", "cervical", "lung_nodule", "diabetic_retinopathy", "breast", "lung_ldct", "ifhas"];
const open = PROGRAMMES.flatMap((p) =>
  CaseList.parse(JSON.parse(readFileSync(path.join(__dirname, "..", "data", "api", `cases_${p}_page1.json`), "utf8"))).cases.filter((c) => !c.closed),
);

beforeEach(() => {
  vi.clearAllMocks();
  mocks.all.mockImplementation(async (q: CaseQuery = {}) => ({ cases: open.filter((c) => matches(c, q)) }));
});

async function render(sp: Record<string, string>) {
  // Server component unit test with mocked reads; not served HTTP or browser evidence.
  const board = (await Page({ params: Promise.resolve({}), searchParams: Promise.resolve(sp) })) as ReactElement<ComponentProps<typeof TodayBoard>>;
  const scope = (board.props.head as ReactElement<{ aside: ReactElement }>).props.aside;
  expect([scope.type, board.type]).toEqual([TodayScope, TodayBoard]);
  return { scope: scope.props as ComponentProps<typeof TodayScope>, board: board.props as ComponentProps<typeof TodayBoard> };
}

it("opens on the CRC queue at Al Mushrif when the URL chooses nothing", async () => {
  const { board } = await render({});
  expect(mocks.all.mock.calls).toEqual([[{ closed: false }]]);
  expect(board.items.length).toBeGreaterThan(0);
  expect(board.items.every((i) => i.id.startsWith("ahs-mushrif/") && i.deadline === 31)).toBe(true);
  expect(board.scope).toBe("Colorectal screening · Al Mushrif");
});

it.each(PROGRAMMES.filter((p) => p !== "crc"))("shows only open %s cases, each on its own clock", async (programme) => {
  const { board, scope } = await render({ programme, provider: "" });
  const want = open.filter((c) => c.programme === programme);
  expect(board.items.map((i) => i.id)).toEqual(want.map((c) => c.id));
  expect(scope.count).toBe(want.length);
  expect(scope.options.programme.map((o) => o.value)).toContain(programme);
  expect(board.items.map((i) => i.deadline)).toEqual(want.map((c) => Math.floor(c.deadline_day)));
});

it("gives an honest empty state for a programme and provider with no open cases", async () => {
  const { board, scope } = await render({ programme: "breast", provider: "ahs-mushrif" });
  expect(board.items).toEqual([]);
  expect(scope.count).toBe(0);
});

it("passes the URL's queue to the board, To call unless it names Waiting or No owner", async () => {
  expect((await render({})).board.queue).toBe("call");
  expect((await render({ queue: "waiting" })).board.queue).toBe("waiting");
  expect((await render({ queue: "none", programme: "crc" })).board.queue).toBe("none");
  expect((await render({ queue: "everything" })).board.queue).toBe("call");
});
