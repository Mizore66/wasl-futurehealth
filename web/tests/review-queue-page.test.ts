import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CaseDetail } from "@/lib/api/schema";
import hold from "@/data/api/case_review_hold.json";

const queue = vi.hoisted(() => ({ reviewQueue: vi.fn() }));
const session = vi.hoisted(() => ({ token: undefined as string | undefined }));
vi.mock("@/components/review/queue", () => queue);
vi.mock("@/lib/api/source", () => ({ apiMode: () => "live" }));
vi.mock("@/lib/api/reviewer-session", () => ({ reviewerToken: async () => session.token }));

afterEach(() => {
  vi.clearAllMocks();
  session.token = undefined;
});

async function render() {
  const { default: Page } = await import("@/app/app/review/page");
  return renderToStaticMarkup(await Page({ searchParams: Promise.resolve({}) } as never));
}

describe("review queue page with rejected reviewer credentials", () => {
  it.each([
    [401, /sign-in was not accepted/],
    [403, /not allowed to read/],
    [404, /could not be loaded/],
  ] as const)("shows an explicit %s error instead of the practice or empty-work claim", async (status, message) => {
    session.token = "stored-token-1234567890";
    queue.reviewQueue.mockResolvedValue({ cases: [], source: "incomplete", failures: [{ id: "case-a", status }] });
    const markup = await render();
    expect(markup).toMatch(message);
    expect(markup).toContain('role="alert"');
    expect(markup).not.toContain("no open review items");
    expect(markup).not.toContain("Practice queue");
    expect(markup).not.toContain("0 cases wait");
    expect(markup).not.toContain("Open review items from the API");
  });

  it("does not present a partial queue as complete", async () => {
    session.token = "stored-token-1234567890";
    const detail = CaseDetail.parse(hold);
    queue.reviewQueue.mockResolvedValue({ cases: [detail], source: "incomplete", failures: [{ id: "case-b", status: 401 }] });
    const markup = await render();
    expect(markup).toContain("Mariam Al Shamsi");
    expect(markup).toMatch(/1 open case could not be loaded/);
    expect(markup).not.toMatch(/1 case waits/);
    expect(markup).not.toContain("Open review items from the API");
    expect(markup).toMatch(/Incomplete/);
  });

  it("keeps the practice queue and its label for a verified, genuinely empty queue", async () => {
    session.token = "stored-token-1234567890";
    queue.reviewQueue.mockResolvedValue({ cases: [CaseDetail.parse(hold)], source: "rebuilt" });
    const markup = await render();
    expect(markup).toContain("no open review items");
    expect(markup).toContain("Practice queue");
  });

  it("keeps the signed-out guidance when no viewer token exists", async () => {
    queue.reviewQueue.mockResolvedValue({ cases: [CaseDetail.parse(hold)], source: "open" });
    const markup = await render();
    expect(markup).toContain("Open a case on the review desk and sign in");
    expect(markup).not.toContain('role="alert"');
  });
});
